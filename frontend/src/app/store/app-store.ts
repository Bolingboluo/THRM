import { create } from 'zustand';
import { types } from '../../../wailsjs/go/models';
import { apiService } from '../services/api';
import { configService } from '../services/config-service';
import { deviceService, type DeviceStatusPayload } from '../services/device-service';
import {
  appendSampledHistoryPoint,
  clampHistoryRetentionHours,
  createLiveHistoryPoint,
  DEFAULT_HISTORY_RETENTION_HOURS,
  HISTORY_SAMPLE_INTERVAL_MS,
  mergeTimelineEvents,
  normalizeHistoryPoint,
  SESSION_HISTORY_LIMIT,
  SESSION_HISTORY_RETENTION_MS,
  trimHistoryPoints,
} from '../lib/temperature-history';
import type { TemperatureHistoryPoint, TimelineEvent } from '../lib/temperature-history';
import { i18n } from '../lib/i18n';
import { toast } from 'sonner';
import type { DeviceSettings } from '../types/app';

const getBridgeWarningMessage = () => i18n.t('store.bridgeWarning.default');

const getCoreServiceErrorMessage = (detail?: string) => {
  const trimmed = detail?.trim();
  if (
    trimmed?.includes(i18n.t('store.coreService.unavailable')) ||
    trimmed?.startsWith('核心服务不可用') ||
    trimmed?.startsWith('Core service is unavailable') ||
    trimmed?.startsWith('Core サービスを利用できません')
  ) {
    return trimmed;
  }
  return trimmed
    ? i18n.t('store.coreService.unavailableWithDetail', { detail: trimmed })
    : i18n.t('store.coreService.unavailable');
};

const isCoreServiceFailureDetail = (detail?: string) => {
  const normalized = detail?.toLowerCase() ?? '';
  return normalized.includes('core') ||
    normalized.includes('核心服务') ||
    normalized.includes('ipc') ||
    normalized.includes('服务器') ||
    normalized.includes('服务');
};

/**
 * 把 next 里与 prev 内容相同的子数组/子对象换成 prev 的引用；整体都相同时直接返回 prev。
 *
 * GUI 进程每次推送温度都会把完整的传感器清单重新合并一遍，清单内容没变也是新数组，
 * 依赖它们的 useMemo 与下拉选项便每次推送都重算。换回旧引用后，订阅者按引用比较即可
 * 跳过没有变化的部分。就地替换 next 的字段：它是刚解析出来的事件载荷，没有别人持有。
 */
function reuseEqualStructure<T>(prev: unknown, next: T): T {
  if (Object.is(prev, next)) return prev as T;
  if (typeof next !== 'object' || next === null || typeof prev !== 'object' || prev === null) return next;
  if (Array.isArray(next) !== Array.isArray(prev)) return next;

  const prevRecord = prev as Record<string, unknown>;
  const nextRecord = next as Record<string, unknown>;
  const keys = Object.keys(nextRecord);
  let same = keys.length === Object.keys(prevRecord).length;
  for (const key of keys) {
    const reused = reuseEqualStructure(prevRecord[key], nextRecord[key]);
    if (reused !== nextRecord[key]) nextRecord[key] = reused;
    if (same && (!Object.is(reused, prevRecord[key]) || !(key in prevRecord))) same = false;
  }
  return same ? (prev as T) : next;
}

// 一小时可容纳的样本数（按固定采样间隔换算），用于把保留小时数换算成点数上限。
const HISTORY_POINTS_PER_HOUR = Math.round((60 * 60 * 1000) / HISTORY_SAMPLE_INTERVAL_MS);

// 核心历史的裁剪窗口。minIntervalMs 为 0：核心推来的点已经按采样间隔筛过，
// 这里只借 appendSampledHistoryPoint 处理系统时钟回拨（丢掉落在"未来"的点，与核心一致）。
const coreHistoryWindow = (hours: number) => ({
  retentionMs: hours * 60 * 60 * 1000,
  limit: hours * HISTORY_POINTS_PER_HOUR,
  minIntervalMs: 0,
});

type ActiveTab = 'status' | 'curve' | 'control' | 'about';
export type CurveFocusTarget = 'curve-editor' | 'history-details';
// labelKey 为 i18n 键（在 FanCurve 时间线渲染时翻译）；不再存储已本地化的字面量，保证跟随语言切换。
export type { TimelineEvent } from '../lib/temperature-history';

