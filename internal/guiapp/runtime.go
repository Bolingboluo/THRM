package guiapp

import (
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/wailsapp/wails/v3/pkg/application"
)

var ensureCoreServiceRunningMu sync.Mutex

// OnSecondInstanceLaunch 在用户再次启动 GUI 时把已有窗口带到前台。
//
//wails:ignore
func (a *App) OnSecondInstanceLaunch(data application.SecondInstanceData) {
	mainLogger.Infof("检测到第二个实例启动，参数: %s，工作目录: %s", strings.Join(data.Args, ","), data.WorkingDir)

	if a.window == nil {
		return
	}
	a.window.UnMinimise()
	a.window.Show().Focus()
	// 短暂置顶，确保窗口能盖过当前前台程序（Windows 会拦截后台进程抢焦点）。
	a.window.SetAlwaysOnTop(true)
	time.AfterFunc(time.Second, func() {
		a.window.SetAlwaysOnTop(false)
	})
}

// EnsureCoreServiceRunning 确保核心服务正在运行。
func EnsureCoreServiceRunning() bool {
	ensureCoreServiceRunningMu.Lock()
	defer ensureCoreServiceRunningMu.Unlock()

	if ipc.CheckCoreServiceRunning() {
		mainLogger.Info("核心服务已经在运行")
		return true
	}

	mainLogger.Info("核心服务未运行，正在启动...")
	exePath, err := os.Executable()
	if err != nil {
		mainLogger.Errorf("获取可执行文件路径失败: %v", err)
		return false
	}

	exeDir := filepath.Dir(exePath)
	corePath := appmeta.FirstExistingPath(appmeta.CoreExecutableCandidates(exeDir))
	if corePath == "" {
		mainLogger.Errorf("核心服务程序不存在: %v", appmeta.CoreExecutableCandidates(exeDir))
		return false
	}

	cmd := exec.Command(corePath)
	configureCoreCommand(cmd)

	if err := cmd.Start(); err != nil {
		mainLogger.Errorf("启动核心服务失败: %v", err)
		return false
	}

	mainLogger.Infof("核心服务已启动，PID: %d", cmd.Process.Pid)
	if cmd.Process != nil {
		cmd.Process.Release()
	}

	for i := range 100 {
		time.Sleep(100 * time.Millisecond)
		if ipc.CheckCoreServiceRunning() {
			mainLogger.Infof("核心服务已就绪（等待 %d ms）", (i+1)*100)
			return true
		}
	}

	mainLogger.Warn("等待核心服务就绪超时（10秒）")
	return false
}

// DefaultFrameless reports whether the desktop window should use the custom Windows frame.
func DefaultFrameless() bool {
	return goruntime.GOOS == "windows"
}
