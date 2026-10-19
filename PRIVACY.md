# Privacy Policy

*中文摘要：THRM 不收集、不上传、不出售任何个人数据。所有配置、日志和温度历史都只保存在你自己的电脑上。只有打开「关于」页时，程序才会联网：向官网 thrm.tianli0.top（失败时改为 GitHub）检查新版本、读取鸣谢名单，并加载名单里的头像；选择更新时从同一来源下载安装包。*

Last updated: 2026-10-20

## Summary

**THRM does not collect, transmit, or sell any personal data.**

THRM has no analytics, no telemetry, no crash reporting, no advertising identifiers, and no
user accounts. The official website only relays public release information, downloads and the
credits list from GitHub; it does not receive any data produced by the application. All data the
application produces stays on your computer.

## Data stored on your device

THRM stores the following locally. None of it leaves your machine unless you choose to share it.

| Data | Contents | Location |
| --- | --- | --- |
| Configuration | Fan curves, profiles, RGB settings, application preferences | Windows: application data / installation directory · Linux: XDG state directory |
| Logs | Diagnostic messages produced at runtime | `logs/` under the same directory · Linux may use `journald` |
| Temperature history | Recorded CPU/GPU/device temperature and fan speed samples | `telemetry/history.bin` under the data directory |

Despite its filename, `telemetry/history.bin` is a **local-only** temperature history file used
to draw charts in the interface. It is never uploaded.

## Data read from your system

To do its job, THRM reads hardware and system information locally, including CPU and GPU
temperatures, fan speeds, power draw, connected HID/BLE device identifiers, and the name of the
foreground process when per-application profiles are enabled. This information is used only to
drive fan control and the on-screen display. It is not stored beyond the local files listed
above and is never transmitted.

## Network connections

THRM connects to the network only while the **About** page is open, and only for the purposes
below. If you never open the About page, the application makes no network connections.

| Destination | Purpose | Data sent |
| --- | --- | --- |
| `thrm.tianli0.top` (official website) | Check for a newer release, load the credits list, and download the installer if you choose to update | Nothing beyond what any HTTPS request necessarily reveals: your IP address and a standard user agent |
| `api.github.com`, `github.com`, `raw.githubusercontent.com`, `*.githubusercontent.com` | The same requests, used only when the official website cannot be reached | Same as above |
| `q1.qlogo.cn`, `avatars.githubusercontent.com` | Display the avatars of sponsors and contributors in the credits list | Same as above |

No identifier, hardware profile, or usage data is attached to any of these requests.

The official website is operated by the maintainer. Its server relays the public GitHub release
information, release downloads and the credits file so that they can be reached from regions
where GitHub is slow or unavailable. Like any web server it keeps standard access logs (IP
address, time, requested path and user agent), which are used only to keep the service running
and are not shared or sold.

GitHub and Tencent (QQ avatars) are independent services, and their handling of these requests is
governed by their own policies, such as the
[GitHub Privacy Statement](https://docs.github.com/site-policy/privacy-policies/github-general-privacy-statement).

## Diagnostic packages

The application can export a diagnostic package (a `.zip` containing hardware information and
recent log files) to help investigate a problem. This is **always** initiated by you, saved to a
location you choose, and never uploaded automatically.

Before attaching a diagnostic package to a public issue report, please review its contents —
logs may contain device identifiers, file paths, and the names of running applications.

## Third-party components

THRM bundles [LibreHardwareMonitor](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor)
and [PawnIO](https://pawnio.eu/) to read hardware sensors on Windows. Both operate locally and
are not used to collect or transmit data.

## Children's privacy

THRM does not knowingly collect data from anyone, including children, because it does not
collect data at all.

## Changes

Changes to this policy are published in this file and tracked in the repository's Git history.

## Contact

Questions about this policy: open an issue at <https://github.com/TIANLI0/THRM/issues> or
contact the maintainer at the address listed in the [README](README.md#作者与联系).

## Related

- [Code Signing Policy](CODE_SIGNING_POLICY.md)
