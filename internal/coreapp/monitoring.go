package coreapp

import (
	"fmt"
	"math"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/TIANLI0/THRM/internal/bridge"
	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/laptopfan"
	"github.com/TIANLI0/THRM/internal/rtss"
	"github.com/TIANLI0/THRM/internal/smartcontrol"
	"github.com/TIANLI0/THRM/internal/temperature"
	"github.com/TIANLI0/THRM/internal/types"
)

const staleBridgeUpdateThreshold = 3

// idleTemperatureMonitorInterval 是后台空闲（无 GUI 连接且未开启智能控温）时的温度采样间隔下限。
// 此时温度读取仅用于托盘提示与历史记录，放慢采样可显著降低桥接进程的传感器扫描开销与后台 CPU 占用。
const idleTemperatureMonitorInterval = 10 * time.Second

// idleMemoryReleaseCooldown 限制 GUI 断开后归还内存的最小间隔，避免频繁开关 GUI 时反复触发 GC。
const idleMemoryReleaseCooldown = 30 * time.Second

const (
	consecutiveBridgeFailureRestartThreshold = 2
	temperatureBridgeRestartCooldown         = 10 * time.Second
)

func trackBridgeTemperatureStaleness(temp types.TemperatureData, lastUpdate int64, staleCount int) (int64, int, bool) {
	if !temp.BridgeOk || temp.UpdateTime <= 0 {
		return 0, 0, false
	}
	if temp.UpdateTime != lastUpdate {
		return temp.UpdateTime, 0, false
	}
	staleCount++
	return lastUpdate, staleCount, staleCount >= staleBridgeUpdateThreshold
}

func shouldRestartTemperatureBridge(temp types.TemperatureData) bool {
	if temp.BridgeOk {
		return false
	}

	// 桥接启动握手改为后台执行，启动窗口内读取必然失败。这是过渡态而非故障，
	// 若据此触发自愈就会反复杀掉正在启动的进程，形成永不收敛的重启风暴。
	if bridge.IsStarting(temp.BridgeMsg) {
		return false
	}

	msg := strings.ToLower(strings.TrimSpace(temp.BridgeMsg))
	if msg == "" {
		return true
	}

	restartHints := []string{
		"启动桥接程序失败",
		"桥接程序通信失败",
		"桥接程序未连接",
		"连接管道失败",
		"发送命令失败",
		"读取响应失败",
		"等待桥接程序启动超时",
		"未能获取管道名称",
		"pipe",
		"broken",
		"closed",
		"timeout",
		"bridge reconnect failed",
	}
	for _, hint := range restartHints {
		if strings.Contains(msg, strings.ToLower(hint)) {
			return true
		}
	}

	// 休眠恢复后硬件监控库偶尔会返回全 0 但进程仍能响应，重启桥接可重新初始化底层传感器。
	return temp.CPUTemp == 0 && temp.GPUTemp == 0
}

// temperatureSelection 由配置构造温度读取选择。控温循环与各种"测试读取"都必须走这里，
// 否则测试看到的 CPU/控制温度会和循环实际使用的不一致（例如漏掉多传感器平均）。
func (a *CoreApp) temperatureSelection(cfg types.AppConfig) types.TemperatureSelection {
	return types.TemperatureSelection{
		TempSource:      cfg.TempSource,
		GpuDevice:       cfg.GpuDevice,
		CpuSensor:       cfg.CpuSensor,
		CpuSensors:      cfg.CpuSensors,
		GpuSensor:       cfg.GpuSensor,
		DisableGpu:      cfg.DisableGpuMonitoring,
		ExtendedSensors: a.extendedSensorsActive(),
	}
}

// useCompactBridgeSample 决定控温循环本次采样是否向桥接请求精简结果
// （标量读数 + 多传感器平均所需的 CPU 传感器，不带完整传感器清单）。
//
// 这是唯一的判定点。没有 GUI 在看时，完整清单没有任何消费者：托盘提示、RTSS、历史、
// 智能控温、功耗读数只用标量；清单由 temperature.Reader 用最近一次完整结果回填，
// 刚连上的 GUI 通过 GetTemperature 仍能立刻拿到清单。有 GUI 时必须读完整结果——
// 判定在 Read 之前，GUI 连上后的第一次采样就是完整的，配合 clientsJustConnected
// 的整帧广播，新 GUI 拿到的是最新清单。
// 散热收益测试（扩展传感器开启期间）要逐帧采集全部传感器，同样始终读完整结果。
func useCompactBridgeSample(hasClients, extendedSensors bool) bool {
	return !hasClients && !extendedSensors
}

// nextTemperatureEMA 计算窗口为 n 的温度 EMA（α = 2/(n+1)）。
//
// 必须用浮点累积：整数除法向下取整，升温时每步都被截掉一点，最终稳定在真实温度之下
// （n=10 时 50→60°C 的阶跃只收敛到 55°C），风扇因此长期比曲线低几百转，
// 自适应学习也会把这段虚低的稳态学成负偏移。降温方向则恰好能收敛，所以很难察觉。
func nextTemperatureEMA(prev float64, sample, n int) float64 {
	n = max(n, 1)
	return (2*float64(sample) + float64(n-1)*prev) / float64(n+1)
}

func (a *CoreApp) recoverTemperatureBridge(reason string) {
	a.safeRun("temperature-bridge-recover@"+reason, func() {
		a.bridgeManager.Stop()
		if err := a.bridgeManager.EnsureRunning(); err != nil {
			a.logError("温度桥接自愈重启失败[%s]: %v", reason, err)
			return
		}
		a.logInfo("温度桥接已完成自愈重启: %s", reason)
	})
}

