"use client";

import { createContext, useContext, useEffect, useState } from "react";
import { formatSize } from "@/lib/format";
import { FALLBACK_RELEASE, fetchLatestRelease, latestDownloadUrl, toPublicRelease, type PublicRelease } from "@/lib/releases";
import { ASSET_MATCH, type AssetKey } from "@/lib/site";

// 静态页面：首帧用内置兜底版本号渲染，挂载后经同源反代实时拉取最新 Release。
// sessionStorage 只用来让同一会话里切页时立即显示上次拿到的结果，随后仍会重新拉取。
const CACHE_KEY = "thrm.site.latest-release";

const ReleaseContext = createContext<PublicRelease>(FALLBACK_RELEASE);

export function ReleaseProvider({ children }: { children: React.ReactNode }) {
  const [release, setRelease] = useState<PublicRelease>(FALLBACK_RELEASE);

  useEffect(() => {
    try {
      const cached = sessionStorage.getItem(CACHE_KEY);
      if (cached) setRelease(JSON.parse(cached) as PublicRelease);
    } catch {
      // 存储不可用时直接等网络结果
    }

    const controller = new AbortController();
    fetchLatestRelease(controller.signal)
      .then((r) => {
        const next = toPublicRelease(r);
        setRelease(next);
        try {
          sessionStorage.setItem(CACHE_KEY, JSON.stringify(next));
        } catch {
          // 忽略
        }
      })
      .catch(() => {
        // 拉不到时保留兜底版本：固定文件名的安装包仍可通过 /download/latest/ 下载
      });
    return () => controller.abort();
  }, []);

  return <ReleaseContext.Provider value={release}>{children}</ReleaseContext.Provider>;
}

export function useRelease() {
  return useContext(ReleaseContext);
}

export type ResolvedAsset = { name: string; url: string; size?: string } | null;

/** 按规则在最新 Release 里找安装包；还没拉到时，文件名固定的包退回 /download/latest/。 */
export function resolveAsset(release: PublicRelease, key: AssetKey): ResolvedAsset {
  const rule = ASSET_MATCH[key];
  const hit = release.assets.find((a) => rule.test(a.name));
  if (hit) return { name: hit.name, url: hit.url, size: formatSize(hit.size) };
  return rule.fallback ? { name: rule.fallback, url: latestDownloadUrl(rule.fallback) } : null;
}
