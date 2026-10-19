import { Activity, AppWindow, Keyboard, Languages, Layers, Mic, Palette, Power, Rainbow, RefreshCw, Sparkles, Spline } from "lucide-react";
import type { Dict, FeatureKey } from "@/content/types";
import { cn } from "@/lib/utils";
import { LightingVisual, SmartVisual } from "./FeatureVisuals";

const ICONS: Record<FeatureKey, typeof Spline> = {
  curve: Spline,
  smart: Sparkles,
  noise: Mic,
  monitor: Activity,
  lighting: Rainbow,
  desktop: AppWindow,
};

function Card({ k, dict, className, children }: { k: FeatureKey; dict: Dict; className?: string; children?: React.ReactNode }) {
  const Icon = ICONS[k];
  const item = dict.features.items[k];
  return (
    <article className={cn("hud flex flex-col rounded-xl border border-border bg-card p-6 shadow-sm shadow-black/5 sm:p-7", className)}>
      <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
        <Icon className="h-5 w-5" />
      </span>
      <h3 className="mt-5 text-lg font-semibold">{item.title}</h3>
      <p className="mt-2 max-w-[34rem] text-[15px] leading-relaxed text-muted-foreground">{item.body}</p>
      {children && <div className="mt-auto pt-7">{children}</div>}
    </article>
  );
}

/* ── 分时方案：一周的方案排布 ─────────────────────────────────────── */

const WEEKDAY: [number, number][] = [[0, 0], [9, 1], [19, 2]];
const WEEKEND: [number, number][] = [[0, 0], [10, 2]];
const PROFILE_TONE = ["bg-primary/20", "bg-primary/55", "bg-primary"];

function ScheduleVisual({ s }: { s: Dict["features"]["schedule"] }) {
  return (
    <div>
      <div className="space-y-1.5" aria-hidden="true">
        {s.days.map((day, d) => {
          const blocks = d < 5 ? WEEKDAY : WEEKEND;
          return (
            <div key={d} className="flex items-center gap-3">
              <span className="w-4 shrink-0 text-xs text-muted-foreground">{day}</span>
              <div className="flex h-4 flex-1 gap-0.5 overflow-hidden rounded">
                {blocks.map(([start, profile], i) => {
                  const end = blocks[i + 1]?.[0] ?? 24;
                  return <span key={start} className={PROFILE_TONE[profile]} style={{ flexGrow: end - start }} />;
                })}
              </div>
            </div>
          );
        })}
        <div className="flex justify-between pl-7 font-mono text-[10px] text-muted-foreground">
          <span>00:00</span>
          <span>12:00</span>
          <span>24:00</span>
        </div>
      </div>
      <ul className="mt-4 flex flex-wrap gap-2">
        {s.profiles.map((p, i) => (
          <li key={p} className="flex items-center gap-1.5 rounded-full border border-border bg-background/60 px-2.5 py-1 text-xs">
            <i className={cn("h-2 w-2 rounded-full", PROFILE_TONE[i])} />
            {p}
          </li>
        ))}
      </ul>
    </div>
  );
}

/* ── 噪音校准：转速—噪音档案 ─────────────────────────────────────── */

const NOISE = Array.from({ length: 13 }, (_, i) => {
  const rpm = 1000 + i * 250;
  const base = 20 + ((rpm - 1000) / 3000) ** 1.4 * 26;
  const resonance = 5.5 * Math.exp(-(((rpm - 2750) / 180) ** 2));
  return { rpm, db: base + resonance };
});

function NoiseVisual({ n }: { n: Dict["features"]["noise"] }) {
  return (
    <div aria-hidden="true">
      <div className="relative flex h-28 items-end gap-1">
        <div className="absolute inset-y-0 left-[46%] w-[23%] rounded-md border border-dashed border-amber-500/50 bg-amber-500/10">
          <span className="absolute -top-0.5 left-1/2 -translate-x-1/2 -translate-y-full whitespace-nowrap text-[10px] font-medium text-amber-600 dark:text-amber-400">{n.resonance}</span>
        </div>
        {NOISE.map((b) => {
          const inBand = b.rpm >= 2500 && b.rpm <= 3000;
          return (
            <span
              key={b.rpm}
              className={cn("relative flex-1 rounded-t-sm", inBand ? "bg-amber-500" : "bg-foreground/20")}
              style={{ height: `${((b.db - 15) / 40) * 100}%` }}
            />
          );
        })}
      </div>
      <div className="mt-2 flex justify-between font-mono text-[10px] text-muted-foreground">
        <span>1000</span>
        <span>{n.axis}</span>
        <span>4000</span>
      </div>
    </div>
  );
}

