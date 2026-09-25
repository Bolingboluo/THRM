'use client';

import { useCallback, useMemo, type ComponentProps } from 'react';
import dynamic from 'next/dynamic';
import { types } from '../../wailsjs/go/models';
import { useShallow } from 'zustand/react/shallow';
import AppFatalError from './components/AppFatalError';
import AppLoadingSkeleton from './components/AppLoadingSkeleton';
import AppShell from './components/AppShell';
import { useAppBootstrap } from './hooks/useAppBootstrap';
import { useAppStore } from './store/app-store';
import { i18n } from './lib/i18n';

const DeviceStatus = dynamic(() => import('./components/DeviceStatus'), { ssr: false });
const FanCurve = dynamic(() => import('./components/FanCurve'), { ssr: false });
const ControlPanel = dynamic(() => import('./components/ControlPanel'), { ssr: false });
const AboutPanel = dynamic(() => import('./components/AboutPanel'), { ssr: false });

// 遥测（fanData 每秒最多 4 次、temperature 约 2 秒一次）只在用得到的标签页里订阅，
// 不放进 Home 的 selector：否则每次推送都会让 Home → AppShell → 整个标签页一起重渲染。

// 首页就是实时仪表盘，随遥测重渲染是预期行为。
function StatusTab(props: Omit<ComponentProps<typeof DeviceStatus>, 'fanData' | 'temperature'>) {
  const { fanData, temperature } = useAppStore(
    useShallow((state) => ({ fanData: state.fanData, temperature: state.temperature })),
  );
  return <DeviceStatus {...props} fanData={fanData} temperature={temperature} />;
}

// FanCurve 不读 fanData，固定传 null，让它的 memo 挡住转速推送，只随温度刷新。
function CurveTab(props: Omit<ComponentProps<typeof FanCurve>, 'fanData' | 'temperature'>) {
  const temperature = useAppStore((state) => state.temperature);
  return <FanCurve {...props} fanData={null} temperature={temperature} />;
}

// 设置页的转速概览由 ControlPanel 内部自行订阅，这里只需要温度（传感器列表等）。
function ControlTab(props: Omit<ComponentProps<typeof ControlPanel>, 'temperature'>) {
  const temperature = useAppStore((state) => state.temperature);
  return <ControlPanel {...props} temperature={temperature} />;
}

export default function Home() {
  useAppBootstrap();

  const view = useAppStore(
    useShallow((state) => ({
      isConnected: state.isConnected,
      deviceProductId: state.deviceProductId,
      deviceModel: state.deviceModel,
      deviceSettings: state.deviceSettings,
      config: state.config,
      legionFnQSupported: state.legionFnQSupported,
      bridgeWarning: state.bridgeWarning,
      coreServiceError: state.coreServiceError,
      isLoading: state.isLoading,
      error: state.error,
      activeTab: state.activeTab,
      curveFocusTarget: state.curveFocusTarget,
      timelineEvents: state.timelineEvents,
    })),
  );

  const initializeApp = useAppStore((state) => state.initializeApp);
  const connectDevice = useAppStore((state) => state.connectDevice);
  const disconnectDevice = useAppStore((state) => state.disconnectDevice);
  const setConfig = useAppStore((state) => state.setConfig);
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const openCurveTab = useAppStore((state) => state.openCurveTab);
  const clearCurveFocusTarget = useAppStore((state) => state.clearCurveFocusTarget);
  const clearBridgeWarning = useAppStore((state) => state.clearBridgeWarning);

  const openCurveEditor = useCallback(() => openCurveTab('curve-editor'), [openCurveTab]);
  const openHistoryDetails = useCallback(() => openCurveTab('history-details'), [openCurveTab]);

  const safeConfig = useMemo(
    () => view.config || new types.AppConfig(),
    [view.config],
  );

  if (view.isLoading) {
    return <AppLoadingSkeleton />;
  }

  // 没拿到核心配置就不能渲染可编辑界面：页面会退回全零的 new AppConfig()，
  // 用户随手改一个开关就会把这份零值配置整份写回核心。error 可能已被后续的
  // device-connected 等事件清空，所以只看 config。
  if (!view.config) {
    return <AppFatalError message={view.error || i18n.t('store.errors.initializeApp')} onRetry={initializeApp} />;
  }

  return (
    <AppShell
      activeTab={view.activeTab}
      onTabChange={setActiveTab}
      isConnected={view.isConnected}
      autoControl={safeConfig.autoControl}
      error={view.error}
      bridgeWarning={view.bridgeWarning}
      onDismissBridgeWarning={clearBridgeWarning}
      statusContent={
        <StatusTab
          isConnected={view.isConnected}
          deviceProductId={view.deviceProductId}
          deviceModel={view.deviceModel}
          deviceSettings={view.deviceSettings}
          config={safeConfig}
          coreServiceError={view.coreServiceError}
          onConnect={connectDevice}
          onDisconnect={disconnectDevice}
          onConfigChange={setConfig}
          onOpenCurveEditor={openCurveEditor}
          onOpenHistoryDetails={openHistoryDetails}
        />
      }
      curveContent={
        <CurveTab
          config={safeConfig}
          onConfigChange={setConfig}
          isConnected={view.isConnected}
          deviceModel={view.deviceModel}
          focusTarget={view.curveFocusTarget}
          onFocusHandled={clearCurveFocusTarget}
          timelineEvents={view.timelineEvents}
        />
      }
      controlContent={
        <ControlTab
          config={safeConfig}
          onConfigChange={setConfig}
          isConnected={view.isConnected}
          legionFnQSupported={view.legionFnQSupported}
          deviceModel={view.deviceModel}
          deviceSettings={view.deviceSettings}
        />
      }
      aboutContent={<AboutPanel />}
    />
  );
}
