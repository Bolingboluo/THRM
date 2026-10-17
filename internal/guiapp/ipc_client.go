package guiapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/types"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ipcRequestPolicy 返回请求的超时与"重连后是否可自动重发"。
//
// 长请求的超时必须高于核心侧自身的最坏耗时，否则核心仍在干活时 GUI 就先报失败：
// ReinstallPawnIO 最坏是卸载 90s + 安装 90s 再加重启桥接；RestartPawnIO 要重启驱动
// 并重新初始化桥接；桥接探测/状态查询在桥接自愈重启期间也会被拖慢。
func ipcRequestPolicy(reqType ipc.RequestType) (time.Duration, bool) {
	switch reqType {
	case ipc.ReqGetDeviceStatus, ipc.ReqGetCurrentFanData, ipc.ReqGetConfig,
		ipc.ReqGetFanCurve, ipc.ReqGetFanCurveProfiles, ipc.ReqGetTemperature,
		ipc.ReqGetTemperatureHistory,
		ipc.ReqGetDebugInfo, ipc.ReqGetDeviceDebugFrames, ipc.ReqGetSmartLightStatus,
		ipc.ReqPing:
		return 6 * time.Second, true
	case ipc.ReqGetBridgeProgramStatus:
		return 15 * time.Second, true
	case ipc.ReqConnect, ipc.ReqTestBridgeProgram:
		return 30 * time.Second, false
	case ipc.ReqRestartPawnIO:
		return 90 * time.Second, false
	case ipc.ReqReinstallPawnIO:
		return 240 * time.Second, false
	case ipc.ReqUpdateGuiResponseTime:
		return ipcRegularRequestTimeout, true
	default:
		return ipcRegularRequestTimeout, false
	}
}

const (
	ipcRegularRequestTimeout     = 12 * time.Second
	ipcTimeoutReconnectThreshold = 3
	ipcTimeoutReconnectWindow    = 30 * time.Second
)

// ipcTimeoutCountsTowardReconnect 判断一次请求超时是否计入连续超时重连阈值。
//
// 超时上限高于常规请求的那批（设备连接、PawnIO 重启/重装、桥接探测）在核心侧本来就
// 可能执行很久，它们超时只说明那项操作慢，不代表连接失效；计入阈值的话，一次重装
// PawnIO 超时再叠加两次普通抖动，就会拆掉一条健康的连接。
func ipcTimeoutCountsTowardReconnect(timeout time.Duration) bool {
	return timeout <= ipcRegularRequestTimeout
}

// errGUIShuttingDown 表示 GUI 已进入退出流程，不再向核心服务发请求。
var errGUIShuttingDown = errors.New("GUI 正在退出，已停止向核心服务发送请求")

func shouldReconnectAfterIPCError(err error, consecutiveTimeouts uint32) bool {
	if !errors.Is(err, ipc.ErrRequestTimeout) {
		return true
	}
	return consecutiveTimeouts >= ipcTimeoutReconnectThreshold
}

func (a *App) recordIPCTimeout(now time.Time) uint32 {
	a.ipcHealthMutex.Lock()
	defer a.ipcHealthMutex.Unlock()
	if a.lastIPCTimeout.IsZero() || now.Sub(a.lastIPCTimeout) > ipcTimeoutReconnectWindow {
		a.ipcTimeouts = 0
	}
	a.ipcTimeouts++
	a.lastIPCTimeout = now
	return a.ipcTimeouts
}

func (a *App) resetIPCTimeouts() {
	a.ipcHealthMutex.Lock()
	a.ipcTimeouts = 0
	a.lastIPCTimeout = time.Time{}
	a.ipcHealthMutex.Unlock()
}

func mergeTemperatureMetadata(previous, incoming types.TemperatureData) types.TemperatureData {
	merged := incoming
	if merged.CpuSensors == nil {
		merged.CpuSensors = previous.CpuSensors
	}
	if merged.GpuSensors == nil {
		merged.GpuSensors = previous.GpuSensors
	}
	if merged.CpuPowerSensors == nil {
		merged.CpuPowerSensors = previous.CpuPowerSensors
	}
	if merged.GpuPowerSensors == nil {
		merged.GpuPowerSensors = previous.GpuPowerSensors
	}
	if merged.GpuDevices == nil {
		merged.GpuDevices = previous.GpuDevices
	}
	if merged.CpuModel == "" {
		merged.CpuModel = previous.CpuModel
	}
	if merged.GpuModel == "" {
		merged.GpuModel = previous.GpuModel
	}
	if merged.SelectedGpuDevice == "" {
		merged.SelectedGpuDevice = previous.SelectedGpuDevice
	}
	return merged
}

func coreServiceUnavailableMessage(detail string) string {
	if detail == "" {
		return fmt.Sprintf("核心服务不可用。请检查 %s 是否仍在安装目录中，或是否被安全软件隔离。", appmeta.CoreExecutableName)
	}
	return fmt.Sprintf("核心服务不可用：%s。请检查 %s 是否仍在安装目录中，或是否被安全软件隔离。", detail, appmeta.CoreExecutableName)
}

