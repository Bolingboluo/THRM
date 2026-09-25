package guiapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/types"
)

func TestUpdateGuiResponseTimeRequestPolicy(t *testing.T) {
	timeout, retryable := ipcRequestPolicy(ipc.ReqUpdateGuiResponseTime)
	if timeout != 12*time.Second {
		t.Fatalf("ipcRequestPolicy timeout = %s, want 12s", timeout)
	}
	if !retryable {
		t.Fatal("UpdateGuiResponseTime should be retryable after reconnect")
	}
}

func TestShouldReconnectAfterIPCError(t *testing.T) {
	timeoutErr := ipc.ErrRequestTimeout
	if shouldReconnectAfterIPCError(timeoutErr, ipcTimeoutReconnectThreshold-1) {
		t.Fatal("transient IPC timeout should keep the current connection")
	}
	if !shouldReconnectAfterIPCError(timeoutErr, ipcTimeoutReconnectThreshold) {
		t.Fatal("repeated IPC timeouts should trigger reconnect")
	}
	if !shouldReconnectAfterIPCError(errors.New("broken pipe"), 0) {
		t.Fatal("non-timeout IPC errors should reconnect immediately")
	}
}

func TestRecordIPCTimeoutUsesWindow(t *testing.T) {
	app := &App{}
	start := time.Unix(1000, 0)
	if got := app.recordIPCTimeout(start); got != 1 {
		t.Fatalf("first timeout count = %d, want 1", got)
	}
	if got := app.recordIPCTimeout(start.Add(ipcTimeoutReconnectWindow / 2)); got != 2 {
		t.Fatalf("timeout inside window count = %d, want 2", got)
	}
	if got := app.recordIPCTimeout(start.Add(2 * ipcTimeoutReconnectWindow)); got != 1 {
		t.Fatalf("timeout outside window count = %d, want reset to 1", got)
	}
	app.resetIPCTimeouts()
	if got := app.recordIPCTimeout(start.Add(3 * ipcTimeoutReconnectWindow)); got != 1 {
		t.Fatalf("timeout after success reset = %d, want 1", got)
	}
}

func TestMergeTemperatureMetadata(t *testing.T) {
	previous := types.TemperatureData{
		CpuModel:          "Ryzen",
		GpuModel:          "RTX",
		SelectedGpuDevice: "gpu0",
		CpuSensors:        []types.TemperatureSensor{{Key: "cpu-package", Name: "CPU Package", Value: 70}},
		GpuSensors:        []types.TemperatureSensor{{Key: "gpu-core", Name: "GPU Core", Value: 66}},
		CpuPowerSensors:   []types.PowerSensor{{Key: "cpu/power/package", Name: "CPU Package Power", Value: 45.5}},
		GpuPowerSensors:   []types.PowerSensor{{Key: "gpu/power/board", Name: "GPU Board Power", Value: 80.2}},
		GpuDevices:        []types.TemperatureGPUDevice{{Key: "gpu0", Name: "GPU 0", Vendor: "nvidia"}},
	}

	incomingCompact := types.TemperatureData{CPUTemp: 72, GPUTemp: 67}
	mergedCompact := mergeTemperatureMetadata(previous, incomingCompact)
	if len(mergedCompact.CpuSensors) != 1 || mergedCompact.CpuSensors[0].Key != "cpu-package" {
		t.Fatal("mergeTemperatureMetadata() should preserve previous cpuSensors when incoming payload omits them")
	}
	if len(mergedCompact.GpuDevices) != 1 || mergedCompact.GpuDevices[0].Key != "gpu0" {
		t.Fatal("mergeTemperatureMetadata() should preserve previous gpuDevices when incoming payload omits them")
	}
	if len(mergedCompact.CpuPowerSensors) != 1 || len(mergedCompact.GpuPowerSensors) != 1 {
		t.Fatal("mergeTemperatureMetadata() should preserve previous power sensors when incoming payload omits them")
	}
	if mergedCompact.CpuModel != "Ryzen" || mergedCompact.GpuModel != "RTX" {
		t.Fatal("mergeTemperatureMetadata() should preserve previous model metadata when incoming payload omits it")
	}

	incomingClear := types.TemperatureData{CpuSensors: []types.TemperatureSensor{}, GpuDevices: []types.TemperatureGPUDevice{}}
	mergedClear := mergeTemperatureMetadata(previous, incomingClear)
	if mergedClear.CpuSensors == nil || len(mergedClear.CpuSensors) != 0 {
		t.Fatal("mergeTemperatureMetadata() should keep explicit empty cpuSensors to allow clearing metadata")
	}
	if mergedClear.GpuDevices == nil || len(mergedClear.GpuDevices) != 0 {
		t.Fatal("mergeTemperatureMetadata() should keep explicit empty gpuDevices to allow clearing metadata")
	}
}

