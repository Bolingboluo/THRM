//go:build !windows

package guiapp

// SetWindowDarkMode 同步窗口的 DWM 沉浸式深色模式属性，让标题栏与 Mica/Acrylic
// 材质的明暗跟随应用内主题，而不是系统主题。
func (a *App) SetWindowDarkMode(dark bool) {
	// DWM 只有 Windows 有，其它平台什么都不做。
	// 上面的文档注释必须与 window_theme_windows.go 一字不差：绑定生成器会把它写进
	// frontend/bindings，CI 在 Linux 上重新生成并校验，两边不同就会判定绑定过期。
}
