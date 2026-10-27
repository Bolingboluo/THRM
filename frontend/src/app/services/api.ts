// Wails API 服务封装：方法调用走 wails3 生成的强类型绑定（frontend/bindings），
// 事件走 @wailsio/runtime 的 Events，载荷类型由 Go 侧 application.RegisterEvent 声明。
import { Events } from '@wailsio/runtime';
import { App } from '@go/guiapp';
import * as guiapp from '@go/guiapp';
import * as fwupdate from '@go/fwupdate';
import * as ipc from '@go/ipc';
import * as rtss from '@go/rtss';
import * as types from '@go/types';
import type { TimelineEvent } from '../lib/temperature-history';

import type {
  DeviceInfo,
  DeviceDebugCommandResult,
  DeviceDebugFrame,
  DeviceSettings,
  DebugInfo,
  FlydigiCompatStatus,
  LegionFnQSupportPayload,
  LegionPowerModePayload,
  ThemeMeta,
} from '../types/app';

// ===== 固件更新类型（由 Go 结构体生成） =====
export type FirmwareUpdateStatus = guiapp.FirmwareUpdateStatus;
export type FirmwareInfo = fwupdate.FirmwareInfo;
export type FlashFirmwareParams = ipc.FlashFirmwareParams;
export type FirmwareFlashEvent = guiapp.FirmwareFlashEvent;
export type UpdateProgress = guiapp.UpdateProgress;

type CustomEventName = keyof Events.CustomEvents;

// 订阅 Go 侧推送的事件，回调直接拿到载荷（v3 的 Events.On 回调收到的是包着 data 的事件对象）。
// 返回取消订阅函数。
function on<E extends CustomEventName>(name: E, callback: (data: Events.CustomEvents[E]) => void): () => void {
  return Events.On(name, (event) => callback(event.data as Events.CustomEvents[E]));
}

class ApiService {
  // 设备连接
  async connectDevice(): Promise<boolean> {
    return await App.ConnectDevice();
  }

  async disconnectDevice(): Promise<void> {
    return await App.DisconnectDevice();
  }

  async getDeviceStatus(): Promise<any> {
    return await App.GetDeviceStatus();
  }

  // 前端的 DeviceSettings 把若干字符串字段收窄成了字面量联合，Go 侧生成的是 string。
  async refreshDeviceSettings(): Promise<DeviceSettings | null> {
    return (await App.RefreshDeviceSettings()) as DeviceSettings | null;
  }

  async reinitializeDeviceFirmware(factoryReset = false): Promise<DeviceSettings | null> {
    return (await App.ReinitializeDeviceFirmware(factoryReset)) as DeviceSettings | null;
  }

  // 配置管理
  async getConfig(): Promise<types.AppConfig> {
    return await App.GetConfig();
  }

  async getAppVersion(): Promise<string> {
    return await App.GetAppVersion();
  }

  // 下载并全自动静默安装应用更新（Windows），期间弹出 CMD 状态窗口展示更新动态。
  // 下载进度通过 update-download-progress 事件推送；三个文案参数为状态窗口的本地化文字。
  async downloadAndInstallUpdate(
    downloadURL: string,
    windowTitle: string,
    windowBody: string,
    windowRestarting: string,
  ): Promise<void> {
    return await App.DownloadAndInstallUpdate(
      downloadURL,
      windowTitle,
      windowBody,
      windowRestarting,
    );
  }

  onUpdateDownloadProgress(callback: (payload: UpdateProgress) => void): () => void {
    return on('update-download-progress', callback);
  }

  async updateConfig(config: types.AppConfig): Promise<void> {
    return await App.UpdateConfig(config);
  }

  async previewRTSSPosition(mode: 'anchor' | 'custom', x: number, y: number): Promise<void> {
    return await App.PreviewRTSSPosition(mode, x, y);
  }

  async getRTSSLayoutStatus(): Promise<rtss.LayoutStatus> {
    return await App.GetRTSSLayoutStatus();
  }

  async createRTSSAnchor(): Promise<rtss.LayoutStatus> {
    return await App.CreateRTSSAnchor();
  }

