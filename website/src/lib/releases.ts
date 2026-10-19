import { SITE } from "./site";

// 浏览器端读取 GitHub Release。默认走同源反代路径，反代没配好（或本地直接打开 out/）时退回直连 GitHub。
// 两个前缀可在构建时用环境变量改，例如反代路径不同的时候。
export const RELEASE_API = (process.env.NEXT_PUBLIC_RELEASE_API || "/api/github").replace(/\/$/, "");
export const DOWNLOAD_BASE = (process.env.NEXT_PUBLIC_DOWNLOAD_BASE || "/download").replace(/\/$/, "");
const DIRECT_API = `https://api.github.com/repos/${SITE.repoSlug}`;

type GhAsset = { id: number; name: string; size: number; browser_download_url: string };

export type GhRelease = {
  id: number;
  tag_name: string;
  name: string | null;
  html_url: string;
  published_at: string | null;
  prerelease: boolean;
  draft: boolean;
  /** GitHub 渲染并净化过的 Markdown（请求头 Accept: full+json 时返回） */
  body_html?: string | null;
  assets: GhAsset[];
};

async function getJson<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { signal, headers: { Accept: "application/vnd.github.full+json" } });
  if (!res.ok) throw new Error(`${res.status} ${url}`);
  return (await res.json()) as T;
}

export async function fetchGithub<T>(path: string, signal?: AbortSignal): Promise<T> {
  try {
    return await getJson<T>(`${RELEASE_API}${path}`, signal);
  } catch (err) {
    if (signal?.aborted || RELEASE_API === DIRECT_API) throw err;
    return getJson<T>(`${DIRECT_API}${path}`, signal);
  }
}

export const fetchLatestRelease = (signal?: AbortSignal) => fetchGithub<GhRelease>("/releases/latest", signal);

export async function fetchReleases(signal?: AbortSignal) {
  const list = await fetchGithub<GhRelease[]>("/releases?per_page=30", signal);
  return list.filter((r) => !r.draft);
}

/** 站内下载地址（由反代转发到 GitHub 的 releases/download） */
export const downloadUrl = (tag: string, name: string) => `${DOWNLOAD_BASE}/${encodeURIComponent(tag)}/${encodeURIComponent(name)}`;

/** GitHub 的 latest/download 会跳到最新版本里的同名文件，适合文件名不带版本号的安装包 */
export const latestDownloadUrl = (name: string) => `${DOWNLOAD_BASE}/latest/${encodeURIComponent(name)}`;

export type PublicAsset = { name: string; size: number; url: string };

export type PublicRelease = {
  tag: string;
  version: string;
  assets: PublicAsset[];
  /** false：还没拉到（或拉取失败），正在用内置的兜底版本号 */
  live: boolean;
};

export function toPublicRelease(r: GhRelease): PublicRelease {
  return {
    tag: r.tag_name,
    version: r.tag_name.replace(/^v/, ""),
    assets: r.assets.map((a) => ({ name: a.name, size: a.size, url: downloadUrl(r.tag_name, a.name) })),
    live: true,
  };
}

export const FALLBACK_RELEASE: PublicRelease = {
  tag: `v${SITE.fallbackVersion}`,
  version: SITE.fallbackVersion,
  assets: [],
  live: false,
};