// temperatureEventPayload 选择本帧温度事件的编码。
//
// fullFrame（刚有 GUI 连上或恢复订阅）发送完整数据；所有订阅中的客户端都能还原
// 传感器读数时，清单不变的传感器组只发读数（ipc.CompactTemperatureEvent）；
// 否则退回旧格式，只省略连读数都没变的清单。
func temperatureEventPayload(current, previous types.TemperatureData, fullFrame, sensorValues bool) any {
	if fullFrame {
		return current
	}
	if sensorValues {
		return ipc.CompactTemperatureEvent(current, previous)
	}
	return compactTemperatureEventPayload(current, previous)
}

// compactTemperatureEventPayload 去掉与上一帧完全相同的传感器清单，只广播变化的部分。
// 这是旧格式：读数一变整份清单就要重发，只在有不认识 sensorValues 的旧客户端时使用。
//
// 这里用逐字段比较而不是 reflect.DeepEqual：这条路径在 GUI 打开时每个采样周期都要
// 跑一遍，而传感器清单动辄几十项，反射比较要为每个元素的每个字段走一次反射调度，
// 比直接比较慢一个数量级。
func compactTemperatureEventPayload(current, previous types.TemperatureData) types.TemperatureData {
	compact := current
	if temperatureSensorsEqual(current.CpuSensors, previous.CpuSensors) {
		compact.CpuSensors = nil
	}
	if temperatureSensorsEqual(current.GpuSensors, previous.GpuSensors) {
		compact.GpuSensors = nil
	}
	if powerSensorsEqual(current.CpuPowerSensors, previous.CpuPowerSensors) {
		compact.CpuPowerSensors = nil
	}
	if powerSensorsEqual(current.GpuPowerSensors, previous.GpuPowerSensors) {
		compact.GpuPowerSensors = nil
	}
	if gpuDevicesEqual(current.GpuDevices, previous.GpuDevices) {
		compact.GpuDevices = nil
	}
	// OtherSensors 不做差分：它带 omitempty，空切片同样会被省略，GUI 无法区分"未变"与
	// "已清空"（关掉扩展传感器后会一直显示旧读数）。它只在散热收益测试期间存在，原样发送代价很小。
	return compact
}

// 三个比较函数都保留 nil 与空切片的区别，与 reflect.DeepEqual 的语义完全一致：
// 空切片序列化成 []（明确告诉前端清空旧清单），nil 则被 omitempty 略去（保持不变），
// 二者对前端的含义不同，不能混为一谈。

func temperatureSensorsEqual(a, b []types.TemperatureSensor) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func powerSensorsEqual(a, b []types.PowerSensor) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func gpuDevicesEqual(a, b []types.TemperatureGPUDevice) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Name != b[i].Name || a[i].Vendor != b[i].Vendor {
			return false
		}
		if !temperatureSensorsEqual(a[i].Sensors, b[i].Sensors) {
			return false
		}
		if !powerSensorsEqual(a[i].PowerSensors, b[i].PowerSensors) {
			return false
		}
	}
	return true
}

func (a *CoreApp) stopTemperatureMonitoring() <-chan struct{} {
	a.monitorMutex.Lock()
	defer a.monitorMutex.Unlock()

	if !a.monitoringTemp.Load() || a.monitorDone == nil {
		return nil
	}
	if !a.monitorStopping {
		close(a.monitorStop)
		a.monitorStopping = true
	}
	return a.monitorDone
}

// ensureTemperatureMonitoring starts a new monitoring session only after a
// previous stopping session has fully exited. This prevents suspend/resume and
// reconnect races from leaving monitoring permanently off until the next health
// check.
func (a *CoreApp) ensureTemperatureMonitoring(reason string) {
	if a.stopping.Load() || a.systemSuspended.Load() {
		return
	}
	a.monitorMutex.Lock()
	running := a.monitoringTemp.Load()
	stopping := a.monitorStopping
	done := a.monitorDone
	a.monitorMutex.Unlock()

	if !running {
		a.safeGo("startTemperatureMonitoring@"+reason, func() {
			a.startTemperatureMonitoring()
		})
		return
	}
	if !stopping || done == nil {
		return
	}

	a.safeGo("restartTemperatureMonitoring@"+reason, func() {
		select {
		case <-done:
			a.startTemperatureMonitoring()
		case <-time.After(12 * time.Second):
			a.logError("等待旧温度监控退出超时，暂不启动新的监控会话（来源=%s）", reason)
		}
	})
}

