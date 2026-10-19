"use client";

import { Download } from "lucide-react";
import { useEffect, useState } from "react";
import type { Dict } from "@/content/types";
import { fmt } from "@/lib/fmt";
import { formatSize } from "@/lib/format";
import { downloadUrl, fetchReleases, type GhRelease } from "@/lib/releases";

type State = { status: "loading" } | { status: "error" } | { status: "ready"; releases: GhRelease[] };

export function ReleaseList({ labels: c, dateLocale }: { labels: Dict["changelog"]; dateLocale: string }) {
  const [state, setState] = useState<State>({ status: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    fetchReleases(controller.signal)
      .then((releases) => setState({ status: "ready", releases }))
      .catch(() => !controller.signal.aborted && setState({ status: "error" }));
    return () => controller.abort();
  }, []);

  // 列表是异步渲染的，带锚点（#v3.7.2）进来时浏览器已经错过了定位，这里补一次
  useEffect(() => {
    if (state.status !== "ready" || !window.location.hash) return;
    document.getElementById(decodeURIComponent(window.location.hash.slice(1)))?.scrollIntoView();
  }, [state.status]);

  if (state.status === "loading") {
    return (
      <div className="mt-14 space-y-6" aria-busy="true">
        {[0, 1].map((i) => (
          <div key={i} className="rounded-xl border border-border bg-card p-6 sm:p-8">
            <div className="h-7 w-32 animate-pulse rounded-md bg-muted" />
            <div className="mt-6 space-y-3">
              <div className="h-4 w-3/4 animate-pulse rounded bg-muted" />
              <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
              <div className="h-4 w-2/3 animate-pulse rounded bg-muted" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (state.status === "error" || state.releases.length === 0) {
    return <p className="mt-12 rounded-xl border border-border bg-card p-6 text-muted-foreground">{c.unavailable}</p>;
  }

  const { releases } = state;
  const latest = releases.find((r) => !r.prerelease) ?? releases[0];
  const date = new Intl.DateTimeFormat(dateLocale, { year: "numeric", month: "long", day: "numeric" });

  return (
    <ol className="mt-14 space-y-6">
      {releases.map((r) => (
        <li key={r.id} id={r.tag_name} className="scroll-mt-24 rounded-xl border border-border bg-card p-6 shadow-sm shadow-black/5 sm:p-8">
          <header className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <h2 className="text-2xl font-bold tracking-[-0.02em]">{r.tag_name}</h2>
            {r.id === latest.id && (
              <span className="rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">{c.latest}</span>
            )}
            {r.prerelease && (
              <span className="rounded-full border border-amber-400/50 bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300">
                {c.prerelease}
              </span>
            )}
            {r.published_at && (
              <time dateTime={r.published_at} className="text-sm text-muted-foreground">
                {fmt(c.publishedOn, { date: date.format(new Date(r.published_at)) })}
              </time>
            )}
          </header>
          {r.name && r.name !== r.tag_name && <p className="mt-1 text-[15px] text-muted-foreground">{r.name}</p>}

          {r.body_html ? (
            // body_html 由 GitHub 渲染并净化，可直接插入
            <div className="release-notes mt-6" dangerouslySetInnerHTML={{ __html: r.body_html }} />
          ) : (
            <p className="mt-6 text-[15px] text-muted-foreground">{c.noNotes}</p>
          )}

          {r.assets.length > 0 && (
            <div className="mt-8 border-t border-border pt-6">
              <h3 className="text-sm font-semibold text-muted-foreground">{c.downloads}</h3>
              <ul className="mt-3 grid gap-2 sm:grid-cols-2">
                {r.assets.map((a) => (
                  <li key={a.id}>
                    <a
                      href={downloadUrl(r.tag_name, a.name)}
                      download
                      className="flex items-center gap-3 rounded-lg border border-border bg-background/50 px-3 py-2.5 text-sm transition-colors hover:border-primary/35 hover:bg-primary/5"
                    >
                      <Download className="h-4 w-4 shrink-0 text-primary" />
                      <span className="min-w-0 flex-1 truncate font-mono text-[13px]">{a.name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">{formatSize(a.size)}</span>
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </li>
      ))}
    </ol>
  );
}
