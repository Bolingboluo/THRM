// 由 scripts/gen-oss-notices.mjs 生成，请勿手改。
// 依赖变动后重新运行： node scripts/gen-oss-notices.mjs
//
// 只收录随发行产物一起分发的组件：Go 侧取真正链接进二进制的模块，前端取
// dependencies。仅在构建期使用、不进产物的工具链（TypeScript、类型声明包等）不列入。

export type OpenSourceGroupId = 'framework' | 'frontend' | 'device' | 'backend' | 'fonts';

export interface OpenSourceNotice {
  name: string;
  version: string;
  /** SPDX 标识，由各组件自带的 LICENSE 正文判定。 */
  license: string;
  url: string;
}

export interface OpenSourceGroup {
  id: OpenSourceGroupId;
  items: OpenSourceNotice[];
}

export const OPEN_SOURCE_GROUPS: OpenSourceGroup[] = [
  {
    id: 'framework',
    items: [
    { name: '@wailsio/runtime', version: '3.0.0', license: 'MIT', url: 'https://www.npmjs.com/package/@wailsio/runtime' },
    { name: 'github.com/wailsapp/wails/v3', version: 'v3.0.0', license: 'MIT', url: 'https://github.com/wailsapp/wails' },
    { name: 'Go', version: '1.27.3', license: 'BSD-3-Clause', url: 'https://go.dev' },
    { name: 'next', version: '16.4.3', license: 'MIT', url: 'https://www.npmjs.com/package/next' },
    { name: 'react', version: '19.3.1', license: 'MIT', url: 'https://www.npmjs.com/package/react' },
    { name: 'react-dom', version: '19.3.1', license: 'MIT', url: 'https://www.npmjs.com/package/react-dom' },
    ],
  },
  {
    id: 'frontend',
    items: [
    { name: '@radix-ui/react-collapsible', version: '1.2.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-collapsible' },
    { name: '@radix-ui/react-context-menu', version: '2.4.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-context-menu' },
    { name: '@radix-ui/react-dialog', version: '1.3.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-dialog' },
    { name: '@radix-ui/react-label', version: '2.1.17', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-label' },
    { name: '@radix-ui/react-radio-group', version: '1.5.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-radio-group' },
    { name: '@radix-ui/react-scroll-area', version: '1.4.0', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-scroll-area' },
    { name: '@radix-ui/react-select', version: '2.4.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-select' },
    { name: '@radix-ui/react-slider', version: '1.6.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-slider' },
    { name: '@radix-ui/react-switch', version: '1.4.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-switch' },
    { name: '@radix-ui/react-tabs', version: '1.2.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-tabs' },
    { name: '@radix-ui/react-tooltip', version: '1.4.1', license: 'MIT', url: 'https://www.npmjs.com/package/@radix-ui/react-tooltip' },
    { name: 'class-variance-authority', version: '0.7.1', license: 'Apache-2.0', url: 'https://www.npmjs.com/package/class-variance-authority' },
    { name: 'clsx', version: '2.1.1', license: 'MIT', url: 'https://www.npmjs.com/package/clsx' },
    { name: 'framer-motion', version: '14.6.0', license: 'MIT', url: 'https://www.npmjs.com/package/framer-motion' },
    { name: 'i18next', version: '26.5.11', license: 'MIT', url: 'https://www.npmjs.com/package/i18next' },
    { name: 'lucide-react', version: '1.62.0', license: 'ISC', url: 'https://www.npmjs.com/package/lucide-react' },
    { name: 'next-themes', version: '0.4.6', license: 'MIT', url: 'https://www.npmjs.com/package/next-themes' },
    { name: 'radix-ui', version: '1.8.1', license: 'MIT', url: 'https://www.npmjs.com/package/radix-ui' },
    { name: 'react-i18next', version: '17.1.9', license: 'MIT', url: 'https://www.npmjs.com/package/react-i18next' },
    { name: 'react-icons', version: '5.7.0', license: 'MIT', url: 'https://www.npmjs.com/package/react-icons' },
    { name: 'recharts', version: '3.10.1', license: 'MIT', url: 'https://www.npmjs.com/package/recharts' },
    { name: 'shadcn', version: '4.22.1', license: 'MIT', url: 'https://www.npmjs.com/package/shadcn' },
    { name: 'sonner', version: '2.0.8', license: 'MIT', url: 'https://www.npmjs.com/package/sonner' },
    { name: 'sortablejs', version: '1.15.7', license: 'MIT', url: 'https://www.npmjs.com/package/sortablejs' },
    { name: 'tailwind-merge', version: '3.7.0', license: 'MIT', url: 'https://www.npmjs.com/package/tailwind-merge' },
    { name: 'tw-animate-css', version: '1.4.0', license: 'MIT', url: 'https://www.npmjs.com/package/tw-animate-css' },
    { name: 'zustand', version: '5.0.16', license: 'MIT', url: 'https://www.npmjs.com/package/zustand' },
    ],
  },
  {
    id: 'device',
    items: [
    { name: 'github.com/go-ole/go-ole', version: 'v1.3.0', license: 'MIT', url: 'https://github.com/go-ole/go-ole' },
    { name: 'github.com/saltosystems/winrt-go', version: 'v0.0.0-20261016124014-b298124d7472', license: 'MIT', url: 'https://github.com/saltosystems/winrt-go' },
    { name: 'github.com/shirou/gopsutil/v4', version: 'v4.26.9', license: 'BSD-3-Clause', url: 'https://github.com/shirou/gopsutil' },
    { name: 'github.com/sstallion/go-hid', version: 'v0.15.0', license: 'BSD-2-Clause', url: 'https://github.com/sstallion/go-hid' },
    { name: 'github.com/yusufpapurcu/wmi', version: 'v1.2.4', license: 'MIT', url: 'https://github.com/yusufpapurcu/wmi' },
    { name: 'HIDAPI', version: 'bundled in go-hid', license: 'BSD-3-Clause', url: 'https://github.com/libusb/hidapi' },
    { name: 'LibreHardwareMonitorLib', version: '0.9.6', license: 'MPL-2.0', url: 'https://github.com/LibreHardwareMonitor/LibreHardwareMonitor' },
    { name: 'tinygo.org/x/bluetooth', version: 'v0.16.0', license: 'BSD-3-Clause', url: 'https://github.com/tinygo-org/bluetooth' },
    ],
  },
  {
    id: 'backend',
    items: [
    { name: 'fyne.io/systray', version: 'v1.12.2', license: 'Apache-2.0', url: 'https://github.com/fyne-io/systray' },
    { name: 'git.sr.ht/~jackmordaunt/go-toast', version: 'v1.1.2', license: 'MIT', url: 'https://git.sr.ht/~jackmordaunt/go-toast' },
    { name: 'github.com/adrg/xdg', version: 'v0.5.3', license: 'MIT', url: 'https://github.com/adrg/xdg' },
    { name: 'github.com/gen2brain/beeep', version: 'v0.11.3', license: 'BSD-2-Clause', url: 'https://github.com/gen2brain/beeep' },
    { name: 'github.com/godbus/dbus/v5', version: 'v5.2.4', license: 'BSD-2-Clause', url: 'https://github.com/godbus/dbus' },
    { name: 'github.com/mattn/go-colorable', version: 'v0.1.16', license: 'MIT', url: 'https://github.com/mattn/go-colorable' },
    { name: 'github.com/mattn/go-isatty', version: 'v0.0.24', license: 'MIT', url: 'https://github.com/mattn/go-isatty' },
    { name: 'github.com/Microsoft/go-winio', version: 'v0.6.4', license: 'MIT', url: 'https://github.com/Microsoft/go-winio' },
    { name: 'github.com/sergeymakinen/go-bmp', version: 'v1.0.0', license: 'BSD-3-Clause', url: 'https://github.com/sergeymakinen/go-bmp' },
    { name: 'github.com/sergeymakinen/go-ico', version: 'v1.0.0', license: 'BSD-3-Clause', url: 'https://github.com/sergeymakinen/go-ico' },
    { name: 'github.com/tadvi/systray', version: 'v0.0.0-20190226123456-11a2b8fa57af', license: 'MIT', url: 'https://github.com/tadvi/systray' },
    { name: 'go.uber.org/multierr', version: 'v1.11.0', license: 'MIT', url: 'https://github.com/uber-go/multierr' },
    { name: 'go.uber.org/zap', version: 'v1.28.0', license: 'MIT', url: 'https://github.com/uber-go/zap' },
    { name: 'golang.design/x/hotkey', version: 'v0.6.4', license: 'MIT', url: 'https://github.com/golang-design/hotkey' },
    { name: 'golang.org/x/sys', version: 'v0.49.0', license: 'BSD-3-Clause', url: 'https://pkg.go.dev/golang.org/x/sys' },
    { name: 'gopkg.in/natefinch/lumberjack.v2', version: 'v2.2.1', license: 'MIT', url: 'https://github.com/natefinch/lumberjack' },
    { name: 'Newtonsoft.Json', version: '13.0.3', license: 'MIT', url: 'https://github.com/JamesNK/Newtonsoft.Json' },
    ],
  },
  {
    id: 'fonts',
    items: [
    { name: '@fontsource-variable/geist-mono', version: '5.3.0', license: 'OFL-1.1', url: 'https://www.npmjs.com/package/@fontsource-variable/geist-mono' },
    { name: '@fontsource-variable/manrope', version: '5.3.0', license: 'OFL-1.1', url: 'https://www.npmjs.com/package/@fontsource-variable/manrope' },
    ],
  },
];

export const OPEN_SOURCE_TOTAL = OPEN_SOURCE_GROUPS.reduce((sum, group) => sum + group.items.length, 0);