// startTemperatureMonitoring 开始温度监控
func (a *CoreApp) startTemperatureMonitoring() {
	if a.stopping.Load() || a.systemSuspended.Load() {
		return
	}
	a.monitorMutex.Lock()
	if a.stopping.Load() || a.systemSuspended.Load() || a.monitoringTemp.Load() {
		a.monitorMutex.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	a.monitorStop = stop
	a.monitorDone = done
	a.monitorStopping = false
	a.monitoringTemp.Store(true)
	a.monitorMutex.Unlock()
	defer func() {
		a.monitorMutex.Lock()
		if a.monitorDone == done {
			a.monitorStop = nil
			a.monitorDone = nil
			a.monitorStopping = false
			a.monitoringTemp.Store(false)
		}
		a.monitorMutex.Unlock()
		close(done)
	}()

	// 注意：不在此处立即调用 EnterAutoMode，因为在启动时温度数据（桥接程序）可能尚未就绪。
	// 如果在温度读取成功之前切换到软件控制模式，设备将不会收到转速指令，导致风扇停转。
	// EnterAutoMode 和转速设置会在首次成功读取温度后，由 SetFanSpeed 内部统一完成。

	cfg, cfgRevision := a.configManager.GetWithRevision()
	hasClients := a.ipcServer != nil && a.ipcServer.HasClients()
	hasActiveClients := a.ipcServer != nil && a.ipcServer.HasActiveClients()
	updateInterval := effectiveTemperatureMonitorInterval(cfg.TempUpdateRate, hasActiveClients, cfg.AutoControl)

	// 温度采样使用 EMA 平滑。
	sampleCount := max(cfg.TempSampleCount, 1)
	tempEMA := 0.0
	tempEMAReady := false

	rawTempHistory := make([]int, 0, 6)
	recentAvgTemps := make([]int, 0, 24)
	recentControlTemps := make([]int, 0, 24)
	initialTemp := a.tempReader.Read(a.temperatureSelection(cfg))
	if initialTemp.ControlTemp > 0 {
		rawTempHistory = append(rawTempHistory, initialTemp.ControlTemp)
	}
	lastTargetRPM := -1
	lastDeviceGeneration := a.deviceManager.ConnectionGeneration()
	settingsRefreshGeneration := uint64(0)
	// 学习偏移每次合并都会立刻进入内存配置；这里只控制落盘与向 GUI 广播的频率。
	learningUnbroadcast := false
	lastLearningSave := time.Now()
	lastLearningBroadcast := time.Now()
	lastBridgeUpdateTime := initialTemp.UpdateTime
	staleBridgeUpdateCount := 0
	bridgeFailureCount := 0
	invalidControlTempCount := 0
	safeFallbackActive := false
	lastBridgeRestart := time.Time{}
	var smartCfg types.SmartControlConfig
	smartCfgRevision := cfgRevision - 1
	// cachedCfg 是最近一次按修订号取到的配置深拷贝。配置没变的周期直接复用它，
	// 不再每个周期深拷贝整份配置（曲线方案、各方案学习偏移、散热收益报告……）。
	cachedCfg, cachedCfgRevision := cfg, cfgRevision

	// 每个曲线点对应一个稳态采样桶。
	steadyObserver := smartcontrol.NewStableObserver(len(cfg.FanCurve))
	// 观测器里的分桶样本与效率历史只对学到它们的那条曲线有意义。
	observerProfileID := cfg.ActiveFanCurveProfileID
	observerCurve := slices.Clone(cfg.FanCurve)
	timer := time.NewTimer(updateInterval)
	defer timer.Stop()
	// 只统计"定时器等待"期间流逝的时间，不把本轮采样/控温的耗时算进去。
	// 桥接一次 GetTemperature 超时就是 10 秒，加上兜底读取轻松突破 20 秒的
	// 唤醒判定阈值——若按"上一轮开始到本轮开始"计时，一次桥接卡顿就会被
	// 误判成系统休眠唤醒，白白触发断连重连与桥接重启，而重启桥接本身又要
	// 十几秒，足以让误判自我延续。
	lastTimerReset := time.Now()
	resetTimer := func(d time.Duration) {
		lastTimerReset = time.Now()
		timer.Reset(d)
	}

	prevHasClients := hasClients
	prevHasActiveClients := hasActiveClients
	var lastMemRelease time.Time
	var laptopSpeeds laptopfan.FanSpeeds
	laptopSpeedsOK := false
	var lastLaptopFanRead time.Time
	var activeClientsChanged <-chan struct{}
	if a.ipcServer != nil {
		activeClientsChanged = a.ipcServer.ActiveClientsChanged()
	}

monitorLoop:
	for {
		select {
		case <-stop:
			break monitorLoop
		case <-activeClientsChanged:
			// GUI 回到前台时，若正处在后台放慢的采样间隔里，立即补一次采样，界面不必
			// 干等一个空闲周期才看到实时数据。只提前、不推迟；不更新 lastTimerReset，
			// 休眠唤醒判定仍按真实的等待时长计算。智能控温开启时间隔本就未放慢，不受影响。
			// HID 读循环也立即恢复常规轮询，下一次采样会按最新状态重新判定。
			a.deviceManager.SetIdlePolling(false)
			if updateInterval > temperatureMonitorInterval(cfg.TempUpdateRate) && timer.Stop() {
				timer.Reset(0)
			}
		case <-timer.C:
			now := time.Now()
			gap := now.Sub(lastTimerReset)
			if a.maybeRecoverFromSystemResume("temperature-monitor", gap, updateInterval) {
				resetTimer(updateInterval)
				continue
			}
			if a.systemSuspended.Load() {
				resetTimer(updateInterval)
				continue
			}
			deviceGeneration := a.deviceManager.ConnectionGeneration()
			if deviceGeneration != lastDeviceGeneration {
				// A reconnect starts the device in its own default gear mode. Forget
				// the target sent through the old HID handle so the next valid
				// temperature sample re-enters realtime mode and writes a target.
				lastDeviceGeneration = deviceGeneration
				lastTargetRPM = -1
				steadyObserver.Reset()
				a.logInfo("检测到设备重连（连接代次=%d），重置智能控温输出状态", deviceGeneration)
			}

			if a.configManager.Revision() != cachedCfgRevision {
				cachedCfg, cachedCfgRevision = a.configManager.GetWithRevision()
			}
			// 浅拷贝：本周期对 cfg 字段的重新赋值不会带进缓存；其中的切片/映射在循环里只读。
			cfg, cfgRevision = cachedCfg, cachedCfgRevision
			if a.applyTimeCurveSchedule(cfg, now) {
				// 刚切换了曲线方案：重取快照，本周期就按新曲线控温，后面也不会拿旧快照回写。
				cachedCfg, cachedCfgRevision = a.configManager.GetWithRevision()
				cfg, cfgRevision = cachedCfg, cachedCfgRevision
			}
			// 后台空闲（无前台 GUI 且未开启智能控温）时放慢采样：此时温度读取不驱动风扇，
			// 仅服务托盘提示与历史记录，降低采样频率可显著减少桥接传感器扫描带来的后台 CPU 占用。
			// 窗口隐藏的 GUI 已退订高频事件，不消费遥测，与没有 GUI 同样对待。
			hasClients = a.ipcServer != nil && a.ipcServer.HasClients()
			hasActiveClients = a.ipcServer != nil && a.ipcServer.HasActiveClients()
			updateInterval = effectiveTemperatureMonitorInterval(cfg.TempUpdateRate, hasActiveClients, cfg.AutoControl)
			a.deviceManager.SetIdlePolling(hidIdlePollingAllowed(hasActiveClients, cfg.RTSS))
			// GUI 断开瞬间把会话期间膨胀的堆内存归还操作系统，降低核心常驻后台时的 RSS。
			if prevHasClients && !hasClients && now.Sub(lastMemRelease) > idleMemoryReleaseCooldown {
				lastMemRelease = now
				a.safeGo("release-idle-memory", func() { debug.FreeOSMemory() })
			}
			// 扩展传感器只为散热收益测试临时开启，而测试跑在 GUI 里。
			a.releaseExtendedSensorsIfIdle(hasClients)
			// 刚有 GUI 连上或恢复订阅：它没收到过上一帧，差分掉的传感器清单对它来说就是缺失。
			clientsJustConnected := hasActiveClients && !prevHasActiveClients
			prevHasClients = hasClients
			prevHasActiveClients = hasActiveClients

			selection := a.temperatureSelection(cfg)
			selection.Compact = useCompactBridgeSample(hasActiveClients, selection.ExtendedSensors)
			temp := a.tempReader.Read(selection)
			// 补充笔记本内置风扇转速（机械革命等 Uniwill/同方准系统），不支持时保持 0。
			// 每次读取是两次 WMI ExecMethod。没有 GUI 时它只服务于温度历史（托盘、RTSS 不显示它），
			// 因此只在历史本轮要记录样本、或上次读数已过旧时才读取，其余周期沿用上次读数。
			// 【读取判据集中于此】GUI 最小化时不算活跃，同样按无 GUI 处理。
			if laptopFanReadDue(hasActiveClients, a.tempHistory.SampleDue(temp), now.Sub(lastLaptopFanRead)) {
				laptopSpeeds, laptopSpeedsOK = a.laptopFanReader.Read()
				lastLaptopFanRead = now
			}
			if laptopSpeedsOK {
				temp.CPUFanRPM = laptopSpeeds.CPUFanRPM
				temp.GPUFanRPM = laptopSpeeds.GPUFanRPM
			}
			if temp.ControlTemp <= 0 {
				// 桥接正在后台启动时读不到温度属于预期过渡态，不能计入"传感器失效"，
				// 否则每次启动/自愈窗口都会把风扇推到安全兜底转速。
				if !bridge.IsStarting(temp.BridgeMsg) {
					invalidControlTempCount++
				}
			} else {
				if safeFallbackActive {
					a.logInfo("Temperature source recovered; resuming normal automatic control")
					if a.ipcServer != nil {
						a.ipcServer.BroadcastEvent(ipc.EventDeviceError, "Temperature source recovered; normal fan control resumed")
					}
				}
				invalidControlTempCount = 0
				safeFallbackActive = false
			}
			if temp.BridgeOk {
				bridgeFailureCount = 0
				staleBridge := false
				lastBridgeUpdateTime, staleBridgeUpdateCount, staleBridge = trackBridgeTemperatureStaleness(temp, lastBridgeUpdateTime, staleBridgeUpdateCount)
				if staleBridge && time.Since(lastBridgeRestart) >= temperatureBridgeRestartCooldown {
					a.logError("温度桥接返回的 updateTime 连续 %d 次未变化，触发桥接重连自愈", staleBridgeUpdateCount+1)
					a.recoverTemperatureBridge("stale-update")
					lastBridgeRestart = time.Now()
					lastBridgeUpdateTime = 0
					staleBridgeUpdateCount = 0
				}
			} else {
				lastBridgeUpdateTime = 0
				staleBridgeUpdateCount = 0
				if shouldRestartTemperatureBridge(temp) {
					bridgeFailureCount++
					if bridgeFailureCount >= consecutiveBridgeFailureRestartThreshold && time.Since(lastBridgeRestart) >= temperatureBridgeRestartCooldown {
						a.logError("温度桥接连续 %d 次读取失败，触发桥接重连自愈: %s", bridgeFailureCount, temp.BridgeMsg)
						a.recoverTemperatureBridge("read-failure")
						lastBridgeRestart = time.Now()
						bridgeFailureCount = 0
					}
				} else {
					bridgeFailureCount = 0
				}
			}

			a.mutex.Lock()
			previousTemp := a.currentTemp
			a.currentTemp = temp
			a.mutex.Unlock()

			historyPoint, recorded := a.tempHistory.Add(temp, a.deviceManager.GetCurrentFanData())

			// 广播温度更新（无前台 GUI 时跳过差分与序列化，核心常驻后台时显著降低每秒开销）
			if hasActiveClients {
				sensorValues := a.ipcServer.ActiveClientsSupport(ipc.FeatureTemperatureSensorValues)
				a.ipcServer.BroadcastEvent(ipc.EventTemperatureUpdate, temperatureEventPayload(temp, previousTemp, clientsJustConnected, sensorValues))
				if recorded {
					a.ipcServer.BroadcastEvent(ipc.EventTemperatureHistoryUpdate, historyPoint)
				}
			}

			if cfgRevision != smartCfgRevision {
				smartChanged := false
				smartCfg, smartChanged = smartcontrol.NormalizeConfig(cfg.SmartControl, cfg.FanCurve, cfg.DebugMode)
				smartCfgRevision = cfgRevision
				if smartChanged {
					cfg.SmartControl = smartCfg
					// 只改写智能控温段，并以最新配置为准重新归一化，不回写本周期开头的整份快照。
					_, persisted := a.configManager.Mutate(func(cur *types.AppConfig) bool {
						next, changed := smartcontrol.NormalizeConfig(cur.SmartControl, cur.FanCurve, cur.DebugMode)
						if changed {
							cur.SmartControl = next
						}
						return changed
					})
					if persisted {
						if err := a.configManager.Save(); err != nil {
							a.logError("保存智能控温配置失败: %v", err)
						}
					}
				}
			}

			controlReady := a.isDeviceControlReady()
			if cfg.LightStrip.Mode == "smart_temp" && temp.MaxTemp > 0 && controlReady {
				// Smart lighting is host-driven: the cooler firmware exposes native
				// green/yellow/red animations through 0x44, but it cannot read the
				// computer's CPU/GPU temperature by itself. The device manager applies
				// hysteresis and suppresses duplicate writes.
				if a.lockDeviceControlIfReady() {
					err := a.deviceManager.UpdateSmartTemperatureLight(temp.MaxTemp)
					a.deviceControlMutex.Unlock()
					if err != nil {
						a.logError("智能温控灯效更新失败: %v", err)
					}
					a.broadcastSmartLightStatus()
				}
			}
			if cfg.AutoControl && controlReady && invalidControlTempCount >= 3 && !safeFallbackActive {
				_, safeRPM := smartcontrol.GetCurveRPMBounds(cfg.FanCurve)
				if safeRPM <= 0 {
					safeRPM = 3000
				}
				ready, written := a.setAutomaticFanSpeed(safeRPM)
				if ready && written {
					safeFallbackActive = true
					lastTargetRPM = safeRPM
					message := fmt.Sprintf("Temperature readings are unavailable; fan switched to safe fallback at %d RPM", safeRPM)
					a.logError("%s", message)
					if a.ipcServer != nil {
						a.ipcServer.BroadcastEvent(ipc.EventDeviceError, message)
					}
				}
			}
			if cfg.AutoControl && temp.ControlTemp > 0 && controlReady {
				// 采样窗口变化时重置 EMA，避免阶跃。
				newSampleCount := max(cfg.TempSampleCount, 1)
				if newSampleCount != sampleCount {
					sampleCount = newSampleCount
					tempEMAReady = false
				}

				// 切换方案或编辑曲线后点数通常不变，只比较点数会让新曲线沿用旧曲线的样本与效率历史。
				if steadyObserver == nil || cfg.ActiveFanCurveProfileID != observerProfileID || !slices.Equal(cfg.FanCurve, observerCurve) {
					steadyObserver = smartcontrol.NewStableObserver(len(cfg.FanCurve))
					observerProfileID = cfg.ActiveFanCurveProfileID
					observerCurve = slices.Clone(cfg.FanCurve)
				}

				activeCurve := cfg.FanCurve
				tickCfg := smartCfg

				sampleTemp := temp.ControlTemp
				sampleSpikeSuppressed := false
				if tickCfg.FilterTransientSpike {
					sampleTemp, sampleSpikeSuppressed = smartcontrol.FilterTransientSample(temp.ControlTemp, rawTempHistory, tickCfg.Hysteresis)
				}
				rawTempHistory = append(rawTempHistory, temp.ControlTemp)
				if len(rawTempHistory) > 6 {
					rawTempHistory = rawTempHistory[len(rawTempHistory)-6:]
				}

				if !tempEMAReady {
					tempEMA = float64(sampleTemp)
					tempEMAReady = true
				} else {
					tempEMA = nextTemperatureEMA(tempEMA, sampleTemp, sampleCount)
				}
				avgTemp := int(math.Round(tempEMA))

				recentAvgTemps = append(recentAvgTemps, avgTemp)
				if len(recentAvgTemps) > 24 {
					recentAvgTemps = recentAvgTemps[len(recentAvgTemps)-24:]
				}

				controlTemp := avgTemp
				controlSpikeSuppressed := false
				if tickCfg.FilterTransientSpike {
					controlTemp, controlSpikeSuppressed = smartcontrol.FilterTransientSpike(avgTemp, recentAvgTemps, tickCfg.TargetTemp, tickCfg.Hysteresis)
				}
				spikeSuppressed := sampleSpikeSuppressed || controlSpikeSuppressed
				recentControlTemps = append(recentControlTemps, controlTemp)
				if len(recentControlTemps) > 24 {
					recentControlTemps = recentControlTemps[len(recentControlTemps)-24:]
				}

				curveMinRPM, curveMaxRPM := smartcontrol.GetCurveRPMBounds(activeCurve)

				baseRPM := temperature.CalculateTargetRPM(controlTemp, activeCurve)
				prevTargetRPM := lastTargetRPM

				targetRPM := smartcontrol.CalculateTargetRPM(controlTemp, activeCurve, tickCfg)
				if targetRPM <= 0 {
					targetRPM = baseRPM
				}

				if targetRPM > 0 {
					targetRPM = min(max(targetRPM, curveMinRPM), curveMaxRPM)
				}

				if shouldApplyRampLimit(targetRPM, prevTargetRPM) {
					targetRPM = smartcontrol.ApplyRampLimit(targetRPM, prevTargetRPM, tickCfg.RampUpLimit, tickCfg.RampDownLimit)
					if targetRPM > 0 {
						targetRPM = min(max(targetRPM, curveMinRPM), curveMaxRPM)
					}
				}

				fanData := a.deviceManager.GetCurrentFanData()
				observedRPM := targetRPM
				if fanData != nil && fanData.CurrentRPM > 0 {
					observedRPM = int(fanData.CurrentRPM)
				}
				if shouldSendTargetRPM(targetRPM, prevTargetRPM, tickCfg.MinRPMChange, fanData) {
					ready, written := a.setAutomaticFanSpeed(targetRPM)
					if written {
						lastTargetRPM = targetRPM
						if deviceGeneration != settingsRefreshGeneration {
							settingsRefreshGeneration = deviceGeneration
							a.safeGo("refreshDeviceSettings@realtime-mode", func() {
								// The device applies a mode change asynchronously. Query after a
								// short settle period so the cached status shown to the GUI is
								// the new realtime mode rather than the pre-reconnect gear mode.
								time.Sleep(200 * time.Millisecond)
								if _, err := a.RefreshDeviceSettings(); err != nil {
									a.logError("实时模式切换后刷新设备状态失败: %v", err)
								}
							})
						}
					} else if ready {
						lastTargetRPM = -1
						a.logError("智能控温转速下发失败，将在下个周期重试: %d RPM", targetRPM)
					}
				}

				switch {
				case smartCfg.Learning:
					if !spikeSuppressed {
						steady := steadyObserver.Observe(controlTemp, observedRPM, cfg.FanCurve, smartCfg)
						if steady.Ready && steady.BucketIdx >= 0 {
							newOffsets, changed := smartcontrol.LearnSteadyOffset(
								steady.BucketIdx,
								steady.MeanTemp,
								steady.MeanRPM,
								steady.LocalEff,
								steady.HaveEff,
								cfg.FanCurve,
								smartCfg.LearnedOffsets,
								smartCfg,
							)
							if changed {
								smartCfg.LearnedOffsets = newOffsets
								cfg.SmartControl = smartCfg
								// 只合并学习偏移。期间若切换了方案或改了曲线，这批偏移是按旧曲线学的，丢弃。
								learnedProfileID := cfg.ActiveFanCurveProfileID
								learnedCurve := cfg.FanCurve
								_, merged := a.configManager.Mutate(func(cur *types.AppConfig) bool {
									if cur.ActiveFanCurveProfileID != learnedProfileID || !slices.Equal(cur.FanCurve, learnedCurve) {
										return false
									}
									cur.SmartControl.LearnedOffsets = cloneIntSlice(newOffsets)
									storeSmartControlOffsetsForActiveProfile(cur)
									return true
								})
								if merged {
									a.learnedOffsetsUnsaved.Store(true)
									learningUnbroadcast = true
								}
							}
						}
					}

				default:
					steadyObserver.Reset()
				}

				// 学习偏移在控温环里高频改写，每次都写盘既伤 SSD 也会拖慢采样周期。
				// 落盘放慢到 learnedOffsetsSaveInterval；退出、系统挂起前、重置学习与切换方案
				// 都会另行把内存中的配置整份落盘，不会因此丢失学习结果。
				if a.learnedOffsetsUnsaved.Load() && time.Since(lastLearningSave) >= learnedOffsetsSaveInterval {
					a.saveLearnedOffsetsIfDirty("periodic")
					lastLearningSave = time.Now()
				}
				// GUI 展示学习后的曲线，广播仍保持原来的 25 秒节奏，不随落盘一起放慢。
				if learningUnbroadcast && time.Since(lastLearningBroadcast) >= learnedOffsetsBroadcastInterval {
					learningUnbroadcast = false
					lastLearningBroadcast = time.Now()
					if a.ipcServer != nil && a.ipcServer.HasClients() {
						a.ipcServer.BroadcastEvent(ipc.EventConfigUpdate, a.configManager.Get())
					}
				}

				if baseRPM > 0 {
					a.logDebug("智能控温: 最高=%d°C 基准=%s 当前=%d°C 平均=%d°C 控制温度=%d°C 基础=%dRPM 目标=%dRPM", temp.MaxTemp, temp.ControlSource, temp.ControlTemp, avgTemp, controlTemp, baseRPM, targetRPM)
				}
			}

			if !cfg.AutoControl || !controlReady {
				lastTargetRPM = -1
			}

			resetTimer(updateInterval)
		}
	}

	a.saveLearnedOffsetsIfDirty("monitor-exit")
}

const (
	// learnedOffsetsSaveInterval 是学习偏移的周期性落盘间隔。每次落盘都是整份配置的
	// 序列化 + fsync，学习活跃时原先每 25 秒一次。
	learnedOffsetsSaveInterval = 3 * time.Minute
	// learnedOffsetsBroadcastInterval 是学习偏移变化后向 GUI 广播配置的最小间隔。
	learnedOffsetsBroadcastInterval = 25 * time.Second
)

// saveLearnedOffsetsIfDirty 把控温循环合并进内存、尚未落盘的学习偏移写盘。
// 在周期保存、控温循环退出、系统挂起前与核心退出时调用；没有待保存内容时什么也不做。
func (a *CoreApp) saveLearnedOffsetsIfDirty(reason string) {
	if a.configManager == nil || !a.learnedOffsetsUnsaved.Swap(false) {
		return
	}
	if err := a.configManager.Save(); err != nil {
		// 保存失败：保留待保存标记，下一次时机再试。
		a.learnedOffsetsUnsaved.Store(true)
		a.logError("保存学习状态失败[%s]: %v", reason, err)
	}
}

// laptopFanMaxAge 是无 GUI 时笔记本风扇读数可沿用的最长时间，与后台空闲采样间隔一致，
// 使 GUI 刚连上时拿到的读数不比原先逐周期读取时更旧。
const laptopFanMaxAge = idleTemperatureMonitorInterval

// laptopFanReadDue 决定本周期是否读取笔记本风扇转速：有 GUI 时每周期都读；
// 否则只在温度历史本轮要记录样本、或上次读数超过 laptopFanMaxAge 时读取。
func laptopFanReadDue(hasClients, historySampleDue bool, sinceLastRead time.Duration) bool {
	return hasClients || historySampleDue || sinceLastRead >= laptopFanMaxAge
}

func temperatureMonitorInterval(updateRateSeconds int) time.Duration {
	if updateRateSeconds < 1 {
		updateRateSeconds = 1
	}
	return time.Duration(updateRateSeconds) * time.Second
}

// effectiveTemperatureMonitorInterval keeps configured responsiveness whenever
// the GUI is present or automatic control is active. Only a fully idle core
// session is slowed down, because those samples do not influence fan output.
func effectiveTemperatureMonitorInterval(updateRateSeconds int, hasClients, autoControl bool) time.Duration {
	interval := temperatureMonitorInterval(updateRateSeconds)
	if !hasClients && !autoControl && interval < idleTemperatureMonitorInterval {
		return idleTemperatureMonitorInterval
	}
	return interval
}

// hidIdlePollingAllowed 决定 HID 读循环能否放慢到空闲轮询间隔（500ms）。
//
// 智能控温不需要快速轮询：控温环每个采样周期（≥1 秒）才读一次缓存的转速，
// 下发转速时等 ACK 由设备层的事务标记即时唤醒读循环。需要亚秒级被动读数的只有
// 前台 GUI 的实时转速，以及刷新间隔短于 1 秒的 RTSS 叠加层（500ms 轮询会让
// 500ms 的刷新时而拖成 1 秒）。
func hidIdlePollingAllowed(hasActiveClients bool, rtssCfg types.RTSSConfig) bool {
	if hasActiveClients {
		return false
	}
	if !rtssCfg.Enabled {
		return true
	}
	return rtss.SupportedUpdateInterval(time.Duration(rtssCfg.UpdateIntervalMS)*time.Millisecond) >= time.Second
}

func shouldApplyRampLimit(targetRPM, prevTargetRPM int) bool {
	return prevTargetRPM > 0 || targetRPM == 0
}

func shouldSendTargetRPM(targetRPM, prevTargetRPM, minRPMChange int, fanData *types.FanData) bool {
	if targetRPM < 0 {
		return false
	}
	if prevTargetRPM < 0 {
		return true
	}
	if absRPMDelta(targetRPM, prevTargetRPM) >= minRPMChange {
		return true
	}
	if fanData == nil {
		return false
	}
	deviceTargetRPM := int(fanData.TargetRPM)
	if targetRPM > 0 && (deviceTargetRPM == 0 || fanData.CurrentRPM == 0) {
		return true
	}
	return absRPMDelta(targetRPM, deviceTargetRPM) >= minRPMChange
}

func absRPMDelta(a, b int) int {
	delta := a - b
	if delta < 0 {
		return -delta
	}
	return delta
}

// startHealthMonitoring 启动健康监控
func (a *CoreApp) startHealthMonitoring() {
	if a.stopping.Load() {
		return
	}
	a.healthMutex.Lock()
	if a.healthDone != nil {
		a.healthMutex.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	a.healthStop = stop
	a.healthDone = done
	a.healthStopping = false
	a.healthMutex.Unlock()

	a.logInfo("启动健康监控系统")
	a.safeGo("healthMonitoringLoop", func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer func() {
			a.healthMutex.Lock()
			if a.healthDone == done {
				a.healthStop = nil
				a.healthDone = nil
				a.healthStopping = false
			}
			a.healthMutex.Unlock()
			close(done)
		}()
		// 记录的是"上一轮健康检查结束"的时刻而非开始时刻：performHealthCheck 会
		// 做重连、重启桥接等外部调用，可能阻塞数分钟。把这段工作耗时算进 gap 会
		// 让一次卡顿被误判成系统休眠唤醒。Ticker 会丢弃错过的 tick，工作超时后
		// 下一个 tick 立即到达，此时相对"上一轮结束"的间隔接近 0，不会误触发。
		lastHealthCheck := time.Now()

		for {
			select {
			case <-ticker.C:
				gap := time.Since(lastHealthCheck)
				if a.maybeRecoverFromSystemResume("health-monitor", gap, 30*time.Second) {
					lastHealthCheck = time.Now()
					continue
				}

				a.emitHeartbeat()
				a.performHealthCheck()
				lastHealthCheck = time.Now()
			case <-stop:
				a.logInfo("健康监控系统已停止")
				return
			}
		}
	})

	if a.logger != nil {
		a.safeGo("cleanOldLogs", func() {
			a.logger.CleanOldLogs()
		})
	}
}

func (a *CoreApp) stopHealthMonitoring(timeout time.Duration) {
	a.healthMutex.Lock()
	done := a.healthDone
	if done != nil && !a.healthStopping {
		close(a.healthStop)
		a.healthStopping = true
	}
	a.healthMutex.Unlock()

	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(timeout):
		a.logError("等待健康监控停止超时")
	}
}