interface AppStore {
  isConnected: boolean;
  deviceProductId: string | null;
  deviceModel: string | null;
  deviceSettings: DeviceSettings | null;
  config: types.AppConfig | null;
  fanData: types.FanData | null;
  temperature: types.TemperatureData | null;
  legionFnQSupported: boolean;
  bridgeWarning: string | null;
  coreServiceError: string | null;
  isLoading: boolean;
  error: string | null;
  activeTab: ActiveTab;
  curveFocusTarget: CurveFocusTarget | null;
  sessionHistoryPoints: TemperatureHistoryPoint[];
  // 核心记录的后台温度历史，全应用共用一份：首页与曲线页不再各自整表下载、各持一份数组。
  historyPoints: TemperatureHistoryPoint[];
  historyEnabled: boolean;
  historyRetentionHours: number;
  historySaving: boolean;
  timelineEvents: TimelineEvent[];

  setActiveTab: (tab: ActiveTab) => void;
  openCurveTab: (target: CurveFocusTarget) => void;
  clearCurveFocusTarget: () => void;
  clearBridgeWarning: () => void;
  handleTemperaturePayload: (data: types.TemperatureData | null) => void;
  appendSessionHistoryPoint: (data: types.TemperatureData | null, fanRpm?: number) => void;
  mergeTimelineEvents: (events: TimelineEvent[] | null | undefined) => void;
  ensureTemperatureHistoryLoaded: () => void;
  loadTemperatureHistory: () => Promise<void>;
  setTemperatureHistoryEnabled: (enabled: boolean) => Promise<void>;
  setTemperatureHistoryRetentionHours: (hours: number) => Promise<void>;

  initializeApp: () => Promise<void>;
  resyncCore: () => Promise<void>;
  connectDevice: () => Promise<void>;
  disconnectDevice: () => Promise<void>;
  setConfig: (config: types.AppConfig) => void;

  startEventListeners: () => () => void;
}

// resyncCore 的重入哨兵，见该方法注释。
let resyncInFlight = false;

// 历史快照是否成功拉取过：只在第一次有组件用到历史时整表下载，之后靠实时推送续上。
// 失败时保持 false，下一个用到历史的组件挂载时再试（与原先每次挂载都拉取的兜底一致）。
let historyLoaded = false;
// 只采用最后一次发起的拉取结果，先发后至的旧快照直接丢弃。
let historyLoadSeq = 0;
// 拉取快照期间到达的实时点：快照可能是在它们之前生成的，落定后要补回末尾。null 表示没有在拉取。
let historyLoadBuffer: unknown[] | null = null;

