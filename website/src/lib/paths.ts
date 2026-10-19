import type { Locale } from "@/content/types";

/** 站内页面路径：中文在根路径，英文在 /en/ 下。静态导出开启了 trailingSlash，路径统一以 / 结尾。 */
export function localePath(locale: Locale, path: "/" | `/${string}/`) {
  return locale === "en" ? `/en${path}` : path;
}

/** 去掉语言前缀，得到与语言无关的页面路径（用于切换语言时留在同一页） */
export function neutralPath(pathname: string): "/" | `/${string}/` {
  const p = pathname.replace(/^\/en(?=\/|$)/, "") || "/";
  return (p.endsWith("/") ? p : `${p}/`) as "/" | `/${string}/`;
}
