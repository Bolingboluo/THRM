import localFont from "next/font/local";
import "@fontsource-variable/manrope/index.css";
import "@fontsource-variable/geist-mono/index.css";

// 西文走 Manrope，中文走 HarmonyOS Sans SC 子集（直接复用桌面端 frontend/public/fonts 的同一份子集）。
// 更新日志正文来自 GitHub，字表不固定，所以不按站点文案再裁剪。
export const harmony = localFont({
  src: [
    { path: "../fonts/HarmonyOSSansSC-Regular.subset.woff2", weight: "400", style: "normal" },
    { path: "../fonts/HarmonyOSSansSC-Medium.subset.woff2", weight: "500 600", style: "normal" },
    { path: "../fonts/HarmonyOSSansSC-Bold.subset.woff2", weight: "700 800", style: "normal" },
  ],
  variable: "--font-harmony",
  display: "swap",
  preload: false,
});