  // 风扇曲线
  async setFanCurve(curve: types.FanCurvePoint[]): Promise<void> {
    return await App.SetFanCurve(curve);
  }

  // 清空学习到的曲线偏移；后端清零所有 LearnedOffsets。
  async resetLearnedOffsets(): Promise<void> {
    return await App.ResetLearnedOffsets();
  }

  /* ── 自适应学习 2.0 ── */

  /* ── 散热收益（与学习模式无关） ── */

  async getCoolingBenefit(): Promise<types.CoolingBenefitPayload> {
    return await App.GetCoolingBenefit();
  }

  // 提交扫描测试的原始档位数据；分析在核心侧完成，返回的是分析后的完整结果。
  // 参数按结构声明而不是用生成的类：调用方拿到的是普通对象，
  // 经 Wails 走 JSON 序列化，没必要为了类型强行 new 一个实例。
  async saveCoolingBenefitReport(params: {
    deviceModel: string;
    cpuModel: string;
    gpuModel: string;
    loadLabel: string;
    steps: types.CoolingBenefitStep[];
  }): Promise<types.CoolingBenefitPayload> {
    return await App.SaveCoolingBenefitReport(params as ipc.SaveCoolingBenefitReportParams);
  }

  async clearCoolingBenefit(): Promise<types.CoolingBenefitPayload> {
    return await App.ClearCoolingBenefit();
  }

  // 导出纯文本报告供外部分析。返回保存路径；用户取消保存对话框时返回空串。
  async exportCoolingBenefitReport(): Promise<string> {
    return await App.ExportCoolingBenefitReport();
  }

  // 临时开启扩展温度传感器（内存/硬盘/主板/EC/电源）。会话级，不写配置；
  // 核心侧在 GUI 断开或超时后会自动关闭。
  async setExtendedSensors(enabled: boolean): Promise<boolean> {
    return await App.SetExtendedSensors(enabled);
  }

  async getFanCurve(): Promise<types.FanCurvePoint[]> {
    return await App.GetFanCurve();
  }

  async getFanCurveProfiles(): Promise<{ profiles: Array<{ id: string; name: string; curve: types.FanCurvePoint[] }>; activeId: string }> {
    return await App.GetFanCurveProfiles();
  }

  async setActiveFanCurveProfile(profileID: string): Promise<void> {
    return await App.SetActiveFanCurveProfile(profileID);
  }

  async saveFanCurveProfile(profileID: string, name: string, curve: types.FanCurvePoint[], setActive: boolean): Promise<{ id: string; name: string; curve: types.FanCurvePoint[] }> {
    return await App.SaveFanCurveProfile(profileID, name, curve, setActive);
  }

  async deleteFanCurveProfile(profileID: string): Promise<void> {
    return await App.DeleteFanCurveProfile(profileID);
  }

  async exportFanCurveProfiles(): Promise<string> {
    return await App.ExportFanCurveProfiles();
  }

  async importFanCurveProfiles(code: string): Promise<void> {
    return await App.ImportFanCurveProfiles(code);
  }

  // 智能变频
  async setAutoControl(enabled: boolean): Promise<void> {
    return await App.SetAutoControl(enabled);
  }

  // 自定义转速
  async setCustomSpeed(enabled: boolean, rpm: number): Promise<void> {
    return await App.SetCustomSpeed(enabled, rpm);
  }

  // 手动挡位控制
  async setManualGear(gear: string, level: string): Promise<boolean> {
    return await App.SetManualGear(gear, level);
  }

  // 获取可用挡位
  async getAvailableGears(): Promise<any> {
    return await App.GetAvailableGears();
  }

  // 设备设置
  async setGearLight(enabled: boolean): Promise<boolean> {
    return await App.SetGearLight(enabled);
  }

  async setPowerOnStart(enabled: boolean): Promise<boolean> {
    return await App.SetPowerOnStart(enabled);
  }

  async setSmartStartStop(mode: string): Promise<boolean> {
    return await App.SetSmartStartStop(mode);
  }

  async setBrightness(percentage: number): Promise<boolean> {
    return await App.SetBrightness(percentage);
  }

