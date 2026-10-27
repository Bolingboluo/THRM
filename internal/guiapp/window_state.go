package guiapp

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const windowStateFileName = "window.json"

const (
	defaultWindowWidth  = 1024
	defaultWindowHeight = 768
	minWindowWidth      = 800
	minWindowHeight     = 600
	windowPositionUnset = -1
)

// WindowState 记录窗口尺寸、位置与最大化状态。
type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximised bool `json:"maximised"`
}

// DefaultWindowState 返回默认窗口状态（居中、默认尺寸）。
func DefaultWindowState() WindowState {
	return WindowState{
		Width:     defaultWindowWidth,
		Height:    defaultWindowHeight,
		X:         windowPositionUnset,
		Y:         windowPositionUnset,
		Maximised: false,
	}
}

// windowStatePath 返回用户配置目录中的窗口状态文件路径。安装目录在 Linux
// 软件包中是只读的，因此无法解析用户目录时宁可禁用持久化，也不写可执行文件旁边。
func windowStatePath() string {
	homeDir, _ := os.UserHomeDir()
	configDir := appmeta.UserConfigDir(homeDir)
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, windowStateFileName)
}

// LoadWindowState 读取持久化的窗口状态；不存在或损坏时返回默认值。
func LoadWindowState() WindowState {
	state := DefaultWindowState()

	path := windowStatePath()
	if path == "" {
		return state
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}

	var loaded WindowState
	if err := json.Unmarshal(data, &loaded); err != nil {
		mainLogger.Warnf("窗口状态文件解析失败，使用默认值: %v", err)
		return state
	}

	if loaded.Width >= minWindowWidth {
		state.Width = loaded.Width
	}
	if loaded.Height >= minWindowHeight {
		state.Height = loaded.Height
	}
	state.X = loaded.X
	state.Y = loaded.Y
	state.Maximised = loaded.Maximised
	return state
}

// saveWindowState 将窗口状态写入磁盘（原子写入，避免半截文件）。
func saveWindowState(state WindowState) {
	path := windowStatePath()
	if path == "" {
		mainLogger.Error("用户配置目录不可用，无法保存窗口状态")
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		mainLogger.Errorf("创建窗口状态目录失败: %v", err)
		return
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		mainLogger.Errorf("序列化窗口状态失败: %v", err)
		return
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		mainLogger.Errorf("写入窗口状态失败: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		mainLogger.Errorf("替换窗口状态文件失败: %v", err)
		_ = os.Remove(tmp)
	}
}

// ApplyTo 把记忆的窗口状态写进主窗口的创建参数。位置在创建时就交给窗口，
// 不再像以前那样先居中显示、启动后再挪过去；是否还落在屏幕内由 keepWindowOnScreen 兜底。
func (s WindowState) ApplyTo(options *application.WebviewWindowOptions) {
	options.Width = s.Width
	options.Height = s.Height
	if s.Maximised {
		options.StartState = application.WindowStateMaximised
		return
	}
	if s.X == windowPositionUnset || s.Y == windowPositionUnset {
		return
	}
	options.InitialPosition = application.WindowXY
	options.X = s.X
	options.Y = s.Y
}

// keepWindowOnScreen 在窗口就绪后检查恢复出来的位置：记录它的显示器可能已经断开，
// 窗口不在任何屏幕内时改为居中。
func (a *App) keepWindowOnScreen() {
	if a.app == nil || a.window == nil || a.window.IsMaximised() {
		return
	}
	bounds := a.window.Bounds()
	if positionOnScreens(a.app.Screen.GetAll(), bounds) {
		return
	}
	mainLogger.Warnf("记录的窗口位置 (%d,%d) 不在任何屏幕内，忽略并居中", bounds.X, bounds.Y)
	a.window.Center()
}

// positionOnScreens 判断窗口标题栏是否还能在某块屏幕上点到。窗口坐标与屏幕边界
// 都是 Wails 统一换算过的 DIP 绝对坐标，多显示器（含负坐标的副屏）都能正确判断。
// 拿不到屏幕信息时不做干预。
func positionOnScreens(screens []*application.Screen, window application.Rect) bool {
	if len(screens) == 0 {
		return true
	}
	// 允许窗口部分越界，但标题栏至少要有一段落在屏幕里，用户才能把它拖回来。
	const margin = 120
	for _, screen := range screens {
		b := screen.Bounds
		if window.X+window.Width-margin >= b.X && window.X+margin <= b.X+b.Width &&
			window.Y >= b.Y-margin && window.Y <= b.Y+b.Height-margin {
			return true
		}
	}
	return false
}

// captureWindowState 从当前窗口读取几何信息并持久化。
func (a *App) captureWindowState() {
	// 最小化时读到的是系统放到屏幕外的占位坐标，保留上一次记录。
	if a.window == nil || a.window.IsMinimised() {
		return
	}

	if a.window.IsMaximised() {
		prev := LoadWindowState()
		prev.Maximised = true
		saveWindowState(prev)
		return
	}

	bounds := a.window.Bounds()
	state := WindowState{
		Width:     bounds.Width,
		Height:    bounds.Height,
		X:         bounds.X,
		Y:         bounds.Y,
		Maximised: false,
	}
	if state.Width < minWindowWidth || state.Height < minWindowHeight {
		prev := LoadWindowState()
		state.Width = prev.Width
		state.Height = prev.Height
	}
	saveWindowState(state)
}
