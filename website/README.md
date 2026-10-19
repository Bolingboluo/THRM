# THRM 官网

Next.js 16 + Tailwind CSS 4，**纯静态导出**（`bun run build` 产物在 `out/`）。设计令牌、字体（Manrope / HarmonyOS Sans SC / Geist Mono）
与桌面端 `frontend/` 保持一致。首页的 App 窗口按 `frontend/src/app/components/AppShell.tsx` 和 `DeviceStatus.tsx` 还原，桌面端首页改版时
要同步修改 `src/components/hero/AppWindow.tsx`；窗口里的文案在 `src/content/{zh,en}.ts` 的 `window` 段，逐字取自桌面端 locales。

```bash
bun install
bun run dev      # http://localhost:3000，dev 下用 rewrites 模拟下面的反代规则
bun run build    # 输出 out/
```

## 实时版本与下载：同源路径 + 服务器反代

页面是静态的，但版本号、安装包列表和更新日志都在浏览器里**实时**获取，只请求本站域名下的两类路径，
由服务器转发到 GitHub。国内用户全程不需要直连 GitHub，发新版也不用重新构建网站。

| 浏览器请求 | 服务器转发到 |
| --- | --- |
| `/api/github/<路径>` | `https://api.github.com/repos/TIANLI0/THRM/<路径>`（用到 `releases/latest`、`releases?per_page=30`） |
| `/download/latest/<文件名>` | `https://github.com/TIANLI0/THRM/releases/latest/download/<文件名>` |
| `/download/<标签>/<文件名>` | `https://github.com/TIANLI0/THRM/releases/download/<标签>/<文件名>` |

反代没配好（比如直接打开 `out/`）时，页面会退回直连 `api.github.com`，并显示内置兜底版本号（`src/lib/site.ts` 的 `fallbackVersion`）。

两个前缀可以在构建时用环境变量改：`NEXT_PUBLIC_RELEASE_API`（默认 `/api/github`）、`NEXT_PUBLIC_DOWNLOAD_BASE`（默认 `/download`）。
另外建议设置 `NEXT_PUBLIC_SITE_URL=https://你的域名`，用于 Open Graph 分享图的绝对地址。

## 线上部署

```
访客 ──► thrm.tianli0.top（香港服务器 · 宝塔 nginx，只做反代）
            ├─ /api/github/*        ──► api.github.com/repos/TIANLI0/THRM/*（缓存 120 秒）
            ├─ /download/latest/*   ──► GitHub latest/download，302 改写回本站
            ├─ /download/<标签>/*   ──► GitHub releases/download，服务器跟随跳转流式转发
            ├─ /credits.json        ──► 仓库 main 分支的 credits.json（缓存 60 秒）
            └─ 其余页面             ──► thrm.vercel.app（Vercel 项目 thrm）
```

- **Vercel**：项目 `thrm`（团队 Tianli's projects），已连接 `TIANLI0/THRM`，Root Directory 为 `website`，
  生产分支 `main`。改网页后 push 到 main 即自动构建上线；`vercel.json` 的 `ignoreCommand`
  让只改动 App 代码的提交跳过网站构建。环境变量 `NEXT_PUBLIC_SITE_URL=https://thrm.tianli0.top`。
- **服务器**：宝塔站点 `thrm.tianli0.top`，Let's Encrypt 证书由宝塔计划任务自动续签。反代规则写在站点的
  「伪静态」里（`/www/server/panel/vhost/rewrite/thrm.tianli0.top.conf`），仓库里的副本是
  [`deploy/nginx-thrm.tianli0.top.conf`](deploy/nginx-thrm.tianli0.top.conf)，改规则时两边一起改。
  规则全部用 `^~` 前缀匹配，是为了避开宝塔站点配置里默认的 `.js/.css`/图片正则（`.*\.(js|css)?$` 连 `.exe` 都会匹配）。
- `/api/github/` 未带 GitHub token，靠 120 秒缓存把请求压在每小时 60 次的匿名额度内；限流或出错时继续返回旧数据。
  需要更实时的话，在规则里加一行 `proxy_set_header Authorization "Bearer <只读 token>";`，再把缓存时间调短。
- 规则里的 `sub_filter` 会把接口 JSON 里的 GitHub 下载地址换成本站。桌面端（`frontend/src/app/lib/brand.ts`）检查更新、
  下载安装包和读取 `/credits.json` 鸣谢清单时都先走本站，失败再回退 GitHub；Go 端下载失败时也会自动改用 GitHub 直链。
  所以这里的路径和返回格式要保持稳定。
