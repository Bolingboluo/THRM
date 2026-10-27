//go:build windows

package guiapp

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// SetWindowDarkMode 同步窗口的 DWM 沉浸式深色模式属性，让标题栏与 Mica/Acrylic
// 材质的明暗跟随应用内主题，而不是系统主题。
func (a *App) SetWindowDarkMode(dark bool) {
	if a.window == nil {
		return
	}
	application.InvokeSync(func() {
		if hwnd := uintptr(a.window.NativeWindow()); hwnd != 0 {
			w32.SetTheme(hwnd, dark)
		}
	})
}