/* ── 硬件监控：历史趋势 + RTSS 叠加层 ─────────────────────────────── */

function series(fn: (i: number) => number) {
  return Array.from({ length: 48 }, (_, i) => `${(i * (300 / 47)).toFixed(1)},${fn(i).toFixed(1)}`).join(" ");
}

function MonitorVisual({ m }: { m: Dict["features"]["monitor"] }) {
  return (
    <div>
      <div className="relative overflow-hidden rounded-lg border border-border/70 bg-background/40">
        <svg viewBox="0 0 300 90" preserveAspectRatio="none" className="block h-24 w-full" aria-hidden="true">
          <polyline points={series((i) => 50 - Math.sin(i / 5) * 14 - Math.sin(i / 1.7) * 3)} fill="none" stroke="var(--primary)" strokeWidth="1.75" vectorEffect="non-scaling-stroke" />
          <polyline points={series((i) => 42 - Math.sin(i / 5 - 0.6) * 10 - Math.cos(i / 1.3) * 2)} fill="none" stroke="var(--viz-3)" strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
          <polyline points={series((i) => 62 - Math.sin(i / 5 - 1) * 7 - Math.sin(i / 2.1) * 1.5)} fill="none" stroke="var(--viz-4)" strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
        </svg>
        <div className="absolute right-2 top-2 rounded-md bg-black/75 px-2 py-1 font-mono text-[10px] leading-tight text-white/90 backdrop-blur" title={m.overlay}>
          Cooler Fan: <span className="text-[#ffb347]">2140</span> RPM
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5"><i className="h-1.5 w-3 rounded-full bg-[var(--viz-3)]" />{m.cpu}</span>
        <span className="flex items-center gap-1.5"><i className="h-1.5 w-3 rounded-full bg-[var(--viz-4)]" />{m.gpu}</span>
        <span className="flex items-center gap-1.5"><i className="h-1.5 w-3 rounded-full bg-primary" />{m.fan}</span>
      </div>
    </div>
  );
}

const DESKTOP_ICONS = [Power, Keyboard, Palette, Layers, Languages, RefreshCw];

export function Features({ dict }: { dict: Dict }) {
  const f = dict.features;
  return (
    <section id="features" className="mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-28">
      <div className="max-w-2xl">
        <h2 className="text-3xl font-bold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.15]">{f.title}</h2>
        <p className="mt-4 text-[17px] leading-relaxed text-muted-foreground">{f.lead}</p>
      </div>

      <div className="mt-12 grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card k="curve" dict={dict} className="lg:col-span-2">
          <ScheduleVisual s={f.schedule} />
        </Card>
        <Card k="smart" dict={dict}>
          <SmartVisual labels={f.smart} groupLabel={f.items.smart.title} />
        </Card>
        <Card k="noise" dict={dict}>
          <NoiseVisual n={f.noise} />
        </Card>
        <Card k="monitor" dict={dict}>
          <MonitorVisual m={f.monitor} />
        </Card>
        <Card k="lighting" dict={dict} className="md:col-span-2 lg:col-span-1">
          <LightingVisual modes={f.lighting.modes} groupLabel={f.items.lighting.title} />
        </Card>

        <article className="hud grid gap-8 rounded-xl border border-border bg-card p-6 shadow-sm shadow-black/5 sm:p-7 md:col-span-2 lg:col-span-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
          <div>
            <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
              <AppWindow className="h-5 w-5" />
            </span>
            <h3 className="mt-5 text-lg font-semibold">{f.items.desktop.title}</h3>
            <p className="mt-2 text-[15px] leading-relaxed text-muted-foreground">{f.items.desktop.body}</p>
          </div>
          <ul className="grid content-center gap-x-6 gap-y-4 sm:grid-cols-2">
            {f.desktop.map((item, i) => {
              const Icon = DESKTOP_ICONS[i];
              return (
                <li key={item} className="flex items-start gap-3 text-[15px]">
                  <Icon className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                  {item}
                </li>
              );
            })}
          </ul>
        </article>
      </div>
    </section>
  );
}
