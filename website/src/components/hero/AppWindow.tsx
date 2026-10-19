"use client";

import {
  Activity,
  BluetoothConnected,
  ChartLine,
  Cpu,
  Fan,
  Gauge,
  Gpu,
  Info,
  LayoutGrid,
  Minus,
  PanelLeftOpen,
  Power,
  Settings2,
  Sparkles,
  Square,
  Thermometer,
  X,
  Zap,
} from "lucide-react";
import type { Dict } from "@/content/types";
import { DEFAULT_CURVE, RPM_MAX } from "@/lib/curve";
import { fmt } from "@/lib/fmt";
import { asset, cn } from "@/lib/utils";
import { Wordmark } from "../Wordmark";
import { CurveChart } from "./CurveChart";
import { HistoryChart } from "./HistoryChart";
import { useTelemetry } from "./useTelemetry";

/*
 * 首页里的 App 窗口：结构、类名和取值规则逐一对照桌面端源码还原——
 *   外壳 / 侧栏 / 标题栏 / 状态徽章  → frontend/src/app/components/AppShell.tsx
 *   概览、指标卡、控制与保护、曲线、历史 → frontend/src/app/components/DeviceStatus.tsx
 * 呈现的是 Windows 11 默认开启云母材质时的样子（侧栏与标题栏透明，内容面板左上圆角）。
 * 桌面端改了首页布局，这里也要跟着改。
 */

type Labels = Dict["window"];

/* ── 取值规则（与桌面端同名函数一致） ── */

// AppShell.getTempColor：顶栏温度徽章
const titleTempColor = (t: number) => (t > 80 ? "text-red-500" : t > 70 ? "text-amber-500" : "text-primary");

// AppShell.getFanSpinDuration：顶栏转速徽章
const titleSpin = (rpm: number) => (rpm <= 0 ? 0 : rpm >= 4200 ? 0.48 : rpm >= 3200 ? 0.72 : rpm >= 2200 ? 1 : 1.35);

// DeviceStatus.getFanSpinDuration：风扇卡片
const cardSpin = (rpm: number) => (rpm <= 0 ? 0 : rpm >= 4200 ? 0.45 : rpm >= 3200 ? 0.7 : rpm >= 2200 ? 1 : 1.35);

// DeviceStatus.getTempStatus / getTempArcColor
function tempStatus(t: number, l: Labels) {
  if (t > 85) return { color: "text-red-500", arc: "#ef4444", label: l.tempStatus.overheat };
  if (t > 75) return { color: "text-orange-500", arc: "#f97316", label: l.tempStatus.high };
  if (t > 60) return { color: "text-primary", arc: "var(--primary)", label: l.tempStatus.normal };
  return { color: "text-primary", arc: "var(--primary)", label: l.tempStatus.good };
}

// 设备按当前转速上报的挡位（示意）
const gearFor = (rpm: number, l: Labels) => l.gears[rpm < 2000 ? 0 : rpm < 2800 ? 1 : rpm < 3600 ? 2 : 3];

/* ── 组件 ── */

function SpinningFanIcon({ duration, className }: { duration: number; className: string }) {
  return (
    <span className={cn("inline-flex", duration > 0 && "fan-spin")} style={duration > 0 ? { animationDuration: `${duration}s` } : undefined}>
      <Fan className={className} />
    </span>
  );
}

function SidebarNavButton({ icon: Icon, label, active }: { icon: typeof LayoutGrid; label: string; active?: boolean }) {
  return (
    <span
      title={label}
      className={cn("relative flex h-11 w-full items-center rounded-xl", active ? "text-primary" : "text-sidebar-foreground/62")}
    >
      <span className={cn("pointer-events-none absolute inset-y-0 left-2.5 right-2.5 rounded-xl", active && "border border-primary/15 bg-primary/10")} />
      <span className="relative z-10 flex w-16 shrink-0 items-center justify-center">
        <Icon className="h-4.5 w-4.5" />
      </span>
    </span>
  );
}

const chip = "inline-flex h-6 items-center gap-1.5 rounded-full border px-2.5 text-[11px] font-medium";

