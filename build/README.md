# Build Directory

Build configuration and assets for the THRM GUI (Wails v3). Run the tasks from the repository root
with `wails3 task <name>` — see [`Taskfile.yml`](../Taskfile.yml).

* `config.yml` - Wails v3 project configuration. `info.version` is the single source of the THRM
  version; `build.bat`, `build.sh`, CI and `PKGBUILD` all read it. `dev_mode` drives `wails3 dev`.
* `appicon.png` - Application icon, also embedded as the Linux window icon.
* `bin` - Output directory.
* `.wails` - Assets rendered from `config.yml` during a build (ignored by git).

## Windows

* `icon.ico` - Executable and installer icon.
* `info.json` - Version resource template; `@VERSION@` is replaced with `info.version` at build time.
  It deliberately declares `FileVersion` and the fixed `product_version`, which Wails' own template
  omits (Windows then shows no version details at all).
* `wails.exe.manifest` - Application manifest. Maintained by hand because THRM requires
  administrator rights; it is not regenerated from `config.yml`.
* `installer/*` - NSIS installer. `wails_tools.nsh` is re-rendered from `config.yml` on every build
  (`wails3 task assets`) and is not tracked by git; run a build (or `wails3 task assets`) before calling
  `makensis` by hand. `project.nsi` and the other files are maintained by hand.
