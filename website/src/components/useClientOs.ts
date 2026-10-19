"use client";

import { useEffect, useState } from "react";

export type ClientOs = "windows" | "linux" | "other";

/** 首帧统一按 Windows 渲染（与静态 HTML 一致），挂载后再按 UA 修正。 */
export function useClientOs(): ClientOs {
  const [os, setOs] = useState<ClientOs>("windows");
  useEffect(() => {
    const ua = navigator.userAgent;
    if (/Windows/i.test(ua)) setOs("windows");
    else if (/Linux|X11/i.test(ua) && !/Android/i.test(ua)) setOs("linux");
    else setOs("other");
  }, []);
  return os;
}