// emitHeartbeat 周期性向 GUI 广播心跳。
//
// 除了让前端知道核心还活着，这个周期性写入还承担着"探活"职责：写协程带写超时，
// 对端进程假死时这次心跳会写超时失败，从而把该客户端摘除。没有任何周期性写入时，
// 一个假死但未退出的 GUI 会永远留在客户端表里，核心也就永远无法回到空闲低频模式。
func (a *CoreApp) emitHeartbeat() {
	if a.ipcServer == nil || !a.ipcServer.HasClients() {
		return
	}
	a.ipcServer.BroadcastEvent(ipc.EventHeartbeat, time.Now().UnixMilli())
}

// performHealthCheck 执行健康检查
func (a *CoreApp) performHealthCheck() {
	defer func() {
		if r := recover(); r != nil {
			a.logError("健康检查中发生panic: %v", r)
		}
	}()

	a.trayManager.CheckHealth()
	a.ensureTemperatureMonitoringHealthy()
	a.checkDeviceHealth()
	// 散热器重新配对或换用另一台设备会产生新的设备节点，这里持续补齐兼容处理。
	a.ensureFlydigiCompat("health-check")

	a.mutex.RLock()
	connected := a.isConnected
	a.mutex.RUnlock()
	a.logDebug("健康检查完成 - 托盘:%v 设备连接:%v",
		a.trayManager.IsInitialized(), connected)
}

