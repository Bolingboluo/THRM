'use client';

import { useEffect } from 'react';
import { useAppStore } from '../store/app-store';

/**
 * 温度历史的读取入口。数据本身放在 store 里全应用共用一份：快照只在第一次有组件用到时
 * 下载，之后由实时推送续上。原先每个用到它的组件各拉一份完整快照（24 小时约 1.7 万点），
 * 切一次标签页就重新下载、解析、排序一遍。
 *
 * 核心历史关闭（或读取失败）时返回会话内采样，source 随之变为 'session'。
 */
export function useTemperatureHistory() {
  const enabled = useAppStore((state) => state.historyEnabled);
  const points = useAppStore((state) => (state.historyEnabled ? state.historyPoints : state.sessionHistoryPoints));
  const saving = useAppStore((state) => state.historySaving);
  const retentionHours = useAppStore((state) => state.historyRetentionHours);
  const setEnabled = useAppStore((state) => state.setTemperatureHistoryEnabled);
  const setRetentionHours = useAppStore((state) => state.setTemperatureHistoryRetentionHours);
  const reload = useAppStore((state) => state.loadTemperatureHistory);
  const ensureLoaded = useAppStore((state) => state.ensureTemperatureHistoryLoaded);

  useEffect(() => {
    ensureLoaded();
  }, [ensureLoaded]);

  return {
    points,
    enabled,
    saving,
    retentionHours,
    setEnabled,
    setRetentionHours,
    source: enabled ? 'core' as const : 'session' as const,
    reload,
  };
}
