package guiapp

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// ServiceStartup 由 Wails 在启动服务时调用，早于窗口真正创建。
func (a *App) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	a.app = application.Get()
	// 连接、探测核心最坏要等十几秒，放到后台，不拖住窗口创建。
	go a.startup()
	return nil
}

// ServiceShutdown 由 Wails 在应用退出时调用。不论走哪条路退出（关窗、托盘、QuitAll、
// 系统注销），都先让看护协程停手再断开 IPC，免得退出途中把断线当成"核心掉线"去重连。
func (a *App) ServiceShutdown() error {
	a.shuttingDown.Store(true)
	if a.ipcClient != nil {
		a.ipcClient.Close()
	}
	return nil
}

// AttachWindow 由 main 在创建主窗口之后、Run 之前调用。
//
//wails:ignore
func (a *App) AttachWindow(window *application.WebviewWindow) {
	a.window = window

	// 前端运行时就绪时窗口一定已经建好，此时再校验恢复出来的位置。
	// 页面重新加载也会触发这个事件，只需要第一次。
	var placed sync.Once
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		placed.Do(a.keepWindowOnScreen)
	})

	// 标题栏按钮、Alt+F4、任务栏关闭都会走到这里：记下窗口几何信息，不拦截关闭。
	window.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.captureWindowState()
	})
}
