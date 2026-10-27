package guiapp

import (
	"encoding/json"
	"fmt"

	"github.com/TIANLI0/THRM/internal/ipc"
)

// ShowWindow 显示主窗口
func (a *App) ShowWindow() {
	if a.window != nil {
		a.window.Show().SetAlwaysOnTop(false)
	}
}

// HideWindow 隐藏主窗口到托盘
func (a *App) HideWindow() {
	if a.window != nil {
		a.captureWindowState()
		a.window.Hide()
	}
}

// quit 退出 GUI 进程。服务的 ServiceShutdown 会随后执行。
func (a *App) quit() {
	if a.app != nil {
		a.app.Quit()
	}
}

// QuitApp 完全退出应用
func (a *App) QuitApp() {
	guiLogger.Info("GUI 请求退出")

	// 先让看护协程停手，否则它会把随后关闭的连接当成"核心掉线"而尝试重连。
	a.shuttingDown.Store(true)
	a.captureWindowState()

	if a.ipcClient != nil {
		a.ipcClient.Close()
	}

	a.quit()
}

// QuitAll 完全退出应用（包括核心服务）
func (a *App) QuitAll() {
	guiLogger.Info("GUI 请求完全退出（包括核心服务）")

	// 必须在通知核心退出之前置位：否则核心退出与本进程退出之间的窗口里，
	// 看护协程会把核心当成掉线并把它重新拉起来。
	a.shuttingDown.Store(true)
	a.captureWindowState()

	a.requestCoreQuit()

	if a.ipcClient != nil {
		a.ipcClient.Close()
	}

	a.quit()
}

// requestCoreQuit 通知核心退出。只用现有连接，断开时至多直连一次仍在运行的核心；
// 不走 sendRequest，因为那条路径在未连接时会 EnsureCoreServiceRunning——
// 为了发一条退出指令先把核心拉起来毫无意义。
func (a *App) requestCoreQuit() {
	if a.ipcClient == nil {
		return
	}
	if !a.ipcClient.IsConnected() {
		if err := a.ipcClient.Connect(); err != nil {
			guiLogger.Infof("核心服务未连接，跳过退出通知: %v", err)
			return
		}
	}
	timeout, _ := ipcRequestPolicy(ipc.ReqQuitApp)
	if _, err := a.ipcClient.SendRequestWithTimeout(ipc.ReqQuitApp, nil, timeout); err != nil {
		guiLogger.Warnf("通知核心服务退出失败: %v", err)
	}
}

// UpdateGuiResponseTime 更新GUI响应时间（供前端调用）
func (a *App) UpdateGuiResponseTime() error {
	_, err := a.sendRequest(ipc.ReqUpdateGuiResponseTime, nil)
	return err
}

// GetDebugInfo 获取调试信息
func (a *App) GetDebugInfo() map[string]any {
	resp, err := a.sendRequest(ipc.ReqGetDebugInfo, nil)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var info map[string]any
	json.Unmarshal(resp.Data, &info)
	return info
}

// SetDebugMode 设置调试模式
func (a *App) SetDebugMode(enabled bool) error {
	resp, err := a.sendRequest(ipc.ReqSetDebugMode, ipc.SetBoolParams{Enabled: enabled})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func (a *App) SendDeviceDebugCommand(hexCommand string, waitMs int) (DeviceDebugCommandResult, error) {
	resp, err := a.sendRequest(ipc.ReqSendDeviceDebugCommand, ipc.DeviceDebugCommandParams{Hex: hexCommand, WaitMs: waitMs})
	if err != nil {
		return DeviceDebugCommandResult{}, err
	}
	if !resp.Success {
		return DeviceDebugCommandResult{}, fmt.Errorf("%s", resp.Error)
	}
	var result DeviceDebugCommandResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return DeviceDebugCommandResult{}, err
	}
	return result, nil
}

func (a *App) GetDeviceDebugFrames() []DeviceDebugFrame {
	resp, err := a.sendRequest(ipc.ReqGetDeviceDebugFrames, nil)
	if err != nil || !resp.Success {
		return nil
	}
	var frames []DeviceDebugFrame
	json.Unmarshal(resp.Data, &frames)
	return frames
}