func (a *App) emitCoreServiceError(detail string) {
	if a.ctx == nil {
		return
	}
	a.coreStatusMutex.Lock()
	defer a.coreStatusMutex.Unlock()
	a.coreServiceOK = false
	runtime.EventsEmit(a.ctx, "core-service-error", coreServiceUnavailableMessage(detail))
}

// emitCoreServiceOK 只在状态跳变（启动后首次成功、从异常恢复）时通知前端。
// 前端只关心这次跳变（据此重新同步），此前每个成功请求都发一次，
// 白白给 webview 灌了大量无用事件。
func (a *App) emitCoreServiceOK() {
	if a.ctx == nil {
		return
	}
	a.coreStatusMutex.Lock()
	defer a.coreStatusMutex.Unlock()
	if !a.markCoreServiceOKLocked() {
		return
	}
	runtime.EventsEmit(a.ctx, "core-service-ok", nil)
}

// markCoreServiceOKLocked 把状态置为正常，返回这是否是一次跳变。调用方须持有 coreStatusMutex。
func (a *App) markCoreServiceOKLocked() bool {
	if a.coreServiceOK {
		return false
	}
	a.coreServiceOK = true
	return true
}

// markCoreServiceDegraded 记录一次未通知前端的请求失败（如未达重连阈值的超时）。
// 前端可能已从被拒绝的 Promise 自行推断出核心异常，下一次成功必须再发 core-service-ok
// 让它恢复，否则会一直停在错误态。
func (a *App) markCoreServiceDegraded() {
	a.coreStatusMutex.Lock()
	a.coreServiceOK = false
	a.coreStatusMutex.Unlock()
}

// handleCoreEvent 处理核心服务推送的事件
func (a *App) handleCoreEvent(event ipc.Event) {
	if a.ctx == nil {
		return
	}

	switch event.Type {
	// 三类高频遥测在窗口隐藏（已退订）时不再转给 webview；恢复显示时 GUI 会主动补一份
	// 完整状态。核心侧也已按订阅过滤，这里兜住旧版核心以及退订生效前的那几帧。
	// 风扇数据与历史点是核心用同一份结构体 json.Marshal 出来的，原样转发与"解码成结构体
	// 再由 Wails 编码"得到的 JSON 完全相同，省掉一次解码与编码。
	case ipc.EventFanDataUpdate:
		if a.coreEventsPaused() || len(event.Data) == 0 {
			return
		}
		runtime.EventsEmit(a.ctx, "fan-data-update", event.Data)

	case ipc.EventTemperatureUpdate:
		var temp ipc.TemperatureEvent
		if err := json.Unmarshal(event.Data, &temp); err == nil {
			// 隐藏期间照样并入缓存，传感器清单的变化不会因此漏掉。
			merged, ok := a.mergeTemperatureEvent(temp)
			if a.coreEventsPaused() {
				return
			}
			if !ok {
				a.resyncTemperatureCatalog()
			}
			runtime.EventsEmit(a.ctx, "temperature-update", merged)
		}

	case ipc.EventTemperatureHistoryUpdate:
		if a.coreEventsPaused() || len(event.Data) == 0 {
			return
		}
		runtime.EventsEmit(a.ctx, "temperature-history-update", event.Data)

	case ipc.EventTimelineEvent:
		var timelineEvent types.TimelineEvent
		if err := json.Unmarshal(event.Data, &timelineEvent); err == nil {
			runtime.EventsEmit(a.ctx, "timeline-event", timelineEvent)
		}

	case ipc.EventDeviceConnected:
		// 载荷里除了字符串字段还嵌着 deviceSettings 对象，按 map[string]string 解码
		// 会整体失败，前端每次连接都拿不到设备设置。
		var deviceInfo map[string]any
		json.Unmarshal(event.Data, &deviceInfo)
		a.mutex.Lock()
		a.isConnected = true
		a.mutex.Unlock()
		runtime.EventsEmit(a.ctx, "device-connected", deviceInfo)

	case ipc.EventDeviceDisconnected:
		a.mutex.Lock()
		a.isConnected = false
		a.mutex.Unlock()
		runtime.EventsEmit(a.ctx, "device-disconnected", nil)

	case ipc.EventDeviceError:
		var errMsg string
		json.Unmarshal(event.Data, &errMsg)
		runtime.EventsEmit(a.ctx, "device-error", errMsg)

	case ipc.EventConfigUpdate:
		var cfg types.AppConfig
		if err := json.Unmarshal(event.Data, &cfg); err == nil {
			runtime.EventsEmit(a.ctx, "config-update", cfg)
		}

	case ipc.EventSmartLightUpdate:
		var status types.SmartTempLightStatus
		if err := json.Unmarshal(event.Data, &status); err == nil {
			runtime.EventsEmit(a.ctx, "smart-light-update", status)
		}

	case ipc.EventHotkeyTriggered:
		var payload map[string]any
		if err := json.Unmarshal(event.Data, &payload); err == nil {
			runtime.EventsEmit(a.ctx, "hotkey-triggered", payload)
		}

	case ipc.EventLegionPowerModeUpdate:
		var payload map[string]any
		if err := json.Unmarshal(event.Data, &payload); err == nil {
			runtime.EventsEmit(a.ctx, "legion-power-mode-update", payload)
		}

	case ipc.EventHealthPing, ipc.EventHeartbeat:
		// 心跳只用于核心侧探活（写超时即摘除假死的客户端），前端没有任何监听者，
		// 不再每 30 秒往 webview 推一次。必须保留这个分支，否则会落进 default 被原样转发。

	case "show-window":
		a.ShowWindow()

	case "quit":
		a.QuitApp()

	default:
		// 其余事件（device-settings-update、legion-fnq-support-update、flydigi-compat-update 等）
		// 由前端直接消费，GUI 无需解读，原样转发。此前这里漏掉的事件会被静默丢弃，
		// 核心新增事件时也不必再同步修改 GUI。
		runtime.EventsEmit(a.ctx, event.Type, event.Data)
	}
}