function StatusBadges({ labels, maxTemp, rpm }: { labels: Labels; maxTemp: number; rpm: number }) {
  return (
    <div className="flex min-w-0 translate-y-px items-center gap-2 overflow-hidden whitespace-nowrap text-[13px] tabular-nums">
      <span className={cn(chip, "status-chip status-chip--tint border-primary/20 bg-primary/10 text-primary")}>
        <BluetoothConnected className="h-3.5 w-3.5" />
        {labels.connected}
      </span>
      <span className={cn(chip, "status-chip status-chip--tint border-primary/20 bg-primary/10 text-primary")}>
        <Sparkles className="h-3.5 w-3.5" />
        {labels.smartControl}
      </span>
      <span className={cn(chip, "status-chip border-border bg-card font-semibold shadow-sm shadow-black/5")}>
        <Thermometer className={cn("h-3.5 w-3.5", titleTempColor(maxTemp))} />
        <span className={titleTempColor(maxTemp)}>{maxTemp}°C</span>
      </span>
      <span className={cn(chip, "status-chip border-border bg-card font-semibold text-primary shadow-sm shadow-black/5")}>
        <SpinningFanIcon duration={titleSpin(rpm)} className="h-3.5 w-3.5" />
        {rpm} RPM
      </span>
    </div>
  );
}

// DeviceStatus.SemiGauge
function SemiGauge({ value, color, children }: { value: number; color: string; children: React.ReactNode }) {
  const r = 84;
  const arc = Math.PI * r;
  const safe = Math.max(0, Math.min(1, value));
  const d = `M ${100 - r} 100 A ${r} ${r} 0 0 1 ${100 + r} 100`;
  return (
    <div className="relative w-full max-w-60">
      <svg viewBox="0 0 200 116" className="block w-full" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
        <path d={d} fill="none" stroke="var(--muted)" strokeWidth="10" strokeLinecap="round" />
        <path
          d={d}
          fill="none"
          stroke={color}
          strokeWidth="10"
          strokeLinecap="round"
          strokeDasharray={arc}
          strokeDashoffset={arc * (1 - safe)}
          style={{ transition: "stroke-dashoffset 600ms cubic-bezier(0.22, 1, 0.36, 1)" }}
        />
      </svg>
      <div className="pointer-events-none absolute inset-x-0 top-[68%] flex -translate-y-1/2 flex-col items-center justify-center">{children}</div>
    </div>
  );
}

function MetricCard({ icon, label, children }: { icon: React.ReactNode; label: string; children: React.ReactNode }) {
  return (
    <div className="flex h-full min-h-[155px] flex-col items-center rounded-xl border border-border bg-card px-5 py-3 shadow-sm shadow-black/5 @min-[768px]:min-h-[171px]">
      <div className="mb-2 flex items-center justify-center">
        <div className="flex min-w-0 max-w-full items-center justify-center gap-2 text-[13px] font-medium text-muted-foreground">
          <span className="shrink-0">{icon}</span>
          <span className="shrink-0">{label}</span>
        </div>
      </div>
      <div className="flex h-full w-full max-w-[20rem] flex-1 flex-col items-center justify-end">{children}</div>
    </div>
  );
}

function TempGauge({ temp, labels }: { temp: number; labels: Labels }) {
  const s = tempStatus(temp, labels);
  return (
    <SemiGauge value={Math.min(1, temp / 100)} color={s.arc}>
      <div className="flex items-baseline gap-0.5">
        <span className={cn("text-[28px] font-bold leading-none tabular-nums tracking-tight", s.color)}>{temp}</span>
        <span className="text-xs font-medium text-muted-foreground">°C</span>
      </div>
      <span className="mt-1 text-[11px] leading-none text-muted-foreground">{s.label}</span>
    </SemiGauge>
  );
}

function StatTile({ icon, label, children }: { icon: React.ReactNode; label: string; children: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-background/55 p-3">
      <div className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        {label}
      </div>
      <div className="truncate text-sm font-semibold">{children}</div>
    </div>
  );
}

function HardwareChip({ icon: Icon, model }: { icon: typeof Cpu; model: string }) {
  return (
    <div className="flex min-w-0 max-w-[18rem] items-center gap-1.5 rounded-full border border-border/70 bg-background/75 px-2.5 py-1 text-[11px] shadow-sm shadow-black/5 backdrop-blur-xl">
      <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate text-foreground/85">{model}</span>
    </div>
  );
}

