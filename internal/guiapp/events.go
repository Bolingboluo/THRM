package guiapp

import (
	"encoding/json"

	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/plugins/fnqpowermode"
	"github.com/TIANLI0/THRM/internal/types"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// 只由 GUI 自己产生、核心不知道的事件。
const (
	eventCoreServiceError = "core-service-error"
	eventCoreServiceOK    = "core-service-ok"
	// eventCoreResynced 由 IPC 看护协程在重连成功后发出，提示前端重取核心状态。
	eventCoreResynced = "core-resynced"
	// eventUpdateProgress 汇报应用更新安装包的下载进度。
	eventUpdateProgress = "update-download-progress"
)

// LegionFnQSupport 是 legion-fnq-support-update 事件的载荷。
type LegionFnQSupport struct {
	Supported bool `json:"supported"`
}

// HotkeyTriggered 是 hotkey-triggered 事件的载荷。
type HotkeyTriggered struct {
	Action   string `json:"action"`
	Shortcut string `json:"shortcut"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

// FirmwareFlashEvent 是 firmware-update 事件的载荷（与 coreapp 同构，经 IPC 传输）。
type FirmwareFlashEvent struct {
	Kind    string `json:"kind"`    // log | progress | done
	Phase   string `json:"phase"`   // write | verify
	Done    int    `json:"done"`    // progress: 已处理字节
	Total   int    `json:"total"`   // progress: 总字节
	Message string `json:"message"` // log/done 文本
	Success bool   `json:"success"` // done: 是否成功
}

// 所有推给前端的事件都在这里声明载荷类型。绑定生成器据此生成强类型的 TS 事件定义
// （Events.On 回调里的 ev.data 直接带类型）；Emit 时 Wails 也会按类型校验，
// 载荷类型对不上的事件会被丢弃并记一条错误，因此 handleCoreEvent 必须先解码再转发。
// 生成器只认 init 里以常量为参数的直接调用。
func init() {
	application.RegisterEvent[types.FanData](ipc.EventFanDataUpdate)
	application.RegisterEvent[types.TemperatureData](ipc.EventTemperatureUpdate)
	application.RegisterEvent[types.TemperatureHistoryPoint](ipc.EventTemperatureHistoryUpdate)
	application.RegisterEvent[types.TimelineEvent](ipc.EventTimelineEvent)
	application.RegisterEvent[FirmwareFlashEvent](ipc.EventFirmwareUpdate)
	application.RegisterEvent[map[string]any](ipc.EventDeviceConnected)
	application.RegisterEvent[application.Void](ipc.EventDeviceDisconnected)
	application.RegisterEvent[string](ipc.EventDeviceError)
	application.RegisterEvent[types.DeviceSettings](ipc.EventDeviceSettingsUpdate)
	application.RegisterEvent[types.AppConfig](ipc.EventConfigUpdate)
	application.RegisterEvent[types.SmartTempLightStatus](ipc.EventSmartLightUpdate)
	application.RegisterEvent[HotkeyTriggered](ipc.EventHotkeyTriggered)
	application.RegisterEvent[fnqpowermode.PowerModeState](ipc.EventLegionPowerModeUpdate)
	application.RegisterEvent[LegionFnQSupport](ipc.EventLegionFnQSupportUpdate)
	application.RegisterEvent[FlydigiCompatStatus](ipc.EventFlydigiCompatUpdate)

	application.RegisterEvent[string](eventCoreServiceError)
	application.RegisterEvent[application.Void](eventCoreServiceOK)
	application.RegisterEvent[application.Void](eventCoreResynced)
	application.RegisterEvent[UpdateProgress](eventUpdateProgress)
}

// emit 向前端推送事件；Wails 尚未就绪（或在单元测试里）时直接丢弃。
// 无载荷的事件（application.Void）不传 data。
func (a *App) emit(name string, data ...any) {
	if a.app == nil {
		return
	}
	a.app.Event.Emit(name, data...)
}

// forwardCoreEvent 把核心推来的 JSON 载荷解码成注册时声明的类型再转发。
// 解码失败说明核心与 GUI 的协议版本对不上，记日志后丢弃，免得把残缺数据交给前端。
func forwardCoreEvent[T any](a *App, name string, raw json.RawMessage) {
	var payload T
	if err := json.Unmarshal(raw, &payload); err != nil {
		guiLogger.Warnf("解析核心事件 %s 失败: %v", name, err)
		return
	}
	a.emit(name, payload)
}
