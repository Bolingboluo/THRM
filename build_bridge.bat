@echo off
setlocal
echo Building TempBridge with the pinned LibreHardwareMonitor commit...

set "ROOT=%~dp0"
set "PROJECT=%ROOT%bridge\TempBridge\TempBridge.csproj"
set "OUTDIR=%ROOT%build\bin\bridge"
set "BUILDROOT=%ROOT%build\bin"
set "TEMPROOT=%ROOT%temp"
rem THRM fork of LibreHardwareMonitor. Its default branch main tracks upstream master;
rem PR #2396 (Fire Range / Zen 5 X3D CPU power & topology) was merged upstream in 2026-10,
rem so main currently matches upstream content. Keep syncing with "Merge upstream master into main".
rem
rem The build is pinned to one commit so that the same THRM source always produces the same
rem TempBridge. To upgrade, look up the new head and paste its SHA below:
rem   git ls-remote https://github.com/TIANLI0/LibreHardwareMonitor.git refs/heads/main
rem LIBRE_HARDWARE_MONITOR_COMMIT overrides the pin for a one-off local experiment.
set "LHM_URL=https://github.com/TIANLI0/LibreHardwareMonitor.git"
set "LHM_COMMIT=1541e5a6753c1acc44ad3351b331f823c3afeb7c"
if defined LIBRE_HARDWARE_MONITOR_COMMIT set "LHM_COMMIT=%LIBRE_HARDWARE_MONITOR_COMMIT%"
set "LHM_REPO=%TEMPROOT%\LibreHardwareMonitor"
if defined LIBRE_HARDWARE_MONITOR_REPO set "LHM_REPO=%LIBRE_HARDWARE_MONITOR_REPO%"
set "LHM_PROJECT=%LHM_REPO%\LibreHardwareMonitorLib\LibreHardwareMonitorLib.csproj"

rem PawnIO installer: version and SHA-256 are pinned in build\windows\pawnio.env (see the
rem upgrade notes there). The same file feeds -DPAWNIO_BUNDLED_VERSION to makensis.
set "PAWNIO_ENV=%ROOT%build\windows\pawnio.env"
set "PAWNIO_VERSION="
set "PAWNIO_SHA256="
if not exist "%PAWNIO_ENV%" (
	echo ERROR: %PAWNIO_ENV% not found.
	goto :error
)
for /f "usebackq eol=# tokens=1,* delims==" %%a in ("%PAWNIO_ENV%") do set "%%a=%%b"
if "%PAWNIO_VERSION%"=="" (
	echo ERROR: PAWNIO_VERSION missing from %PAWNIO_ENV%
	goto :error
)
if "%PAWNIO_SHA256%"=="" (
	echo ERROR: PAWNIO_SHA256 missing from %PAWNIO_ENV%
	goto :error
)
set "PAWNIO_URL=https://github.com/namazso/PawnIO.Setup/releases/download/%PAWNIO_VERSION%/PawnIO_setup.exe"
set "PAWNIO_OUT=%BUILDROOT%\PawnIO_setup.exe"

if not exist "%BUILDROOT%" mkdir "%BUILDROOT%"
if not exist "%TEMPROOT%" mkdir "%TEMPROOT%"
if exist "%OUTDIR%" rmdir /s /q "%OUTDIR%"
if not exist "%OUTDIR%" mkdir "%OUTDIR%"

where git >nul 2>nul
if errorlevel 1 (
	echo ERROR: git not found. Cannot fetch LibreHardwareMonitor.
	goto :error
)

if not exist "%LHM_REPO%\.git" (
	echo Initializing LibreHardwareMonitor checkout in %LHM_REPO%...
	git init -q "%LHM_REPO%"
	if errorlevel 1 goto :error
	git -C "%LHM_REPO%" remote add origin "%LHM_URL%"
	if errorlevel 1 goto :error
)

