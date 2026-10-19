"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { clamp, rpmAt, type CurvePoint } from "@/lib/curve";

export type Sample = { cpu: number; gpu: number; rpm: number };

export type Telemetry = {
  cpu: number;
  gpu: number;
  /** 当前转速：按升降速限制逐步逼近 target，和 App 里「当前 / 目标」两个读数一致 */
  rpm: number;
  target: number;
  history: Sample[];
};

const TICK_MS = 500;
const HISTORY_LEN = 96;
const MANUAL_HOLD_MS = 6000;
const RAMP_UP = 160;
const RAMP_DOWN = 90;

// 首屏历史用确定性数据预填，保证服务端和客户端首帧一致、图表不是空的。
function seedHistory(curve: CurvePoint[]): Sample[] {
  return Array.from({ length: HISTORY_LEN }, (_, i) => {
    const cpu = 61 + Math.sin(i / 7) * 4 + Math.sin(i / 2.3) * 1.2;
    const gpu = cpu * 0.72 + 4 + Math.sin(i / 3.1) * 0.8;
    return { cpu, gpu, rpm: rpmAt(curve, Math.max(cpu, gpu)) };
  });
}

/**
 * 模拟一台正在跑负载的游戏本：CPU 温度在随机切换的负载目标之间漂移，
 * 风扇转速按曲线插值后再做升降速限制。用户拖动温度时接管 CPU 温度若干秒。
 */
export function useTelemetry(curve: CurvePoint[]) {
  const [state, setState] = useState<Telemetry>(() => {
    const history = seedHistory(curve);
    const last = history[history.length - 1];
    return { ...last, target: last.rpm, history };
  });
  const manual = useRef<{ t: number; until: number } | null>(null);
  const load = useRef({ target: 66, next: 0 });

  const setTemperature = useCallback(
    (t: number) => {
      const value = clamp(t, 30, 100);
      manual.current = { t: value, until: performance.now() + MANUAL_HOLD_MS };
      setState((prev) => ({ ...prev, cpu: value, target: rpmAt(curve, Math.max(value, prev.gpu)) }));
    },
    [curve],
  );

  useEffect(() => {
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const id = window.setInterval(() => {
      if (document.hidden) return;
      setState((prev) => {
        const now = performance.now();
        let cpu = prev.cpu;
        const m = manual.current;
        if (m && now < m.until) {
          cpu = m.t;
        } else if (!reduced) {
          if (now > load.current.next) {
            load.current = { target: 50 + Math.random() * 34, next: now + 5000 + Math.random() * 6000 };
          }
          cpu += (load.current.target - cpu) * 0.1 + (Math.random() - 0.5) * 1.1;
        }
        const gpuTarget = cpu * 0.72 + 4;
        const gpu = prev.gpu + (gpuTarget - prev.gpu) * 0.2 + (reduced ? 0 : (Math.random() - 0.5) * 0.6);
        const target = rpmAt(curve, Math.max(cpu, gpu));
        const rpm = prev.rpm + clamp(target - prev.rpm, -RAMP_DOWN, RAMP_UP);
        const history = [...prev.history.slice(1 - HISTORY_LEN), { cpu, gpu, rpm }];
        return { cpu, gpu, rpm, target, history };
      });
    }, TICK_MS);
    return () => window.clearInterval(id);
  }, [curve]);

  return { ...state, setTemperature };
}