export const useAppStore = create<AppStore>((set, get) => ({
  isConnected: false,
  deviceProductId: null,
  deviceModel: null,
  deviceSettings: null,
  config: null,
  fanData: null,
  temperature: null,
  legionFnQSupported: false,
  bridgeWarning: null,
  coreServiceError: null,
  isLoading: true,
  error: null,
  activeTab: 'status',
  curveFocusTarget: null,
  sessionHistoryPoints: [],
  historyPoints: [],
  historyEnabled: false,
  historyRetentionHours: DEFAULT_HISTORY_RETENTION_HOURS,
  historySaving: false,
  timelineEvents: [],

  setActiveTab: (tab) => set({ activeTab: tab, curveFocusTarget: null }),

  openCurveTab: (target) => set({ activeTab: 'curve', curveFocusTarget: target }),

  clearCurveFocusTarget: () => set({ curveFocusTarget: null }),

  clearBridgeWarning: () => set({ bridgeWarning: null }),

  handleTemperaturePayload: (data) => {
    const bridgeMessage = data?.bridgeMessage?.trim() ?? '';
    set({
      temperature: reuseEqualStructure(get().temperature, data),
      bridgeWarning: data?.bridgeOk === false ? bridgeMessage || getBridgeWarningMessage() : null,
    });
  },

  // fanRpm 缺省取 store 里的转速；窗口隐藏期间 store 不更新遥测，由调用方传入最新值。
  appendSessionHistoryPoint: (data, fanRpm) => {
    if (!data) return;

    const point = createLiveHistoryPoint({
      updateTime: data.updateTime,
      cpuTemp: data.cpuTemp,
      gpuTemp: data.gpuTemp,
      cpuPower: data.cpuPower,
      gpuPower: data.gpuPower,
      cpuFanRpm: (data as { cpuFanRpm?: number }).cpuFanRpm,
      gpuFanRpm: (data as { gpuFanRpm?: number }).gpuFanRpm,
    }, Number((fanRpm ?? get().fanData?.currentRpm) || 0));

    if (!point) return;

    set((state) => ({
      sessionHistoryPoints: appendSampledHistoryPoint(state.sessionHistoryPoints, point, {
        retentionMs: SESSION_HISTORY_RETENTION_MS,
        limit: SESSION_HISTORY_LIMIT,
      }),
    }));
  },

  // 快照与实时推送共用一个入口，由 mergeTimelineEvents 负责去重与排序。
  mergeTimelineEvents: (events) => {
    if (!events || events.length === 0) return;
    set((state) => ({ timelineEvents: mergeTimelineEvents(state.timelineEvents, events) }));
  },

  ensureTemperatureHistoryLoaded: () => {
    if (historyLoaded || historyLoadBuffer !== null) return;
    void get().loadTemperatureHistory();
  },

  loadTemperatureHistory: async () => {
    const seq = ++historyLoadSeq;
    if (historyLoadBuffer === null) historyLoadBuffer = [];
    try {
      const payload = await apiService.getTemperatureHistory();
      if (seq !== historyLoadSeq) return;
      const buffered = historyLoadBuffer ?? [];
      historyLoadBuffer = null;

      const enabled = payload?.enabled !== false;
      const retentionHours = clampHistoryRetentionHours(payload?.retentionHours);
      const historyWindow = coreHistoryWindow(retentionHours);
      let points = enabled
        ? trimHistoryPoints((payload?.points || []) as TemperatureHistoryPoint[], historyWindow.retentionMs, historyWindow.limit)
        : [];
      if (enabled) {
        for (const raw of buffered) {
          const point = normalizeHistoryPoint(raw as TemperatureHistoryPoint);
          const last = points[points.length - 1];
          if (point && (!last || point.timestamp > last.timestamp)) {
            points = appendSampledHistoryPoint(points, point, historyWindow);
          }
        }
      }

      historyLoaded = true;
      set({ historyEnabled: enabled, historyRetentionHours: retentionHours, historyPoints: points });
      // 补齐界面关闭期间核心记下的断连/唤醒标记，否则曲线有一小时、标记只有几分钟。
      get().mergeTimelineEvents((payload?.events || []) as TimelineEvent[]);
    } catch {
      if (seq !== historyLoadSeq) return;
      historyLoadBuffer = null;
      // 读不到核心历史时退回会话内采样。
      set({ historyEnabled: false, historyPoints: [] });
    }
  },

  setTemperatureHistoryEnabled: async (enabled) => {
    set({ historySaving: true });
    try {
      await apiService.setTemperatureHistoryEnabled(enabled);
      await get().loadTemperatureHistory();
    } catch (error) {
      console.error('设置温度历史失败:', error);
    } finally {
      set({ historySaving: false });
    }
  },

  setTemperatureHistoryRetentionHours: async (hours) => {
    const clamped = clampHistoryRetentionHours(hours);
    set({ historySaving: true });
    try {
      await apiService.setTemperatureHistoryRetentionHours(clamped);
      await get().loadTemperatureHistory();
    } catch (error) {
      console.error('设置温度历史保留时长失败:', error);
    } finally {
      set({ historySaving: false });
    }
  },

  initializeApp: async () => {
    try {
      set({ isLoading: true });

      const [appConfig, deviceStatus, debugInfo] = await Promise.all([
        configService.getConfig(),
        deviceService.getStatus() as Promise<DeviceStatusPayload>,
        apiService.getDebugInfo().catch(() => null),
      ]);
      const coreServiceError = deviceStatus.error ? getCoreServiceErrorMessage(deviceStatus.error) : null;

      set({
        config: appConfig,
        isConnected: deviceStatus.connected || false,
        deviceProductId: deviceStatus.productId || null,
        deviceModel: deviceStatus.model || null,
        deviceSettings: deviceStatus.deviceSettings || null,
        fanData: deviceStatus.currentData || null,
        legionFnQSupported: debugInfo?.legionFnQSupported === true,
        coreServiceError,
        error: coreServiceError,
      });

      get().handleTemperaturePayload(deviceStatus.temperature || null);
    } catch (error) {
      console.error('初始化失败:', error);
      const detail = error instanceof Error ? error.message : undefined;
      const coreServiceError = isCoreServiceFailureDetail(detail) ? getCoreServiceErrorMessage(detail) : null;
      set({ error: coreServiceError || i18n.t('store.errors.initializeApp'), coreServiceError });
    } finally {
      set({ isLoading: false });
    }
  },

  // resyncCore 在 IPC 连接恢复后重新拉取核心状态。
  //
  // 必须有这一步：core-service-error 会把 isConnected/deviceModel/deviceSettings 清空，
  // 而这些字段只能由 device-connected 事件恢复——设备其实从未断开，该事件不会再来。
  // 缺少主动重取时，一次 IPC 抖动就会让界面永久停在"设备未连接"，只能重启 GUI。
  // 与 initializeApp 的区别是不触碰 isLoading，避免恢复时闪一下加载态。
  //
  // core-service-ok 只在"错误→正常"跳变时发出，但恢复期间 core-resynced 与它可能先后到达，
  // 因此仍用 resyncInFlight 兜住重入，避免一次恢复引发多轮重复拉取。
  resyncCore: async () => {
    if (resyncInFlight) return;
    resyncInFlight = true;
    try {
      const [appConfig, deviceStatus, debugInfo] = await Promise.all([
        configService.getConfig(),
        deviceService.getStatus() as Promise<DeviceStatusPayload>,
        apiService.getDebugInfo().catch(() => null),
      ]);
      const coreServiceError = deviceStatus.error ? getCoreServiceErrorMessage(deviceStatus.error) : null;

      set({
        config: appConfig,
        isConnected: deviceStatus.connected || false,
        deviceProductId: deviceStatus.productId || null,
        deviceModel: deviceStatus.model || null,
        deviceSettings: deviceStatus.deviceSettings || null,
        fanData: deviceStatus.currentData || null,
        legionFnQSupported: debugInfo?.legionFnQSupported === true,
        coreServiceError,
        error: coreServiceError,
      });

      get().handleTemperaturePayload(deviceStatus.temperature || null);
      // IPC 断开期间核心照常记录，但实时推送没送到界面：重新拉一次快照补上这段。
      if (historyLoaded) void get().loadTemperatureHistory();
    } catch (error) {
      console.error('核心状态重同步失败:', error);
    } finally {
      resyncInFlight = false;
    }
  },

  connectDevice: async () => {
    try {
      const success = await deviceService.connect();
      if (success) {
        const status = await deviceService.getStatus().catch(() => null);
        const coreServiceError = status?.error ? getCoreServiceErrorMessage(status.error) : null;
        set({
          isConnected: true,
          deviceSettings: status?.deviceSettings || null,
          deviceProductId: status?.productId || get().deviceProductId,
          deviceModel: status?.model || get().deviceModel,
          coreServiceError,
          error: coreServiceError,
        });
      }
    } catch (error) {
      console.error('连接失败:', error);
      set({ error: i18n.t('store.errors.connectDevice') });
    }
  },

  disconnectDevice: async () => {
    try {
      await deviceService.disconnect();
      set({ isConnected: false, deviceProductId: null, deviceModel: null, deviceSettings: null, fanData: null });
    } catch (error) {
      console.error('断开连接失败:', error);
    }
  },

  // 只同步本地状态，不再写回核心：各组件调用 onConfigChange 之前都已经通过
  // apiService.updateConfig 或专用接口（setAutoControl、setLightStrip 等）落盘，
  // 或者就是刚从 getConfig 读回来的。这里再写一次只会把旧快照覆盖回去。
  setConfig: (config) => set({ config, error: null }),

  startEventListeners: () => {
    if (typeof window === 'undefined' || !(window as any).runtime?.EventsOnMultiple) {
      return () => {};
    }
    const unsubscribers: Array<() => void> = [];
    let telemetryTimer: number | null = null;
    let pendingFanData: types.FanData | null | undefined;
    let pendingTemperature: types.TemperatureData | null | undefined;
    // 窗口隐藏期间收到的历史点，可见后再一次性补上（或改为重新拉取快照，见 handleVisibilityChange）。
    let hiddenHistoryPoints: unknown[] = [];
    let hiddenSince: number | null = document.hidden ? performance.now() : null;
    // 窗口隐藏（最小化、收进托盘）时界面看不见，不再把遥测写进 store 触发重渲染：
    // 只留着最新一份，重新可见时立刻应用一次。噪音测试、散热收益测试自己向核心取转速，不读这里。
    const flushTelemetry = () => {
      if (telemetryTimer !== null) {
        window.clearTimeout(telemetryTimer);
        telemetryTimer = null;
      }
      if (document.hidden) return;
      if (pendingFanData !== undefined) {
        const previous = get().fanData;
        const next = reuseEqualStructure(previous, pendingFanData);
        pendingFanData = undefined;
        // 转速推送每秒最多 4 次，内容完全没变时不写 store，首页与徽标就不必跟着重渲染。
        if (next !== previous) set({ fanData: next });
      }
      if (pendingTemperature !== undefined) {
        const data = pendingTemperature;
        pendingTemperature = undefined;
        get().handleTemperaturePayload(data);
        get().appendSessionHistoryPoint(data);
      }
    };
    const scheduleTelemetryFlush = () => {
      if (telemetryTimer !== null || document.hidden) return;
      telemetryTimer = window.setTimeout(flushTelemetry, 200);
    };
    const applyHistoryPoint = (point: unknown) => {
      if (!get().historyEnabled) return;
      set((state) => {
        const next = appendSampledHistoryPoint(
          state.historyPoints,
          point as TemperatureHistoryPoint,
          coreHistoryWindow(state.historyRetentionHours),
        );
        return next === state.historyPoints ? state : { historyPoints: next };
      });
    };
    const handleVisibilityChange = () => {
      if (document.hidden) {
        hiddenSince ??= performance.now();
        if (telemetryTimer !== null) {
          window.clearTimeout(telemetryTimer);
          telemetryTimer = null;
        }
        return;
      }

      const hiddenFor = hiddenSince === null ? 0 : performance.now() - hiddenSince;
      hiddenSince = null;
      flushTelemetry();

      const buffered = hiddenHistoryPoints;
      hiddenHistoryPoints = [];
      // 隐藏期间 GUI 进程可能不再转发高频事件，历史点会缺一段；隐藏超过一个采样间隔就
      // 重新拉一次快照（核心那边一直在记），否则曲线上会多出一段"记录中断"。
      if (hiddenFor >= HISTORY_SAMPLE_INTERVAL_MS && historyLoaded && get().historyEnabled) {
        void get().loadTemperatureHistory();
        return;
      }
      buffered.forEach(applyHistoryPoint);
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    unsubscribers.push(() => document.removeEventListener('visibilitychange', handleVisibilityChange));

    unsubscribers.push(
      apiService.onCoreServiceError((message) => {
        const coreServiceError = getCoreServiceErrorMessage(message);
        // 隐藏期间攒下的旧转速不能在恢复可见时把这里清空的 fanData 又写回去。
        pendingFanData = undefined;
        set({
          coreServiceError,
          error: coreServiceError,
          isConnected: false,
          deviceProductId: null,
          deviceModel: null,
          deviceSettings: null,
          fanData: null,
        });
      })
    );

    unsubscribers.push(
      apiService.onCoreServiceOK(() => {
        // 该事件在每次请求成功时都会到达，只有"从错误态恢复"这一次跳变需要重同步。
        const recovering = get().coreServiceError !== null;
        set((state) => ({
          coreServiceError: null,
          error: state.coreServiceError && state.error === state.coreServiceError ? null : state.error,
        }));
        if (recovering) {
          void get().resyncCore();
        }
      })
    );

    // 核心侧 IPC 看护协程重连成功后主动推送，用于覆盖"GUI 空闲期间核心重启"
    // 这类没有任何请求在飞、因而不会触发 core-service-ok 跳变的场景。
    unsubscribers.push(
      apiService.onCoreResynced(() => {
        void get().resyncCore();
      })
    );

    unsubscribers.push(
      deviceService.onDeviceConnected((deviceInfo) => {
        console.log('设备已连接:', deviceInfo);
        const info = deviceInfo as { productId?: string; model?: string };
        const settings = (deviceInfo as { deviceSettings?: DeviceSettings | null })?.deviceSettings || null;
        set({
          isConnected: true,
          deviceProductId: info.productId || null,
          deviceModel: info.model || null,
          deviceSettings: settings,
          coreServiceError: null,
          error: null,
        });
      })
    );

    unsubscribers.push(
      deviceService.onDeviceDisconnected(() => {
        console.log('设备已断开');
        pendingFanData = undefined;
        set({ isConnected: false, deviceProductId: null, deviceModel: null, deviceSettings: null, fanData: null });
      })
    );

    // 时间轴标记统一由核心推送：核心常驻后台，界面关着时发生的断连/唤醒也记得下来，
    // 打开界面后连同历史快照一起补齐。前端自己从 IPC 事件推导只能覆盖开着界面的那段。
    unsubscribers.push(
      apiService.onTimelineEvent((event) => {
        get().mergeTimelineEvents([event]);
      })
    );

    unsubscribers.push(
      deviceService.onDeviceSettingsUpdate((settings) => {
        set({ deviceSettings: settings || null });
      })
    );

    unsubscribers.push(
      deviceService.onDeviceError((errorMsg) => {
        console.error('设备错误:', errorMsg);
        set({ error: errorMsg });
      })
    );

    unsubscribers.push(
      deviceService.onFanDataUpdate((data) => {
        pendingFanData = data;
        scheduleTelemetryFlush();
      })
    );

    unsubscribers.push(
      deviceService.onTemperatureUpdate((data) => {
        pendingTemperature = data;
        if (document.hidden) {
          // 会话内历史（核心历史关闭时首页与详情页用它）隐藏期间照常采样，可见后曲线才是连续的。
          const fanRpm = pendingFanData !== undefined ? pendingFanData?.currentRpm : get().fanData?.currentRpm;
          get().appendSessionHistoryPoint(data, Number(fanRpm || 0));
        }
        scheduleTelemetryFlush();
      })
    );

    unsubscribers.push(
      apiService.onTemperatureHistoryUpdate((point) => {
        if (historyLoadBuffer !== null) historyLoadBuffer.push(point);
        if (document.hidden) {
          hiddenHistoryPoints.push(point);
          return;
        }
        applyHistoryPoint(point);
      })
    );

    unsubscribers.push(
      configService.onConfigUpdate((updatedConfig) => {
        set({ config: updatedConfig });
      })
    );

    unsubscribers.push(
      deviceService.onHotkeyTriggered((payload) => {
        const message = typeof payload?.message === 'string' ? payload.message : '';
        if (!message) return;
        const ok = payload?.success !== false;
        if (ok) {
          toast.success(i18n.t('store.hotkey.successTitle'), { description: message, duration: 2600 });
        } else {
          toast.error(i18n.t('store.hotkey.failureTitle'), { description: message, duration: 3200 });
        }
      })
    );

    unsubscribers.push(
      deviceService.onLegionPowerModeUpdate((payload) => {
        const mode = typeof payload?.mode === 'string' ? payload.mode : '';
        if (!mode) return;
        const modeLabel: Record<string, string> = {
          Quiet: i18n.t('store.legionModes.Quiet'),
          Balance: i18n.t('store.legionModes.Balance'),
          Performance: i18n.t('store.legionModes.Performance'),
          Extreme: i18n.t('store.legionModes.Extreme'),
          GodMode: i18n.t('store.legionModes.GodMode'),
        };
        toast.success(i18n.t('store.legionFnQ.modeChangedTitle'), {
          description: i18n.t('store.legionFnQ.modeDescription', { mode: modeLabel[mode] || mode }),
          duration: 2600,
        });
      })
    );

    unsubscribers.push(
      apiService.onLegionFnQSupportUpdate((payload) => {
        set({ legionFnQSupported: payload?.supported === true });
      })
    );

    return () => {
      if (telemetryTimer !== null) window.clearTimeout(telemetryTimer);
      unsubscribers.forEach((unsubscribe) => unsubscribe());
    };
  },
}));
