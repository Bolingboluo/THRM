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

## nginx 配置

```nginx
# GitHub API 短缓存：所有访客共用服务器出口 IP，不缓存很快会触发限流
proxy_cache_path /var/cache/nginx/thrm_api levels=1:2 keys_zone=thrm_api:1m max_size=50m inactive=1d use_temp_path=off;

server {
    listen 443 ssl http2;
    server_name thrm.example.com;
    # ssl_certificate ...;

    resolver 1.1.1.1 8.8.8.8 ipv6=off valid=300s;
    proxy_ssl_server_name on;
    proxy_http_version 1.1;

    # 静态站点：直接放 out/，或者转发到 Vercel / Cloudflare Pages 等静态托管
    root /var/www/thrm;
    location / {
        try_files $uri $uri/ $uri.html =404;
    }

    # ① Release 接口（只读，只限本仓库）
    location /api/github/ {
        limit_except GET HEAD { deny all; }
        proxy_pass https://api.github.com/repos/TIANLI0/THRM/;
        proxy_set_header Host api.github.com;
        proxy_set_header User-Agent "thrm-website";
        # 只读 token（fine-grained，无需任何权限）：限额从 60 次/小时提到 5000 次/小时
        proxy_set_header Authorization "Bearer <GITHUB_TOKEN>";
        proxy_set_header Accept-Encoding "";
        proxy_hide_header Set-Cookie;

        proxy_cache thrm_api;
        proxy_cache_key "$request_uri|$http_accept";
        proxy_cache_valid 200 60s;
        proxy_cache_use_stale error timeout updating http_403 http_429 http_500 http_502 http_503;
        proxy_cache_background_update on;
        proxy_cache_lock on;
        add_header X-Cache $upstream_cache_status;

        # 可选：把返回 JSON 里的 GitHub 下载地址换成本站，桌面端检查更新也能直接用这个接口
        sub_filter_types application/json;
        sub_filter_once off;
        sub_filter "https://github.com/TIANLI0/THRM/releases/download/" "https://$host/download/";
    }

    # ② 最新版下载：GitHub 302 到具体标签，改写成本站地址后交给浏览器
    location ~ ^/download/latest/(?<file>[^/]+)$ {
        proxy_pass https://github.com/TIANLI0/THRM/releases/latest/download/$file;
        proxy_set_header Host github.com;
        proxy_redirect https://github.com/TIANLI0/THRM/releases/download/ /download/;
    }

    # ③ 指定版本下载：GitHub 会 302 到带签名的 release-assets 地址，由服务器跟随并转发文件
    location ~ ^/download/(?<tag>[^/]+)/(?<file>[^/]+)$ {
        proxy_pass https://github.com/TIANLI0/THRM/releases/download/$tag/$file;
        proxy_set_header Host github.com;
        proxy_intercept_errors on;
        recursive_error_pages on;
        error_page 301 302 307 308 = @github_asset;
    }

    location @github_asset {
        set $asset_url $upstream_http_location;
        # 变量形式的 proxy_pass 会自动用目标域名作 Host；客户端的 Range 头默认透传，断点续传可用。
        # 文件名由 GitHub 返回的 Content-Disposition 带回。
        proxy_pass $asset_url;
        proxy_buffering off;
        proxy_read_timeout 300s;
        proxy_hide_header Set-Cookie;
    }
}
```

说明：

- `/api/github/` 缓存 60 秒，访客看到的最新版本最多晚 1 分钟。想立即生效，删掉 `/var/cache/nginx/thrm_api` 里的文件即可。
- `sub_filter` 那段是可选的。启用后，`https://你的域名/api/github/releases/latest` 就是一个可以直接给桌面端用的更新接口：
  把 `frontend/src/app/lib/brand.ts` 里的 `latestReleaseApiUrl` / `releasesApiUrl` 换成这个地址，字段结构不变，下载地址已经是本站。
- 下载文件由服务器流式转发、不落盘。下载量大的话，可以在 ③ 上再加 `slice` + `proxy_cache` 分片缓存。