func (a *CoreApp) ensureTemperatureMonitoringHealthy() {
	if a.systemSuspended.Load() {
		return
	}

	a.ensureTemperatureMonitoring("health-check")
}

const (
	// healthReconnectFastAttempts 是健康检查按原有 30 秒节奏连续重连的次数（约 3 分钟）。
	healthReconnectFastAttempts = 6
	// healthReconnectRelaxedInterval 是设备确实不在、且 HID 接口到达通知可用时的重连兜底间隔。
	// 设备插入/重新配对会立刻触发到达通知并重连，这里只是通知丢失时的保底。
	healthReconnectRelaxedInterval = 3 * time.Minute
	// healthReconnectLogSummaryEvery 是连续重连失败时输出 Info 汇总的间隔（按次数）。
	healthReconnectLogSummaryEvery = 20
)

// healthReconnectDue 决定本轮健康检查是否要触发重连。
//
// 只有同时满足以下条件才放慢：HID 接口到达通知已注册（Linux 上是桩实现，保持原节奏）、
// 最近一次尝试连一个受支持的 HID 接口都没枚举到且不涉及 BS1 的 BLE 扫描（relaxable）、
// 并且断线后已经按原节奏重试过 healthReconnectFastAttempts 次。
func healthReconnectDue(streak int32, sinceLast time.Duration, arrivalNotify, relaxable bool) bool {
	if !arrivalNotify || !relaxable || streak < healthReconnectFastAttempts {
		return true
	}
	// 健康检查 30 秒一跳，留出余量，避免计时抖动导致整整多等一跳。
	return sinceLast >= healthReconnectRelaxedInterval-5*time.Second
}