  async setLightStrip(config: types.LightStripConfig): Promise<void> {
    return await App.SetLightStrip(config);
  }

  async getSmartLightStatus(): Promise<types.SmartTempLightStatus> {
    return await App.GetSmartLightStatus();
  }

  // Windows自启动相关
  async checkWindowsAutoStart(): Promise<boolean> {
    return await App.CheckWindowsAutoStart();
  }

  async setWindowsAutoStart(enabled: boolean): Promise<void> {
    return await App.SetWindowsAutoStart(enabled);
  }

  async getAutoStartMethod(): Promise<string> {
    // 获取当前自启动方式
    return await App.GetAutoStartMethod();
  }

  async setAutoStartWithMethod(enabled: boolean, method: string): Promise<void> {
    // 使用指定方式设置自启动
    return await App.SetAutoStartWithMethod(enabled, method);
  }

  async isRunningAsAdmin(): Promise<boolean> {
    // 检查是否以管理员权限运行
    return await App.IsRunningAsAdmin();
  }

  // 数据获取
  async getTemperature(): Promise<types.TemperatureData> {
    return await App.GetTemperature();
  }

  async getTemperatureHistory(): Promise<types.TemperatureHistoryPayload> {
    return await App.GetTemperatureHistory();
  }

  async setTemperatureHistoryEnabled(enabled: boolean): Promise<void> {
    return await App.SetTemperatureHistoryEnabled(enabled);
  }

  async setTemperatureHistoryRetentionHours(hours: number): Promise<void> {
    return await App.SetTemperatureHistoryRetentionHours(hours);
  }

  async getCurrentFanData(): Promise<types.FanData | null> {
    return await App.GetCurrentFanData();
  }

  async testTemperatureReading(): Promise<types.TemperatureData> {
    return await App.TestTemperatureReading();
  }

  // 桥接程序相关
  async getBridgeProgramStatus(): Promise<any> {
    return await App.GetBridgeProgramStatus();
  }

  async testBridgeProgram(): Promise<any> {
    return await App.TestBridgeProgram();
  }

  async restartPawnIO(): Promise<any> {
    return await App.RestartPawnIO();
  }

  async reinstallPawnIO(): Promise<any> {
    return await App.ReinstallPawnIO();
  }

  // 飞智空间站兼容
  async getFlydigiCompatStatus(): Promise<FlydigiCompatStatus | null> {
    const value = await App.GetFlydigiCompatStatus();
    return (value ?? null) as FlydigiCompatStatus | null;
  }

  async setFlydigiCompat(enabled: boolean): Promise<FlydigiCompatStatus | null> {
    const value = await App.SetFlydigiCompat(enabled);
    return (value ?? null) as FlydigiCompatStatus | null;
  }

  // ===== 固件更新 =====
  async getFirmwareUpdateStatus(): Promise<FirmwareUpdateStatus> {
    const v = await App.GetFirmwareUpdateStatus();
    return (v ?? { supported: false }) as FirmwareUpdateStatus;
  }

  async listFirmwareUpdates(): Promise<FirmwareInfo[]> {
    const v = await App.ListFirmwareUpdates();
    return (v ?? []) as FirmwareInfo[];
  }

  // 弹出原生文件对话框选本地固件；用户取消时返回空串。
  async selectFirmwareFile(): Promise<string> {
    return await App.SelectFirmwareFile();
  }

  async installFirmwareDriver(): Promise<void> {
    return await App.InstallFirmwareDriver();
  }

  async uninstallFirmwareDriver(): Promise<void> {
    return await App.UninstallFirmwareDriver();
  }

  async flashFirmware(params: FlashFirmwareParams): Promise<void> {
    return await App.FlashFirmware(params);
  }

  onFirmwareUpdate(callback: (ev: FirmwareFlashEvent) => void): () => void {
    return on('firmware-update', callback);
  }

  // 事件监听
  onDeviceConnected(callback: (data: DeviceInfo) => void): () => void {
    return on('device-connected', (data) => callback(data as DeviceInfo));
  }

  onDeviceDisconnected(callback: () => void): () => void {
    return on('device-disconnected', callback);
  }

