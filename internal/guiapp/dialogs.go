package guiapp

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var errAppNotReady = errors.New("application is not ready")

// dialogParent 返回对话框的父窗口。直接把 nil 的 *WebviewWindow 赋给 Window 接口
// 会得到一个非 nil 的接口值，Wails 会拿它去取窗口句柄，所以必须显式返回 nil 接口。
func (a *App) dialogParent() application.Window {
	if a.window == nil {
		return nil
	}
	return a.window
}

// promptSaveFile 弹出挂在主窗口上的原生保存对话框。用户取消时返回空路径且不报错。
func (a *App) promptSaveFile(options application.SaveFileDialogOptions) (string, error) {
	if a.app == nil {
		return "", errAppNotReady
	}
	options.CanCreateDirectories = true
	options.Window = a.dialogParent()
	return dialogSelection(a.app.Dialog.SaveFileWithOptions(&options).PromptForSingleSelection())
}

// promptOpenFile 弹出挂在主窗口上的原生打开文件对话框。用户取消时返回空路径且不报错。
func (a *App) promptOpenFile(options application.OpenFileDialogOptions) (string, error) {
	if a.app == nil {
		return "", errAppNotReady
	}
	options.CanChooseFiles = true
	options.Window = a.dialogParent()
	return dialogSelection(a.app.Dialog.OpenFileWithOptions(&options).PromptForSingleSelection())
}

// dialogCancelledMessage 是 Windows 文件对话框被取消时 Wails 透传的错误文本。
// 该错误定义在 Wails 的 internal 包里，没法用 errors.Is 判断，只能比对文本。
const dialogCancelledMessage = "cancelled by user"

// dialogSelection 把"用户取消"统一成空路径、无错误；其他平台取消时本来就是这样。
func dialogSelection(path string, err error) (string, error) {
	if err != nil && err.Error() == dialogCancelledMessage {
		return "", nil
	}
	return path, err
}
