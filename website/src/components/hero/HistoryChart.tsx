import type { Sample } from "./useTelemetry";

// 照搬桌面端 DeviceStatus.tsx 的 TemperatureHistoryPanel：温度自适应纵轴，转速单独按 0–最大转速投影，
// 三条线的颜色在 App 里是写死的（CPU 蓝、GPU 橙、风扇绿），深浅色主题都不变。
const W = 520;
const H = 168;
const PAD = { left: 8, right: 8, top: 10, bottom: 10 };
const PLOT_W = W - PAD.left - PAD.right;
const PLOT_H = H - PAD.top - PAD.bottom;

export function HistoryChart({ history }: { history: Sample[] }) {
  let minTemp = 35;
  let maxTemp = 80;
  let maxFan = 4000;
  for (const p of history) {
    minTemp = Math.min(minTemp, p.cpu, p.gpu);
    maxTemp = Math.max(maxTemp, p.cpu, p.gpu);
    maxFan = Math.max(maxFan, p.rpm);
  }
  const minY = Math.max(0, Math.floor((minTemp - 6) / 5) * 5);
  const maxY = Math.min(110, Math.ceil((maxTemp + 6) / 5) * 5);
  const rangeY = Math.max(10, maxY - minY);

  const xFor = (i: number) => PAD.left + (i / Math.max(1, history.length - 1)) * PLOT_W;
  const yTemp = (v: number) => PAD.top + PLOT_H - ((v - minY) / rangeY) * PLOT_H;
  const yFan = (v: number) => PAD.top + PLOT_H - (v / maxFan) * PLOT_H;
  const path = (pick: (s: Sample) => number, project: (v: number) => number) =>
    history.map((s, i) => `${i ? "L" : "M"} ${xFor(i).toFixed(1)} ${project(pick(s)).toFixed(1)}`).join(" ");

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="h-full w-full" preserveAspectRatio="none" aria-hidden="true">
      {[0.2, 0.5, 0.8].map((r) => (
        <line key={r} x1={PAD.left} y1={PAD.top + PLOT_H * r} x2={PAD.left + PLOT_W} y2={PAD.top + PLOT_H * r} stroke="var(--chart-grid)" strokeWidth="1" opacity="0.7" />
      ))}
      <path d={path((s) => s.cpu, yTemp)} fill="none" stroke="#2f6df6" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d={path((s) => s.gpu, yTemp)} fill="none" stroke="#f97316" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d={path((s) => s.rpm, yFan)} fill="none" stroke="#10b981" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
