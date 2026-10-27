package guiapp

import (
	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/theme"
	"github.com/TIANLI0/THRM/internal/types"
	"github.com/TIANLI0/THRM/internal/version"
)

// New 创建 GUI 应用实例
func New(themeManager *theme.Manager) *App {
	if themeManager != nil {
		themeManager.EnsureSeeded()
	}
	return &App{
		ipcClient:    ipc.NewClient(nil),
		currentTemp:  types.TemperatureData{BridgeOk: true},
		themeManager: themeManager,
	}
}

// startup 连接核心服务，由 ServiceStartup 在后台协程里调用。
func (a *App) startup() {
	guiLogger.Infof("=== %s GUI 启动 ===", appmeta.AppName)

	// 看护协程负责在核心重启/连接失效后自动重连并让前端重新同步状态，
	// 这样"连接是否还活着"不再依赖前端恰好有请求在飞。
	// 必须在下面的早退分支之前启动：核心此刻不可用恰恰是最需要它的场景。
	a.startIPCWatchdog()

	if !EnsureCoreServiceRunning() {
		guiLogger.Error("核心服务不可用，GUI 将进入受限状态")
		a.emitCoreServiceError("启动或探测核心服务失败")
		guiLogger.Infof("=== %s GUI 启动完成 ===", appmeta.AppName)
		return
	}

	if err := a.ipcClient.Connect(); err != nil {
		guiLogger.Errorf("连接核心服务失败: %v", err)
		a.emitCoreServiceError(err.Error())
	} else {
		guiLogger.Info("已连接到核心服务")
		a.ipcClient.SetEventHandler(a.handleCoreEvent)
		go a.syncCoreEventSubscription()
		if resp, gen, pingErr := a.ipcClient.SendRequestTracked(ipc.ReqPing, nil, 0); pingErr != nil || resp == nil || !resp.Success {
			if pingErr != nil {
				guiLogger.Errorf("核心服务 Ping 失败: %v", pingErr)
				a.emitCoreServiceError(pingErr.Error())
			} else {
				guiLogger.Errorf("核心服务 Ping 返回异常: %+v", resp)
				a.emitCoreServiceError("核心服务 Ping 返回异常")
			}
			// 只关 Ping 所在的连接，看护协程此时可能已经换上了新连接。
			a.ipcClient.CloseGeneration(gen)
		} else {
			a.emitCoreServiceOK()
		}
	}

	guiLogger.Infof("=== %s GUI 启动完成 ===", appmeta.AppName)
}

// GetAppVersion 返回应用版本号（来自版本模块）
func (a *App) GetAppVersion() string {
	return version.Get()
}
