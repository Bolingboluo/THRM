import type { NextConfig } from "next";

// 纯静态导出（产物在 out/）。版本信息、更新日志和安装包都由浏览器请求同源路径，
// 再由服务器上的反代规则转发到 GitHub（规则见 README.md）：
//   /api/github/...            → https://api.github.com/repos/TIANLI0/THRM/...
//   /download/latest/<文件>    → https://github.com/TIANLI0/THRM/releases/latest/download/<文件>
//   /download/<标签>/<文件>    → https://github.com/TIANLI0/THRM/releases/download/<标签>/<文件>
// 静态导出不支持 rewrites，所以只在 next dev 下用 rewrites 模拟这几条反代。
const isDev = process.env.NODE_ENV === "development";

const nextConfig: NextConfig = {
  output: isDev ? undefined : "export",
  trailingSlash: true,
  images: {
    unoptimized: true,
  },
  experimental: {
    useTypeScriptCli: true,
  },
  ...(isDev && {
    // 否则 trailingSlash 会先把 /api/github/releases/latest 308 到带斜杠的地址，GitHub 不认
    skipTrailingSlashRedirect: true,
    async rewrites() {
      return [
        { source: "/api/github/:path*", destination: "https://api.github.com/repos/TIANLI0/THRM/:path*" },
        { source: "/download/latest/:file", destination: "https://github.com/TIANLI0/THRM/releases/latest/download/:file" },
        { source: "/download/:tag/:file", destination: "https://github.com/TIANLI0/THRM/releases/download/:tag/:file" },
      ];
    },
  }),
};

export default nextConfig;
