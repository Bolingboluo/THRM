// 文案字典的结构。字典会被直接作为 props 传给客户端组件，所以只能放可序列化的值：
// 需要插值的地方用 {version} / {rpm} 这类占位符，由 fmt() 替换。

export type Locale = "zh-CN" | "en";

export type FeatureKey = "curve" | "smart" | "noise" | "monitor" | "lighting" | "desktop";

export type Dict = {
  locale: Locale;
  htmlLang: string;
  meta: { title: string; description: string };
  nav: {
    features: string;
    devices: string;
    download: string;
    faq: string;
    changelog: string;
    github: string;
    downloadCta: string;
    openMenu: string;
    closeMenu: string;
    switchLanguage: string;
    languageLabel: string;
    theme: { system: string; light: string; dark: string; label: string };
  };
  hero: {
    release: string;
    title: string[];
    lead: string;
    downloadWindows: string;
    downloadLinux: string;
    source: string;
    platforms: string[];
    demoHint: string;
  };
  /** 首页 App 窗口：文案逐字取自桌面端 locales，改动时与 frontend/src/app/locales 保持一致 */
  window: {
    sidebar: { status: string; curve: string; control: string; about: string; expand: string };
    connected: string;
    smartControl: string;
    modeLine: string;
    disconnect: string;
    cpuTemperature: string;
    gpuTemperature: string;
    fanRpm: string;
    tempStatus: { overheat: string; high: string; normal: string; good: string };
    targetSummary: string;
    gears: [string, string, string, string];
    controlProtection: string;
    stats: {
      controlMode: string;
      controlModeValue: string;
      maxRpm: string;
      workMode: string;
      workModeValue: string;
      cpuPower: string;
      gpuPower: string;
    };
    curveTitle: string;
    curveSlider: string;
    historyTitle: string;
    historyBadge: string;
  };
  pipeline: {
    title: string;
    lead: string;
    steps: { title: string; body: string }[];
  };
  features: {
    title: string;
    lead: string;
    items: Record<FeatureKey, { title: string; body: string }>;
    schedule: { days: string[]; profiles: [string, string, string] };
    smart: { tendencies: [string, string, string]; base: string; learned: string };
    noise: { resonance: string; axis: string };
    monitor: { cpu: string; gpu: string; fan: string; overlay: string };
    lighting: { modes: string[] };
    desktop: string[];
  };
  devices: {
    title: string;
    lead: string;
    pairFirst: string;
    autoScan: string;
  };
  quickstart: {
    title: string;
    steps: { title: string; body: string }[];
  };
  download: {
    title: string;
    tablist: string;
    windowsReq: string;
    linuxReq: string;
    recommended: string;
    installer: { name: string; body: string };
    portable: { name: string; body: string };
    linux: { key: "deb" | "arch" | "linuxPortable"; label: string; body: string; install: string }[];
    linuxDeps: string;
    signing: string;
    releases: string;
    changelog: string;
    copy: string;
    copied: string;
    get: string;
  };
  changelog: {
    title: string;
    lead: string;
    latest: string;
    prerelease: string;
    publishedOn: string;
    downloads: string;
    noNotes: string;
    unavailable: string;
    viewOnGithub: string;
  };
  faq: {
    title: string;
    items: { q: string; a: string[] }[];
  };
  footer: {
    disclaimer: string;
    links: { github: string; releases: string; issues: string; privacy: string; signing: string; license: string };
    copyright: string;
  };
};
