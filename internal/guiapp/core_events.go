package guiapp

import (
	"bytes"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/types"
)

// coreEventSubscriptionTimeout 是订阅/退订请求的超时。核心直接在 IPC 层处理它们，
// 不经过业务逻辑，正常情况下毫秒级返回。
const coreEventSubscriptionTimeout = 5 * time.Second

// coreEventSubscription 记录前端是否需要高频遥测（风扇、温度、温度历史）。
//
// 窗口隐藏（最小化或收进托盘）且没有测试在跑时前端会把它关掉：GUI 不再把高频事件
// 转给 webview，同时向核心退订，核心便按"没有前台 GUI"回到后台低频采样。
type coreEventSubscription struct {
	// paused 为 true 表示前端当前不需要高频遥测。零值即"需要"，与核心对新连接的默认一致。
	paused atomic.Bool
	// mu 串行化订阅请求：核心并发处理同一连接上的请求，只有等上一条返回再发下一条，
	// 快速连续的隐藏/显示才能按顺序生效。
	mu sync.Mutex
	// resyncing 防止温度清单对不上时重复发起完整快照拉取。
	resyncing atomic.Bool
}

// SetCoreEventSubscription 由前端在窗口可见性变化（以及后台测试开始/结束）时调用。
// active=false 表示窗口不可见且没有需要实时遥测的测试在跑。
func (a *App) SetCoreEventSubscription(active bool) {
	if a.coreEvents.paused.Swap(!active) == !active {
		return
	}
	a.syncCoreEventSubscription()
	if active {
		// 隐藏期间前端没收到任何遥测，恢复显示时立即补一份完整状态（含传感器清单），
		// 不必等核心的下一个采样周期。
		a.emitTelemetrySnapshot()
	}
}

// syncCoreEventSubscription 把当前的订阅意愿同步给核心。
//
// 每次（重新）连上核心后也要调用：核心把每条新连接都当作已订阅（兼容不发订阅请求的
// 旧 GUI），窗口隐藏期间发生重连的话需要再退订一次；订阅请求同时声明本 GUI 能还原的
// 协议能力。旧版核心不认识这两个请求，会回一个失败响应，此时 GUI 侧的事件过滤照常生效。
func (a *App) syncCoreEventSubscription() {
	a.coreEvents.mu.Lock()
	defer a.coreEvents.mu.Unlock()
	if a.shuttingDown.Load() || !a.ipcClient.IsConnected() {
		return
	}
	var err error
	if a.coreEvents.paused.Load() {
		_, err = a.ipcClient.SendRequestWithTimeout(ipc.ReqUnsubscribeEvents, nil, coreEventSubscriptionTimeout)
	} else {
		params := ipc.SubscribeEventsParams{Features: []string{ipc.FeatureTemperatureSensorValues}}
		_, err = a.ipcClient.SendRequestWithTimeout(ipc.ReqSubscribeEvents, params, coreEventSubscriptionTimeout)
	}
	if err != nil {
		// 不在这里重连：IPC 看护协程会发现断线并重连，重连后会再同步一次。
		guiLogger.Warnf("同步核心事件订阅失败: %v", err)
	}
}

// coreEventsPaused 报告前端当前是否不需要高频遥测。
func (a *App) coreEventsPaused() bool {
	return a.coreEvents.paused.Load()
}

// emitTelemetrySnapshot 拉取核心的完整温度快照与当前风扇数据并推给前端。
func (a *App) emitTelemetrySnapshot() {
	a.emitTemperatureSnapshot()

	if a.app == nil {
		return
	}
	resp, err := a.sendRequest(ipc.ReqGetCurrentFanData, nil)
	if err != nil || resp == nil || !resp.Success || isJSONNull(resp.Data) {
		return
	}
	forwardCoreEvent[types.FanData](a, ipc.EventFanDataUpdate, resp.Data)
}

// emitTemperatureSnapshot 用核心的完整温度快照刷新缓存并推给前端。
func (a *App) emitTemperatureSnapshot() {
	if a.app == nil {
		return
	}
	resp, err := a.sendRequest(ipc.ReqGetTemperature, nil)
	if err != nil || resp == nil || !resp.Success {
		return
	}
	var temp types.TemperatureData
	if err := json.Unmarshal(resp.Data, &temp); err != nil {
		return
	}
	a.emit(ipc.EventTemperatureUpdate, a.mergeCurrentTemp(temp))
}

// mergeTemperatureEvent 还原温度事件并并入缓存。ok=false 表示有一组传感器读数
// 因本地清单与核心对不上而没能套用（缓存里仍是旧读数），调用方应拉取完整快照。
func (a *App) mergeTemperatureEvent(event ipc.TemperatureEvent) (types.TemperatureData, bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	incoming, ok := ipc.ResolveTemperatureEvent(event, a.currentTemp)
	a.currentTemp = mergeTemperatureMetadata(a.currentTemp, incoming)
	return a.currentTemp, ok
}

// resyncTemperatureCatalog 在后台拉一次完整温度快照，修复缓存里过期的传感器清单。
func (a *App) resyncTemperatureCatalog() {
	if !a.coreEvents.resyncing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer a.coreEvents.resyncing.Store(false)
		defer func() {
			if r := recover(); r != nil {
				guiLogger.Errorf("重新同步温度传感器清单时发生 panic: %v", r)
			}
		}()
		a.emitTemperatureSnapshot()
	}()
}

func isJSONNull(data json.RawMessage) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
