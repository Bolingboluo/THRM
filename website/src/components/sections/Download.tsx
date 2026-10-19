"use client";

import Link from "next/link";
import { Check, Copy, Download as DownloadIcon, FileArchive, Package, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import type { Dict } from "@/content/types";
import { fmt } from "@/lib/fmt";
import { localePath } from "@/lib/paths";
import { SITE } from "@/lib/site";
import { cn } from "@/lib/utils";
import { resolveAsset, useRelease, type ResolvedAsset } from "../release";
import { useClientOs } from "../useClientOs";
import { buttonClass } from "../ui/button";
import { LinuxIcon, WindowsIcon } from "../ui/icons";

type Tab = "windows" | "linux";

function CopyButton({ text, labels }: { text: string; labels: Dict["download"] }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const id = window.setTimeout(() => setCopied(false), 1600);
    return () => window.clearTimeout(id);
  }, [copied]);

  return (
    <button
      type="button"
      onClick={() => navigator.clipboard?.writeText(text).then(() => setCopied(true), () => undefined)}
      className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md px-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      aria-label={labels.copy}
    >
      {copied ? <Check className="h-3.5 w-3.5 text-primary" /> : <Copy className="h-3.5 w-3.5" />}
      <span aria-live="polite">{copied ? labels.copied : labels.copy}</span>
    </button>
  );
}

function FileRow({
  icon,
  name,
  asset,
  body,
  badge,
  cta,
}: {
  icon: React.ReactNode;
  name: string;
  asset: ResolvedAsset;
  body: string;
  badge?: string;
  cta: string;
}) {
  return (
    <div className="flex flex-col gap-4 rounded-xl border border-border bg-background/50 p-5 sm:flex-row sm:items-center">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">{icon}</span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-semibold">{name}</h3>
          {badge && <span className="rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary">{badge}</span>}
        </div>
        {asset && (
          <div className="mt-1 break-all font-mono text-xs text-muted-foreground">
            {asset.name}
            {asset.size && <span className="ml-2 text-muted-foreground/70">{asset.size}</span>}
          </div>
        )}
        <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{body}</p>
      </div>
      {asset && (
        <a href={asset.url} download className={buttonClass({ variant: badge ? "default" : "outline", className: "shrink-0" })}>
          <DownloadIcon className="h-4 w-4" />
          {cta}
        </a>
      )}
    </div>
  );
}

export function Download({ dict }: { dict: Dict }) {
  const d = dict.download;
  const os = useClientOs();
  const release = useRelease();
  const [tab, setTab] = useState<Tab>("windows");
  const [touched, setTouched] = useState(false);
  // 命令里的下载地址用访客实际访问的域名（反代域名），首帧先用配置的站点地址
  const [origin, setOrigin] = useState<string>(SITE.url);

  useEffect(() => setOrigin(window.location.origin), []);
  useEffect(() => {
    if (!touched) setTab(os === "linux" ? "linux" : "windows");
  }, [os, touched]);

  const select = (t: Tab) => {
    setTouched(true);
    setTab(t);
  };

  const changelog = localePath(dict.locale, "/changelog/");
  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: "windows", label: "Windows", icon: <WindowsIcon className="h-4 w-4" /> },
    { id: "linux", label: "Linux", icon: <LinuxIcon className="h-4 w-4" /> },
  ];

  return (
    <section id="download" className="relative overflow-hidden border-y border-border/70 bg-card/60">
      <div className="blueprint pointer-events-none absolute inset-0 opacity-70" aria-hidden="true" />
      <div className="relative mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-24">
        <div className="flex flex-col gap-6 md:flex-row md:items-end md:justify-between">
          <div className="max-w-2xl">
            <h2 className="text-3xl font-bold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.15]">{d.title}</h2>
          </div>
          <div className="flex gap-2">
            <Link href={`${changelog}#${encodeURIComponent(release.tag)}`} className={buttonClass({ variant: "ghost", size: "sm" })}>
              {d.changelog} <span className="text-xs text-muted-foreground">v{release.version}</span>
            </Link>
            <Link href={changelog} className={buttonClass({ variant: "ghost", size: "sm" })}>
              {d.releases}
            </Link>
          </div>
        </div>

        <div className="hud mt-10 rounded-2xl border border-border bg-card p-4 shadow-sm shadow-black/5 sm:p-6">
          <div role="tablist" aria-label={d.tablist} className="inline-flex gap-1 rounded-lg border border-border bg-muted/60 p-1">
            {tabs.map((t) => (
              <button
                key={t.id}
                type="button"
                role="tab"
                id={`tab-${t.id}`}
                aria-selected={tab === t.id}
                aria-controls={`panel-${t.id}`}
                onClick={() => select(t.id)}
                className={cn(
                  "inline-flex items-center gap-2 rounded-md px-4 py-1.5 text-sm font-medium transition-colors",
                  tab === t.id ? "bg-card text-primary shadow-sm shadow-black/5" : "text-muted-foreground hover:text-foreground",
                )}
              >
                {t.icon}
                {t.label}
              </button>
            ))}
          </div>

          {tab === "windows" ? (
            <div role="tabpanel" id="panel-windows" aria-labelledby="tab-windows" className="mt-5 space-y-3">
              <p className="text-sm text-muted-foreground">{d.windowsReq}</p>
              <FileRow
                icon={<Package className="h-5 w-5" />}
                name={d.installer.name}
                asset={resolveAsset(release, "winInstaller")}
                body={d.installer.body}
                badge={d.recommended}
                cta={d.get}
              />
              <FileRow
                icon={<FileArchive className="h-5 w-5" />}
                name={d.portable.name}
                asset={resolveAsset(release, "winPortable")}
                body={d.portable.body}
                cta={d.get}
              />
              <p className="flex items-start gap-2 pt-2 text-[13px] leading-relaxed text-muted-foreground">
                <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                {d.signing}
              </p>
            </div>
          ) : (
            <div role="tabpanel" id="panel-linux" aria-labelledby="tab-linux" className="mt-5 space-y-3">
              <p className="text-sm text-muted-foreground">{d.linuxReq}</p>
              {d.linux.map((l) => {
                const asset = resolveAsset(release, l.key);
                const command = asset ? `curl -LO ${origin}${asset.url}\n${fmt(l.install, { file: asset.name })}` : null;
                return (
                  <div key={l.key} className="rounded-xl border border-border bg-background/50 p-5">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0">
                        <h3 className="font-semibold">{l.label}</h3>
                        <p className="mt-1 text-sm text-muted-foreground">{l.body}</p>
                      </div>
                      {asset ? (
                        <a href={asset.url} download className={buttonClass({ variant: "outline", size: "sm", className: "shrink-0" })}>
                          <DownloadIcon className="h-3.5 w-3.5" />
                          {d.get}
                          {asset.size && <span className="text-xs text-muted-foreground">{asset.size}</span>}
                        </a>
                      ) : (
                        <Link href={changelog} className={buttonClass({ variant: "outline", size: "sm", className: "shrink-0" })}>
                          {d.releases}
                        </Link>
                      )}
                    </div>
                    {command && (
                      <div className="mt-3 flex items-start gap-2 rounded-lg border border-border/70 bg-muted/50 py-2 pl-4 pr-2">
                        <pre className="min-w-0 flex-1 overflow-x-auto py-1.5 font-mono text-[13px] leading-relaxed">
                          <code>{command}</code>
                        </pre>
                        <CopyButton text={command} labels={d} />
                      </div>
                    )}
                  </div>
                );
              })}
              <p className="pt-2 text-[13px] leading-relaxed text-muted-foreground">{d.linuxDeps}</p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
