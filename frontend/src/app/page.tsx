'use client';

import { useCallback, useMemo } from 'react';
import dynamic from 'next/dynamic';
import * as types from '@go/types';
import { useShallow } from 'zustand/react/shallow';
import AppFatalError from './components/AppFatalError';
import AppLoadingSkeleton from './components/AppLoadingSkeleton';
import AppShell from './components/AppShell';
// 首页（默认标签页）随主包一起加载：用 next/dynamic 的话它不在 index.html 里，
// 冷启动时要等骨架屏结束后再多拉一次 chunk，界面会先空一下。
import DeviceStatus from './components/DeviceStatus';
import { useAppBootstrap } from './hooks/useAppBootstrap';
import { useCoreEventSubscription } from './hooks/useCoreEventSubscription';
import { useAppStore } from './store/app-store';
import { i18n, useLocale } from './lib/i18n';

const FanCurve = dynamic(() => import('./components/FanCurve'), { ssr: false });
const ControlPanel = dynamic(() => import('./components/ControlPanel'), { ssr: false });
const AboutPanel = dynamic(() => import('./components/AboutPanel'), { ssr: false });

// 遥测（fanData 每秒最多 4 次、temperature 约 2 秒一次）只在用得到的标签页里订阅，
// 不放进 Home 的 selector：否则每次推送都会让 Home → AppShell → 整个标签页一起重渲染。
// 三个标签页都在组件内部按需订阅各自显示的数值，整页只随这些数值之外的慢变化重渲染。

export default function Home() {
  useAppBootstrap();
  useCoreEventSubscription();

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

  // 非默认语言的词条是按需加载的：加载完之前继续显示骨架屏，避免先闪一下中文。
  const { localeReady } = useLocale();

  if (view.isLoading || !localeReady) {
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
        <DeviceStatus
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
        <FanCurve
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
        <ControlPanel
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