export function AppWindow({ labels }: { labels: Labels }) {
  const tel = useTelemetry(DEFAULT_CURVE);
  const cpu = Math.round(tel.cpu);
  const gpu = Math.round(tel.gpu);
  const maxTemp = Math.max(cpu, gpu);
  const rpm = Math.round(tel.rpm);
  const target = Math.round(tel.target);
  const cpuPower = Math.round(18 + Math.max(0, tel.cpu - 40) * 2.1);
  const gpuPower = Math.round(30 + Math.max(0, tel.gpu - 38) * 3.2);

  return (
    <div
      className="app-mock @container overflow-hidden rounded-[10px] border border-black/10 text-foreground shadow-[0_40px_80px_-36px_rgba(16,24,40,0.5)] dark:border-white/10 dark:shadow-[0_40px_80px_-30px_rgba(0,0,0,0.9)]"
      aria-label="THRM"
    >
      <div className="app-mock-shell relative flex h-[680px] w-full @min-[768px]:h-auto">
        {/* 侧栏（收起态 64px） */}
        <aside className="flex w-16 shrink-0 flex-col overflow-hidden" aria-hidden="true">
          <div className="flex h-[76px] items-center pl-2">
            <Wordmark className="w-12" />
          </div>
          <nav className="flex flex-1 flex-col gap-1">
            <SidebarNavButton icon={LayoutGrid} label={labels.sidebar.status} active />
            <SidebarNavButton icon={ChartLine} label={labels.sidebar.curve} />
            <SidebarNavButton icon={Settings2} label={labels.sidebar.control} />
          </nav>
          <div className="flex flex-col gap-1 pb-5">
            <SidebarNavButton icon={Info} label={labels.sidebar.about} />
            <SidebarNavButton icon={PanelLeftOpen} label={labels.sidebar.expand} />
          </div>
        </aside>

        <section className="relative flex min-w-0 flex-1 flex-col overflow-hidden">
          {/* 自绘标题栏 */}
          <div className="absolute left-0 right-0 top-0 z-40 flex h-10 items-center pr-32">
            <div className="flex h-full min-w-0 flex-1 items-center px-3 pt-1">
              <StatusBadges labels={labels} maxTemp={maxTemp} rpm={rpm} />
            </div>
          </div>
          <div className="absolute right-0 top-0 z-50 flex h-10 items-center gap-0.5 pr-1 text-muted-foreground" aria-hidden="true">
            <span className="flex h-8 w-10 items-center justify-center rounded-md"><Minus className="h-3.5 w-3.5" /></span>
            <span className="flex h-8 w-10 items-center justify-center rounded-md"><Square className="h-3 w-3" /></span>
            <span className="flex h-8 w-10 items-center justify-center rounded-md"><X className="h-3.5 w-3.5" /></span>
          </div>

          {/* 内容面板 */}
          <div className="app-mock-panel relative mt-10 min-h-0 flex-1 overflow-hidden">
            <div className="app-mock-scroll h-full overflow-y-auto">
              <div className="min-h-full px-4 pb-6 pt-4 @min-[640px]:px-5 @min-[1024px]:px-6">
                <div className="mx-auto w-full max-w-[1120px] min-w-0 px-1 pb-2">
                  <div className="grid grid-cols-1 items-stretch gap-3 @min-[768px]:grid-cols-12">
                    {/* 设备概览 */}
                    <div className="@min-[768px]:col-span-12">
                      <div className="relative overflow-hidden rounded-xl border border-border bg-card p-4 shadow-sm shadow-black/5">
                        <div className="relative z-10 flex flex-wrap items-center justify-between gap-3">
                          <div className="flex items-center gap-3">
                            <div className="flex h-14 w-20 items-center justify-center overflow-hidden rounded-xl bg-muted/45 p-1.5">
                              <img src={asset("/devices/bs2pro.webp")} alt="BS2 PRO" className="h-full w-full object-contain" draggable={false} />
                            </div>
                            <div>
                              <div className="flex items-center gap-2">
                                <span className="text-base font-semibold text-foreground">BS2 PRO</span>
                                <span className="rounded-md bg-primary/10 px-2 py-0.5 text-[11px] font-semibold text-primary">{labels.connected}</span>
                              </div>
                              <div className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                                <Zap className="h-3 w-3 shrink-0 text-primary" />
                                <span>{labels.modeLine}</span>
                              </div>
                            </div>
                          </div>
                          <div className="flex items-center gap-3" aria-hidden="true">
                            <div className="inline-flex min-h-6 items-center gap-3 align-middle">
                              <span className="inline-flex items-center text-sm font-medium leading-none text-muted-foreground">{labels.smartControl}</span>
                              <span className="inline-flex h-6 w-11 shrink-0 items-center rounded-full border-2 border-transparent bg-primary">
                                <span className="block h-5 w-5 translate-x-5 rounded-full bg-background shadow-sm" />
                              </span>
                            </div>
                            <span className="inline-flex h-8 items-center justify-center rounded-lg bg-muted px-3 text-sm font-medium text-foreground">
                              {labels.disconnect}
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>

                    {/* CPU / GPU / 风扇 */}
                    <div className="@min-[768px]:col-span-4">
                      <MetricCard icon={<Cpu className="h-4 w-4" />} label={labels.cpuTemperature}>
                        <TempGauge temp={cpu} labels={labels} />
                      </MetricCard>
                    </div>
                    <div className="@min-[768px]:col-span-4">
                      <MetricCard icon={<Gpu className="h-4 w-4" />} label={labels.gpuTemperature}>
                        <TempGauge temp={gpu} labels={labels} />
                      </MetricCard>
                    </div>
                    <div className="@min-[768px]:col-span-4">
                      <MetricCard icon={<SpinningFanIcon duration={cardSpin(rpm)} className="h-4 w-4" />} label={labels.fanRpm}>
                        <SemiGauge value={rpm / RPM_MAX} color="var(--primary)">
                          <div className="flex items-baseline gap-0.5">
                            <span className="text-[28px] font-bold leading-none tabular-nums tracking-tight text-primary">{rpm}</span>
                            <span className="text-[11px] font-medium text-muted-foreground">RPM</span>
                          </div>
                          <span className="mt-1 max-w-[11rem] truncate text-[11px] leading-none text-muted-foreground">
                            {fmt(labels.targetSummary, { target, gear: gearFor(target, labels) })}
                          </span>
                        </SemiGauge>
                      </MetricCard>
                    </div>

                    {/* 控制与保护 */}
                    <div className="@min-[768px]:col-span-12">
                      <div className="rounded-xl border border-border bg-card p-3 shadow-sm shadow-black/5">
                        <div className="mb-3 flex flex-wrap items-center justify-between gap-2 px-1">
                          <div className="flex items-center gap-2">
                            <Gauge className="h-4 w-4 text-muted-foreground" />
                            <h3 className="text-xs font-semibold text-muted-foreground">{labels.controlProtection}</h3>
                          </div>
                          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
                            <HardwareChip icon={Cpu} model="Intel Core Ultra 9 275HX" />
                            <HardwareChip icon={Gpu} model="NVIDIA GeForce RTX 5070 Ti Laptop GPU" />
                          </div>
                        </div>
                        <div className="grid grid-cols-2 gap-2.5 @min-[560px]:grid-cols-5">
                          <StatTile icon={<Sparkles className="h-3.5 w-3.5" />} label={labels.stats.controlMode}>
                            <span className="text-primary">{labels.stats.controlModeValue}</span>
                          </StatTile>
                          <StatTile icon={<Power className="h-3.5 w-3.5" />} label={labels.stats.maxRpm}>
                            {RPM_MAX} RPM
                          </StatTile>
                          <StatTile icon={<Fan className="h-3.5 w-3.5" />} label={labels.stats.workMode}>
                            {labels.stats.workModeValue}
                          </StatTile>
                          <StatTile icon={<Zap className="h-3.5 w-3.5" />} label={labels.stats.cpuPower}>
                            <span className="tabular-nums">{cpuPower}W</span>
                          </StatTile>
                          <StatTile icon={<Activity className="h-3.5 w-3.5" />} label={labels.stats.gpuPower}>
                            <span className="tabular-nums">{gpuPower}W</span>
                          </StatTile>
                        </div>
                      </div>
                    </div>

                    {/* 风扇转速曲线 */}
                    <div className="@min-[768px]:col-span-7">
                      <div className="flex h-full w-full flex-col rounded-xl border border-border bg-card p-3 text-left shadow-sm shadow-black/5">
                        <div className="mb-2 flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            <div className="text-xs font-semibold text-foreground">{labels.curveTitle}</div>
                            <div className="text-[11px] text-muted-foreground">RPM</div>
                          </div>
                        </div>
                        <div className="aspect-[520/146] w-full overflow-hidden">
                          <CurveChart curve={DEFAULT_CURVE} temperature={maxTemp} onTemperature={tel.setTemperature} label={labels.curveSlider} />
                        </div>
                      </div>
                    </div>

                    {/* 温度与风扇历史 */}
                    <div className="@min-[768px]:col-span-5">
                      <div className="flex h-full min-h-[239px] flex-col rounded-xl border border-border bg-card p-3 shadow-sm shadow-black/5">
                        <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
                          <div className="flex min-w-0 items-center gap-2">
                            <div className="text-xs font-semibold text-foreground">{labels.historyTitle}</div>
                            <span className="rounded-full border border-border/70 bg-background/70 px-2 py-0.5 text-[10px] text-muted-foreground">{labels.historyBadge}</span>
                          </div>
                        </div>
                        <div className="flex min-h-[163px] flex-1 overflow-hidden rounded-lg bg-muted/25 p-2.5">
                          <div className="h-full w-full overflow-hidden">
                            <HistoryChart history={tel.history} />
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}
