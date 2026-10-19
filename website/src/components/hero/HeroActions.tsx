"use client";

import Link from "next/link";
import { Check, Download } from "lucide-react";
import type { Dict, Locale } from "@/content/types";
import { fmt } from "@/lib/fmt";
import { localePath } from "@/lib/paths";
import { SITE } from "@/lib/site";
import { resolveAsset, useRelease } from "../release";
import { useClientOs } from "../useClientOs";
import { buttonClass } from "../ui/button";
import { GithubIcon } from "../ui/icons";

export function ReleaseChip({ template, locale }: { template: string; locale: Locale }) {
  const release = useRelease();
  return (
    <Link
      href={`${localePath(locale, "/changelog/")}#${encodeURIComponent(release.tag)}`}
      className="inline-flex h-7 items-center gap-1.5 rounded-full border border-primary/20 bg-primary/10 px-3 text-xs font-medium text-primary transition-colors hover:bg-primary/15"
    >
      <span className="relative flex h-1.5 w-1.5">
        <span className="absolute inline-flex h-full w-full rounded-full bg-primary opacity-60 motion-safe:animate-ping" />
        <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-primary" />
      </span>
      {fmt(template, { version: release.version })}
    </Link>
  );
}

export function HeroActions({ labels }: { labels: Dict["hero"] }) {
  const os = useClientOs();
  const release = useRelease();
  const installer = resolveAsset(release, "winInstaller");

  return (
    <div className="flex flex-col gap-4 lg:items-end">
      <div className="flex flex-wrap gap-3">
        {os === "linux" || !installer ? (
          <a href="#download" className={buttonClass({ size: "lg" })}>
            <Download className="h-4 w-4" />
            {os === "linux" ? labels.downloadLinux : labels.downloadWindows}
          </a>
        ) : (
          <a href={installer.url} download className={buttonClass({ size: "lg" })}>
            <Download className="h-4 w-4" />
            {labels.downloadWindows}
          </a>
        )}
        <a href={SITE.repo} className={buttonClass({ size: "lg", variant: "outline" })}>
          <GithubIcon className="h-4 w-4" />
          {labels.source}
        </a>
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1.5 text-[13px] text-muted-foreground">
        {labels.platforms.map((p) => (
          <li key={p} className="flex items-center gap-1.5">
            <Check className="h-3.5 w-3.5 text-primary" />
            {p}
          </li>
        ))}
      </ul>
    </div>
  );
}