// checkDeviceHealth 检查设备健康状态
func (a *CoreApp) checkDeviceHealth() {
	a.mutex.RLock()
	connected := a.isConnected
	a.mutex.RUnlock()

	if !connected {
		if a.reconnectInProgress.Load() {
			// 断线/唤醒排定的重连流程仍在退避重试，本轮不计入健康检查的连续重连次数。
			a.logDebug("健康检查: 设备未连接，重连流程已在进行中")
			return
		}
		streak := a.healthReconnectStreak.Load()
		sinceLast := time.Duration(math.MaxInt64)
		if last := a.lastHealthReconnectUnixNano.Load(); last != 0 {
			sinceLast = time.Since(time.Unix(0, last))
		}
		arrivalNotify := a.hidArrivalNotifyActive.Load()
		relaxable := arrivalNotify && a.deviceManager.ReconnectPollingRelaxable()
		if !healthReconnectDue(streak, sinceLast, arrivalNotify, relaxable) {
			a.logDebug("健康检查: 设备未连接且未枚举到 HID 接口，由接口到达通知负责发现设备，放慢重连兜底（距上次 %v）",
				sinceLast.Round(time.Second))
			return
		}
		streak = a.healthReconnectStreak.Add(1)
		a.lastHealthReconnectUnixNano.Store(time.Now().UnixNano())
		// 连续失败只有第一次照常记录，之后降到 Debug，并定期输出一条 Info 汇总。
		quiet := streak > 1
		switch {
		case !quiet:
			a.logInfo("健康检查: 设备未连接，尝试重新连接")
		case streak%healthReconnectLogSummaryEvery == 0:
			a.logInfo("健康检查: 设备仍未连接，已连续 %d 次尝试重新连接", streak)
		default:
			a.logDebug("健康检查: 设备未连接，尝试重新连接（连续第 %d 次）", streak)
		}
		a.requestReconnectWithLogging("health-check", []time.Duration{0}, quiet)
	} else {
		a.healthReconnectStreak.Store(0)
		// 验证设备实际连接状态
		if !a.deviceManager.IsConnected() {
			a.logError("健康检查: 检测到设备状态不一致，触发断开回调")
			a.onDeviceDisconnect()
		}
	}
}