// 长请求的超时必须高于核心侧最坏耗时，且超时不计入连续超时重连阈值。
func TestLongRequestTimeoutsExceedCoreBounds(t *testing.T) {
	cases := map[ipc.RequestType]time.Duration{
		ipc.ReqReinstallPawnIO:        240 * time.Second, // 卸载 90s + 安装 90s + 重启桥接
		ipc.ReqRestartPawnIO:          90 * time.Second,
		ipc.ReqTestBridgeProgram:      30 * time.Second,
		ipc.ReqGetBridgeProgramStatus: 15 * time.Second,
	}
	for reqType, minimum := range cases {
		timeout, _ := ipcRequestPolicy(reqType)
		if timeout < minimum {
			t.Errorf("%s timeout = %s, want >= %s", reqType, timeout, minimum)
		}
		if ipcTimeoutCountsTowardReconnect(timeout) {
			t.Errorf("%s 超时不应计入连续超时重连阈值", reqType)
		}
	}
	for _, reqType := range []ipc.RequestType{ipc.ReqPing, ipc.ReqGetConfig, ipc.ReqUpdateConfig} {
		timeout, _ := ipcRequestPolicy(reqType)
		if !ipcTimeoutCountsTowardReconnect(timeout) {
			t.Errorf("常规请求 %s 的超时应计入重连阈值", reqType)
		}
	}
}

// core-service-ok 只在跳变时发送：首次成功一次、每次从异常恢复一次。
func TestCoreServiceOKOnlyOnTransition(t *testing.T) {
	app := &App{}
	if !app.markCoreServiceOKLocked() {
		t.Fatal("启动后首次成功必须通知前端")
	}
	if app.markCoreServiceOKLocked() {
		t.Fatal("持续正常时不应重复通知")
	}
	app.markCoreServiceDegraded()
	if !app.markCoreServiceOKLocked() {
		t.Fatal("从异常恢复时必须通知前端，前端据此重新同步")
	}
}

// 退出流程中 sendRequest 必须直接拒绝，不能去 EnsureCoreServiceRunning 把核心拉起来。
func TestSendRequestRefusedWhileShuttingDown(t *testing.T) {
	app := &App{ipcClient: ipc.NewClient(nil)}
	app.shuttingDown.Store(true)
	if _, err := app.sendRequest(ipc.ReqPing, nil); !errors.Is(err, errGUIShuttingDown) {
		t.Fatalf("sendRequest error = %v, want errGUIShuttingDown", err)
	}
	if _, err := app.GetConfig(); err == nil {
		t.Fatal("退出流程中 GetConfig 应返回错误而不是默认配置")
	}
}

// startTestCore 启动一个隔离管道上的假核心，并返回已连上它的 App。
func startTestCore(t *testing.T, handler ipc.RequestHandler) *App {
	t.Helper()
	t.Setenv(ipc.EnvPipeName, fmt.Sprintf("THRM-guiapp-test-%d-%d", os.Getpid(), time.Now().UnixNano()))
	server := ipc.NewServer(handler, nil)
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	t.Cleanup(server.Stop)

	app := &App{ipcClient: ipc.NewClient(nil)}
	if err := app.ipcClient.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	t.Cleanup(app.ipcClient.Close)
	return app
}

// 取配置失败时必须报错：返回默认配置会被前端整份写回，把用户配置重置掉。
func TestGetConfigReportsFailuresInsteadOfDefaults(t *testing.T) {
	app := startTestCore(t, func(req ipc.Request) ipc.Response {
		switch req.Type {
		case ipc.ReqGetConfig:
			return ipc.Response{Success: false, Error: "config unavailable"}
		case ipc.ReqGetFanCurve:
			return ipc.Response{Success: true, Data: json.RawMessage(`{"not":"a curve"}`)}
		case ipc.ReqGetFanCurveProfiles:
			return ipc.Response{Success: false, Error: "profiles unavailable"}
		}
		return ipc.Response{Success: true}
	})

	if _, err := app.GetConfig(); err == nil {
		t.Fatal("GetConfig 在核心返回失败时应报错")
	}
	if _, err := app.GetFanCurve(); err == nil {
		t.Fatal("GetFanCurve 在响应无法解析时应报错")
	}
	if _, err := app.GetFanCurveProfiles(); err == nil {
		t.Fatal("GetFanCurveProfiles 在核心返回失败时应报错")
	}
}

// GetTemperature 的完整快照必须播种温度缓存，之后被压缩的事件才能合并出完整清单。
func TestGetTemperatureSeedsCacheForCompactEvents(t *testing.T) {
	full := types.TemperatureData{
		CPUTemp:    60,
		CpuModel:   "Ryzen",
		CpuSensors: []types.TemperatureSensor{{Key: "cpu-package", Name: "CPU Package", Value: 60}},
		GpuDevices: []types.TemperatureGPUDevice{{Key: "gpu0", Name: "GPU 0"}},
	}
	app := startTestCore(t, func(req ipc.Request) ipc.Response {
		data, _ := json.Marshal(full)
		return ipc.Response{Success: true, Data: data}
	})

	if got := app.GetTemperature(); len(got.CpuSensors) != 1 {
		t.Fatalf("GetTemperature 返回的传感器清单不完整: %+v", got)
	}

	// 模拟 GUI 打开后收到的第一帧压缩事件：清单未变化因而被核心省略。
	merged := app.mergeCurrentTemp(types.TemperatureData{CPUTemp: 61})
	if len(merged.CpuSensors) != 1 || len(merged.GpuDevices) != 1 || merged.CpuModel != "Ryzen" {
		t.Fatalf("压缩事件没有合并出完整清单: %+v", merged)
	}
	if merged.CPUTemp != 61 {
		t.Fatalf("合并后应采用新读数: %d", merged.CPUTemp)
	}
}
