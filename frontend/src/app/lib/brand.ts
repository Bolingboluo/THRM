// 官网 thrm.tianli0.top 由香港服务器反代：/api/github → GitHub API、/download → Release 安装包、
// /credits.json → 仓库里的鸣谢清单。国内用户直连 GitHub 经常超时，所以版本检查、安装包下载和鸣谢清单
// 都先走官网，失败再回退 GitHub。反代规则见 website/deploy/nginx-thrm.tianli0.top.conf。
const WEBSITE_URL = 'https://thrm.tianli0.top';
const GITHUB_API_URL = 'https://api.github.com/repos/TIANLI0/THRM';

export const BRAND = {
  name: 'THRM',
  legacyName: 'BS2PRO Controller',
  description: '面向飞智笔记本压风散热器系列设备的第三方驱动与控制工具',
  websiteUrl: WEBSITE_URL,
  repositoryUrl: 'https://github.com/TIANLI0/THRM',
  /** 手动下载 / 查看更新日志时打开的页面（官网更新日志，带各版本安装包） */
  latestReleaseUrl: `${WEBSITE_URL}/changelog/`,
  /** 以下地址按顺序尝试，前一个失败（超时、非 2xx）再用下一个 */
  latestReleaseApiUrls: [`${WEBSITE_URL}/api/github/releases/latest`, `${GITHUB_API_URL}/releases/latest`],
  releasesApiUrls: [`${WEBSITE_URL}/api/github/releases`, `${GITHUB_API_URL}/releases`],
  creditsUrls: [`${WEBSITE_URL}/credits.json`, 'https://raw.githubusercontent.com/TIANLI0/THRM/main/credits.json'],
} as const;

/** 官网更新日志里某个版本的锚点地址 */
export function websiteChangelogUrl(tag: string) {
  return tag ? `${BRAND.latestReleaseUrl}#${encodeURIComponent(tag)}` : BRAND.latestReleaseUrl;
}