  onDeviceError(callback: (error: string) => void): () => void {
    return on('device-error', callback);
  }

  onDeviceSettingsUpdate(callback: (data: DeviceSettings) => void): () => void {
    return on('device-settings-update', (data) => callback(data as DeviceSettings));
  }

  onFanDataUpdate(callback: (data: types.FanData) => void): () => void {
    return on('fan-data-update', callback);
  }

  onTemperatureUpdate(callback: (data: types.TemperatureData) => void): () => void {
    return on('temperature-update', callback);
  }

  onTemperatureHistoryUpdate(callback: (data: { timestamp: number; cpuTemp: number; gpuTemp: number; cpuPower?: number; gpuPower?: number; fanRpm?: number }) => void): () => void {
    return on('temperature-history-update', callback);
  }

  onConfigUpdate(callback: (config: types.AppConfig) => void): () => void {
    return on('config-update', callback);
  }

  onSmartLightUpdate(callback: (status: types.SmartTempLightStatus) => void): () => void {
    return on('smart-light-update', callback);
  }

  onHotkeyTriggered(callback: (payload: { action: string; shortcut: string; success: boolean; message: string }) => void): () => void {
    return on('hotkey-triggered', callback);
  }

  // 调试相关方法
  onLegionPowerModeUpdate(callback: (payload: LegionPowerModePayload) => void): () => void {
    return on('legion-power-mode-update', callback);
  }

  onLegionFnQSupportUpdate(callback: (payload: LegionFnQSupportPayload) => void): () => void {
    return on('legion-fnq-support-update', callback);
  }

  onFlydigiCompatUpdate(callback: (payload: FlydigiCompatStatus) => void): () => void {
    return on('flydigi-compat-update', callback);
  }

  async getDebugInfo(): Promise<DebugInfo> {
    return await App.GetDebugInfo() as DebugInfo;
  }

  async exportDiagnosticPackage(): Promise<string> {
    const value = await App.ExportDiagnosticPackage();
    return typeof value === 'string' ? value : '';
  }

  async setDebugMode(enabled: boolean): Promise<void> {
    return await App.SetDebugMode(enabled);
  }

  async sendDeviceDebugCommand(hexCommand: string, waitMs = 800): Promise<DeviceDebugCommandResult> {
    return await App.SendDeviceDebugCommand(hexCommand, waitMs);
  }

  async getDeviceDebugFrames(): Promise<DeviceDebugFrame[]> {
    const frames = await App.GetDeviceDebugFrames();
    return Array.isArray(frames) ? frames as DeviceDebugFrame[] : [];
  }

  // ── 自定义主题 ──

  // 列出安装目录/用户目录下发现的全部自定义主题。
  async listThemes(): Promise<ThemeMeta[]> {
    const list = await App.ListThemes();
    return Array.isArray(list) ? (list as ThemeMeta[]) : [];
  }

  // 读取指定主题的 CSS 文本（用于注入页面）。
  async getThemeCSS(id: string): Promise<string> {
    const css = await App.GetThemeCSS(id);
    return typeof css === 'string' ? css : '';
  }

  // 在系统文件管理器中打开主题文件夹，便于用户编辑/新增主题。
  async openThemesFolder(): Promise<void> {
    return await App.OpenThemesFolder();
  }

  async updateGuiResponseTime(): Promise<void> {
    return await App.UpdateGuiResponseTime();
  }

  // 核心记录的时间轴事件（设备断连、睡眠唤醒、切换控温模式等）。
  onTimelineEvent(callback: (event: TimelineEvent) => void): () => void {
    return on('timeline-event', (event) => callback(event as TimelineEvent));
  }

  onCoreServiceError(callback: (message: string) => void): () => void {
    return on('core-service-error', callback);
  }

  onCoreServiceOK(callback: () => void): () => void {
    return on('core-service-ok', callback);
  }

  // core-resynced 由 GUI 侧的 IPC 看护协程在重连成功后发出，提示前端重取核心状态。
  onCoreResynced(callback: () => void): () => void {
    return on('core-resynced', callback);
  }
}

export const apiService = new ApiService();
