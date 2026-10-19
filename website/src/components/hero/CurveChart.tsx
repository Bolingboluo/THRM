"use client";

import { useRef } from "react";
import { RPM_MAX, T_MAX, T_MIN, clamp, type CurvePoint } from "@/lib/curve";

// 几何与样式照搬桌面端 DeviceStatus.tsx 的 MiniFanCurveChart（首页「风扇转速曲线」卡片）。
const W = 520;
const H = 146;
const PAD = { left: 44, right: 20, top: 14, bottom: 18 };
const PLOT_W = W - PAD.left - PAD.right;
const PLOT_H = H - PAD.top - PAD.bottom;
const Y_TICKS = [0, 1000, 2000, 3000, 4000];

const x = (t: number) => PAD.left + ((t - T_MIN) / (T_MAX - T_MIN)) * PLOT_W;
const y = (rpm: number) => PAD.top + PLOT_H - (rpm / RPM_MAX) * PLOT_H;

type Props = {
  curve: CurvePoint[];
  temperature: number;
  onTemperature: (t: number) => void;
  label: string;
};

/** 与 App 外观一致，额外加了一点官网专属交互：拖动（或用方向键移动）红色温度线来模拟温度。 */
export function CurveChart({ curve, temperature, onTemperature, label }: Props) {
  const svgRef = useRef<SVGSVGElement>(null);
  const dragging = useRef(false);

  const linePoints = curve.map((p) => `${x(p.t).toFixed(1)},${y(p.rpm).toFixed(1)}`).join(" ");
  const areaPoints = `${PAD.left},${PAD.top + PLOT_H} ${linePoints} ${PAD.left + PLOT_W},${PAD.top + PLOT_H}`;
  const t = clamp(temperature, T_MIN, T_MAX);
  const currentX = clamp(x(t), PAD.left, PAD.left + PLOT_W);

  const fromPointer = (clientX: number) => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    const vx = ((clientX - rect.left) / rect.width) * W;
    onTemperature(T_MIN + ((vx - PAD.left) / PLOT_W) * (T_MAX - T_MIN));
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const step = e.key === "PageUp" || e.key === "PageDown" ? 5 : 1;
    if (e.key === "ArrowRight" || e.key === "ArrowUp" || e.key === "PageUp") onTemperature(t + step);
    else if (e.key === "ArrowLeft" || e.key === "ArrowDown" || e.key === "PageDown") onTemperature(t - step);
    else if (e.key === "Home") onTemperature(30);
    else if (e.key === "End") onTemperature(100);
    else return;
    e.preventDefault();
  };

  return (
    <svg
      ref={svgRef}
      viewBox={`0 0 ${W} ${H}`}
      className="h-full w-full cursor-ew-resize touch-pan-y select-none rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      preserveAspectRatio="xMidYMid meet"
      role="slider"
      tabIndex={0}
      aria-label={label}
      aria-valuemin={30}
      aria-valuemax={100}
      aria-valuenow={Math.round(t)}
      aria-valuetext={`${Math.round(t)}°C`}
      onKeyDown={onKeyDown}
      onPointerDown={(e) => {
        dragging.current = true;
        e.currentTarget.setPointerCapture(e.pointerId);
        fromPointer(e.clientX);
      }}
      onPointerMove={(e) => dragging.current && fromPointer(e.clientX)}
      onPointerUp={() => (dragging.current = false)}
      onPointerCancel={() => (dragging.current = false)}
    >
      {Y_TICKS.map((tick) => (
        <g key={tick}>
          <line x1={PAD.left} y1={y(tick)} x2={PAD.left + PLOT_W} y2={y(tick)} stroke="var(--chart-grid)" strokeWidth="1" />
          <text x={PAD.left - 8} y={y(tick) + 4} textAnchor="end" fontSize="10" fill="var(--chart-tick)">
            {tick}
          </text>
        </g>
      ))}
      <polygon points={areaPoints} fill="var(--primary)" opacity="0.14" />
      <polyline points={linePoints} fill="none" stroke="var(--primary)" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
      <line
        x1={currentX}
        y1={PAD.top}
        x2={currentX}
        y2={PAD.top + PLOT_H}
        stroke="var(--chart-temperature-indicator)"
        strokeWidth="1.5"
        strokeDasharray="4 4"
        opacity="0.9"
      />
      <text x={PAD.left} y={H - 7} fontSize="10" fill="var(--chart-tick)">
        {T_MIN}
      </text>
      <text x={PAD.left + PLOT_W} y={H - 7} textAnchor="end" fontSize="10" fill="var(--chart-tick)">
        {T_MAX} °C
      </text>
    </svg>
  );
}
