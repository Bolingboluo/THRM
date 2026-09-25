package guiapp

import (
	"encoding/json"
	"fmt"

	"github.com/TIANLI0/THRM/internal/ipc"
)

// GetTemperature 获取当前温度。
//
// 返回的完整快照同时用来播种 a.currentTemp：核心推送的温度事件会省略未变化的
// 传感器清单，缓存为空时合并不出任何东西，GUI 打开后的头几帧事件就会把前端的
// 传感器清单冲成 null。
func (a *App) GetTemperature() TemperatureData {
	resp, err := a.sendRequest(ipc.ReqGetTemperature, nil)
	if err != nil || !resp.Success {
		a.mutex.RLock()
		defer a.mutex.RUnlock()
		return a.currentTemp
	}
	var temp TemperatureData
	if err := json.Unmarshal(resp.Data, &temp); err != nil {
		guiLogger.Errorf("解析温度数据失败: %v", err)
		a.mutex.RLock()
		defer a.mutex.RUnlock()
		return a.currentTemp
	}
	return a.mergeCurrentTemp(temp)
}

// GetTemperatureHistory 获取核心服务记录的温度历史。
func (a *App) GetTemperatureHistory() TemperatureHistoryPayload {
	resp, err := a.sendRequest(ipc.ReqGetTemperatureHistory, nil)
	if err != nil || !resp.Success {
		return TemperatureHistoryPayload{}
	}
	var payload TemperatureHistoryPayload
	json.Unmarshal(resp.Data, &payload)
	return payload
}

// SetTemperatureHistoryEnabled 设置后台历史记录开关。
func (a *App) SetTemperatureHistoryEnabled(enabled bool) error {
	resp, err := a.sendRequest(ipc.ReqSetTemperatureHistoryEnabled, ipc.SetBoolParams{Enabled: enabled})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// SetTemperatureHistoryRetentionHours 设置后台历史保留时长(小时)。
func (a *App) SetTemperatureHistoryRetentionHours(hours int) error {
	resp, err := a.sendRequest(ipc.ReqSetTemperatureHistoryRetentionHours, ipc.SetIntParams{Value: hours})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

// TestTemperatureReading 测试温度读取
func (a *App) TestTemperatureReading() TemperatureData {
	resp, err := a.sendRequest(ipc.ReqTestTemperatureReading, nil)
	if err != nil {
		return TemperatureData{}
	}
	var temp TemperatureData
	json.Unmarshal(resp.Data, &temp)
	return temp
}
