export type CurvePoint = { t: number; rpm: number };

// 与 internal/types/types.go 的 GetDefaultFanCurve 保持一致。
export const DEFAULT_CURVE: CurvePoint[] = [
  { t: 30, rpm: 1000 },
  { t: 35, rpm: 1200 },
  { t: 40, rpm: 1400 },
  { t: 45, rpm: 1600 },
  { t: 50, rpm: 1800 },
  { t: 55, rpm: 2000 },
  { t: 60, rpm: 2300 },
  { t: 65, rpm: 2600 },
  { t: 70, rpm: 2900 },
  { t: 75, rpm: 3200 },
  { t: 80, rpm: 3500 },
  { t: 85, rpm: 3800 },
  { t: 90, rpm: 4000 },
  { t: 95, rpm: 4000 },
  { t: 100, rpm: 4000 },
  { t: 105, rpm: 4000 },
  { t: 110, rpm: 4000 },
];

export const T_MIN = 30;
export const T_MAX = 110;
export const RPM_MAX = 4000;

export function rpmAt(curve: CurvePoint[], t: number) {
  if (t <= curve[0].t) return curve[0].rpm;
  for (let i = 1; i < curve.length; i++) {
    const a = curve[i - 1];
    const b = curve[i];
    if (t <= b.t) return a.rpm + ((t - a.t) / (b.t - a.t)) * (b.rpm - a.rpm);
  }
  return curve[curve.length - 1].rpm;
}

export function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}
