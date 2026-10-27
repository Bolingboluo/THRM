//go:build !windows

package guiapp

// SetWindowDarkMode 同步窗口的 DWM 沉浸式深色模式属性；只有 Windows 有这个概念。
func (a *App) SetWindowDarkMode(dark bool) {}
