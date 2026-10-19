"use client";

import { useState } from "react";
import type { Dict } from "@/content/types";
import { DEFAULT_CURVE } from "@/lib/curve";
import { cn } from "@/lib/utils";

function Segmented({
  options,
  value,
  onChange,
  label,
  className,
}: {
  options: string[];
  value: number;
  onChange: (i: number) => void;
  label: string;
  className?: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className={cn("inline-flex flex-wrap gap-1 rounded-lg border border-border bg-muted/60 p-1", className)}>
      {options.map((o, i) => (
        <button
          key={o}
          type="button"
          role="radio"
          aria-checked={value === i}
          onClick={() => onChange(i)}
          className={cn(
            "rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
            value === i ? "bg-card text-primary shadow-sm shadow-black/5" : "text-muted-foreground hover:text-foreground",
          )}
        >
          {o}
        </button>
      ))}
    </div>
  );
}

/* ── SmartControl：三种学习倾向对曲线的修正 ─────────────────────────── */

const SW = 300;
const SH = 130;
const sx = (t: number) => 6 + ((t - 30) / 60) * (SW - 12);
const sy = (rpm: number) => 8 + (1 - rpm / 4200) * (SH - 16);
const bump = (t: number, c: number, w: number) => Math.exp(-(((t - c) / w) ** 2));

// 示意用的修正量：散热优先整体前移，静音优先压低中段，均衡只做小幅修整。
const TENDENCY = [
  (t: number) => -240 * bump(t, 50, 8) + 170 * bump(t, 76, 7),
  (t: number) => 320 * bump(t, 62, 12) + 160 * bump(t, 80, 7),
  (t: number) => -380 * bump(t, 55, 10) - 120 * bump(t, 72, 6),
];

const smartPoints = DEFAULT_CURVE.filter((p) => p.t <= 90);
const toPath = (f: (t: number) => number) =>
  smartPoints.map((p, i) => `${i ? "L" : "M"}${sx(p.t).toFixed(1)},${sy(Math.min(4000, Math.max(800, p.rpm + f(p.t)))).toFixed(1)}`).join(" ");
const BASE_PATH = toPath(() => 0);
const LEARNED_PATHS = TENDENCY.map(toPath);

export function SmartVisual({ labels, groupLabel }: { labels: Dict["features"]["smart"]; groupLabel: string }) {
  const [mode, setMode] = useState(0);
  return (
    <div>
      <Segmented options={labels.tendencies} value={mode} onChange={setMode} label={groupLabel} />
      <svg viewBox={`0 0 ${SW} ${SH}`} className="mt-4 block h-auto w-full" aria-hidden="true">
        {[0.33, 0.66].map((f) => (
          <line key={f} x1="0" x2={SW} y1={SH * f} y2={SH * f} stroke="var(--chart-grid)" />
        ))}
        <path d={BASE_PATH} fill="none" stroke="var(--muted-foreground)" strokeOpacity="0.55" strokeWidth="1.5" strokeDasharray="4 4" />
        <path
          className="morph-path"
          style={{ d: `path("${LEARNED_PATHS[mode]}")` } as React.CSSProperties}
          d={LEARNED_PATHS[mode]}
          fill="none"
          stroke="var(--chart-adaptive)"
          strokeWidth="2.25"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      <div className="mt-2 flex gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5"><i className="h-0 w-4 border-t-[1.5px] border-dashed border-muted-foreground/70" />{labels.base}</span>
        <span className="flex items-center gap-1.5"><i className="h-[2px] w-4 rounded-full bg-[var(--chart-adaptive)]" />{labels.learned}</span>
      </div>
    </div>
  );
}

/* ── RGB 灯效：六种模式的灯条预览 ─────────────────────────────────── */

const LEDS = 18;

function ledColor(mode: number, i: number) {
  const f = i / (LEDS - 1);
  switch (mode) {
    case 0: // 智能温控：从低温蓝到高温橙
      return `hsl(${210 - f * 190} 90% 58%)`;
    case 2:
    case 3:
      return `hsl(${Math.round(f * 330)} 90% 60%)`;
    default:
      return "var(--primary)";
  }
}

export function LightingVisual({ modes, groupLabel }: { modes: string[]; groupLabel: string }) {
  const [mode, setMode] = useState(3);
  return (
    <div>
      <div className="relative overflow-hidden rounded-full bg-[#0b0f15] p-2 ring-1 ring-white/5">
        <div className={cn("flex gap-1", mode === 3 && "animate-led-rotate", mode === 5 && "animate-led-breathe", mode === 4 && "opacity-35")}>
          {Array.from({ length: LEDS }, (_, i) => {
            const c = ledColor(mode, i);
            return <span key={i} className="h-2 flex-1 rounded-full" style={{ background: c, boxShadow: `0 0 10px ${c}` }} />;
          })}
        </div>
        {mode === 4 && (
          // 流光：在暗下去的灯条上叠一排全亮的灯，用移动的遮罩只露出一段
          <div className="flow-mask animate-led-flow pointer-events-none absolute inset-2 flex gap-1">
            {Array.from({ length: LEDS }, (_, i) => (
              <span key={i} className="h-2 flex-1 rounded-full bg-primary" style={{ boxShadow: "0 0 10px var(--primary)" }} />
            ))}
          </div>
        )}
      </div>
      <div className="mt-4">
        <Segmented options={modes} value={mode} onChange={setMode} label={groupLabel} className="grid grid-cols-3" />
      </div>
    </div>
  );
}