// mergeCurrentTemp 把新到的温度数据并入缓存并返回合并结果。
// 核心推送的温度事件会省略未变化的传感器清单，缓存必须先由完整快照
// （GetTemperature / GetDeviceStatus）播种，否则前端会拿到被清空的清单。
func (a *App) mergeCurrentTemp(temp types.TemperatureData) types.TemperatureData {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.currentTemp = mergeTemperatureMetadata(a.currentTemp, temp)
	return a.currentTemp
}

// sendRequest 发送请求到核心服务
func (a *App) sendRequest(reqType ipc.RequestType, data any) (*ipc.Response, error) {
	// 退出流程中不再发请求：一次恰好在途的前端调用会走到下面的 EnsureCoreServiceRunning，
	// 把 QuitAll 刚让其退出的核心重新拉起来。
	if a.shuttingDown.Load() {
		return nil, errGUIShuttingDown
	}
	timeout, retryable := ipcRequestPolicy(reqType)
	if !a.ipcClient.IsConnected() {
		if !EnsureCoreServiceRunning() {
			err := fmt.Errorf("核心服务未运行且启动失败")
			a.emitCoreServiceError(err.Error())
			return nil, err
		}
		if err := a.ipcClient.Connect(); err != nil {
			wrapped := fmt.Errorf("未连接到核心服务: %v", err)
			a.emitCoreServiceError(wrapped.Error())
			return nil, wrapped
		}
		a.ipcClient.SetEventHandler(a.handleCoreEvent)
		go a.syncCoreEventSubscription()
		a.emitCoreServiceOK()
	}

	resp, gen, err := a.ipcClient.SendRequestTracked(reqType, data, timeout)
	if err == nil {
		a.resetIPCTimeouts()
		a.emitCoreServiceOK()
		return resp, nil
	}

	timeoutCount := uint32(0)
	if errors.Is(err, ipc.ErrRequestTimeout) && ipcTimeoutCountsTowardReconnect(timeout) {
		timeoutCount = a.recordIPCTimeout(time.Now())
	}
	if !shouldReconnectAfterIPCError(err, timeoutCount) {
		guiLogger.Warnf("IPC 请求超时，保留当前连接等待恢复（%d/%d）: %v",
			timeoutCount, ipcTimeoutReconnectThreshold, err)
		a.markCoreServiceDegraded()
		return nil, err
	}
	a.resetIPCTimeouts()
	// 请求在途期间 GUI 可能已开始退出，此时同样不能去拉起核心。
	if a.shuttingDown.Load() {
		return nil, err
	}

	guiLogger.Warnf("IPC 请求失败，尝试重新连接核心服务后重试: %v", err)
	// 只拆掉这次请求所在的连接。直接 Close() 关的是"当前"连接，
	// 而看护协程或并发请求可能已经在这段时间里重连成功了。
	a.ipcClient.CloseGeneration(gen)
	if !EnsureCoreServiceRunning() {
		wrapped := fmt.Errorf("核心服务连接断开且重新启动失败: %v", err)
		a.emitCoreServiceError(wrapped.Error())
		return nil, wrapped
	}
	if connectErr := a.ipcClient.Connect(); connectErr != nil {
		wrapped := fmt.Errorf("重新连接核心服务失败: %v；原始错误: %v", connectErr, err)
		a.emitCoreServiceError(wrapped.Error())
		return nil, wrapped
	}
	a.ipcClient.SetEventHandler(a.handleCoreEvent)
	go a.syncCoreEventSubscription()
	a.resetIPCTimeouts()
	if !retryable {
		wrapped := fmt.Errorf("request %s may have been applied before IPC disconnected; state was not replayed automatically: %v", reqType, err)
		a.emitCoreServiceError(wrapped.Error())
		return nil, wrapped
	}

	resp, err = a.ipcClient.SendRequestWithTimeout(reqType, data, timeout)
	if err != nil {
		a.emitCoreServiceError(err.Error())
		return nil, err
	}
	a.resetIPCTimeouts()
	a.emitCoreServiceOK()
	return resp, nil
}