set "LHM_HEAD="
for /f %%i in ('git -C "%LHM_REPO%" rev-parse HEAD 2^>nul') do set "LHM_HEAD=%%i"
if /i "%LHM_HEAD%"=="%LHM_COMMIT%" (
	echo LibreHardwareMonitor already at %LHM_COMMIT%
) else (
	rem Point the checkout at the fork even if it previously tracked upstream, then fetch
	rem exactly the pinned commit. GitHub serves any reachable commit by SHA.
	echo Fetching LibreHardwareMonitor %LHM_COMMIT% from %LHM_URL%...
	git -C "%LHM_REPO%" remote set-url origin "%LHM_URL%"
	if errorlevel 1 goto :error
	git -C "%LHM_REPO%" fetch --depth 1 origin %LHM_COMMIT%
	if errorlevel 1 goto :error
	git -C "%LHM_REPO%" checkout -q --detach FETCH_HEAD
	if errorlevel 1 goto :error
)

set "LHM_HEAD="
for /f %%i in ('git -C "%LHM_REPO%" rev-parse HEAD') do set "LHM_HEAD=%%i"
if /i not "%LHM_HEAD%"=="%LHM_COMMIT%" (
	echo ERROR: LibreHardwareMonitor checkout is at %LHM_HEAD%, expected %LHM_COMMIT%
	goto :error
)

if not exist "%LHM_PROJECT%" (
	echo ERROR: LibreHardwareMonitorLib project not found at %LHM_PROJECT%
	goto :error
)

echo Using LibreHardwareMonitor commit: %LHM_COMMIT%

echo Restoring TempBridge dependencies...
dotnet restore "%PROJECT%" /p:Platform=x64 /p:UseLibreHardwareMonitorProjectReference=true /p:LibreHardwareMonitorRepoRoot="%LHM_REPO%"
if errorlevel 1 goto :error

echo Publishing TempBridge...
dotnet publish "%PROJECT%" -c Release --self-contained false -o "%OUTDIR%" /p:Platform=x64 /p:DebugType=none /p:DebugSymbols=false /p:UseLibreHardwareMonitorProjectReference=true /p:LibreHardwareMonitorRepoRoot="%LHM_REPO%"
if errorlevel 1 goto :error

echo Removing non-runtime bridge artifacts...
del /q "%OUTDIR%\*.pdb" 2>nul
del /q "%OUTDIR%\*.xml" 2>nul
for /d %%D in ("%OUTDIR%\??-??") do if exist "%%~D" rmdir /s /q "%%~D"

rem Reuse an existing download only if it matches the pinned hash; otherwise download into a
rem temporary file and move it into place after the SHA-256 check, so a bad download never
rem lands in build\bin. The PowerShell snippet reads URL, path and hash from the environment.
echo Fetching PawnIO %PAWNIO_VERSION% installer...
powershell -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference = 'Stop'; try { $want = $env:PAWNIO_SHA256.Trim().ToLowerInvariant(); $out = $env:PAWNIO_OUT; if ((Test-Path -LiteralPath $out) -and ((Get-FileHash -LiteralPath $out -Algorithm SHA256).Hash.ToLowerInvariant() -eq $want)) { Write-Host 'Existing PawnIO_setup.exe matches the pinned SHA-256'; exit 0 }; [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; $tmp = $out + '.download'; Invoke-WebRequest -Uri $env:PAWNIO_URL -OutFile $tmp -UseBasicParsing; $got = (Get-FileHash -LiteralPath $tmp -Algorithm SHA256).Hash.ToLowerInvariant(); if ($got -ne $want) { Remove-Item -LiteralPath $tmp -Force; throw ('PawnIO_setup.exe SHA-256 mismatch: expected ' + $want + ', got ' + $got) }; Move-Item -LiteralPath $tmp -Destination $out -Force; exit 0 } catch { Write-Error $_; exit 1 }"
if errorlevel 1 goto :error

if not exist "%PAWNIO_OUT%" goto :error
echo PawnIO installer %PAWNIO_VERSION% (SHA-256 verified) saved to: %PAWNIO_OUT%

echo Build completed. Output directory: %OUTDIR%
goto :end

:error
echo Build failed. See the output above.
exit /b 1

:end
endlocal
