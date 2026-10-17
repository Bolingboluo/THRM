package guiapp

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/types"
)

func guiTemperatureFixture(offset int) types.TemperatureData {
	gpu := []types.TemperatureSensor{
		{Key: "gpu/0/temperature/0", Name: "GPU Core", Value: 50 + offset},
		{Key: "gpu/0/temperature/1", Name: "GPU Hot Spot", Value: 61 + offset},
	}
	gpuPower := []types.PowerSensor{{Key: "gpu/0/power/0", Name: "GPU Package", Value: 70.5 + float64(offset)}}
	return types.TemperatureData{
		CPUTemp:  65 + offset,
		GPUTemp:  50 + offset,
		CpuModel: "Ryzen",
		CpuSensors: []types.TemperatureSensor{
			{Key: "cpu/0/temperature/0", Name: "Tctl", Value: 65 + offset},
			{Key: "cpu/0/temperature/1", Name: "CCD1", Value: 60 + offset},
		},
		GpuSensors:      gpu,
		CpuPowerSensors: []types.PowerSensor{{Key: "cpu/0/power/0", Name: "Package", Value: 40.25 + float64(offset)}},
		GpuPowerSensors: gpuPower,
		GpuDevices:      []types.TemperatureGPUDevice{{Key: "gpu/0", Name: "GPU 0", Vendor: "nvidia", Sensors: gpu, PowerSensors: gpuPower}},
		UpdateTime:      int64(1000 + offset),
		BridgeOk:        true,
	}
}

func roundTripTemperatureEvent(t *testing.T, event ipc.TemperatureEvent) ipc.TemperatureEvent {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ipc.TemperatureEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

// TestMergeTemperatureEventMatchesLegacyMerge 保证新格式下 GUI 推给前端的对象与旧格式
// （核心发完整清单）完全一致，前端无需任何改动。
func TestMergeTemperatureEventMatchesLegacyMerge(t *testing.T) {
	prev := guiTemperatureFixture(0)
	cur := guiTemperatureFixture(2)

	legacyApp := &App{currentTemp: prev}
	want := legacyApp.mergeCurrentTemp(cur)

	app := &App{currentTemp: prev}
	got, ok := app.mergeTemperatureEvent(roundTripTemperatureEvent(t, ipc.CompactTemperatureEvent(cur, prev)))
	if !ok {
		t.Fatal("catalog should match the cached frame")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged frame differs from legacy merge\n got: %+v\nwant: %+v", got, want)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("webview payload differs\n got: %s\nwant: %s", gotJSON, wantJSON)
	}

	// 旧版核心的帧（没有 sensorValues）走的仍是原来的合并逻辑。
	legacyFrame := cur
	legacyFrame.GpuDevices = nil
	got, ok = app.mergeTemperatureEvent(roundTripTemperatureEvent(t, ipc.TemperatureEvent{TemperatureData: legacyFrame}))
	if !ok || !reflect.DeepEqual(got.GpuDevices, cur.GpuDevices) || !reflect.DeepEqual(got.CpuSensors, cur.CpuSensors) {
		t.Fatalf("legacy frame merge changed: %+v", got)
	}
}

// 带新清单的帧被丢弃时，读数不能贴到旧清单上；GUI 沿用缓存并报告需要重新同步。
func TestMergeTemperatureEventKeepsCacheOnStaleCatalog(t *testing.T) {
	stale := guiTemperatureFixture(0)
	prev := guiTemperatureFixture(1)
	prev.CpuSensors[1].Name = "Renamed"
	cur := guiTemperatureFixture(2)
	cur.CpuSensors[1].Name = "Renamed"

	app := &App{currentTemp: stale}
	got, ok := app.mergeTemperatureEvent(roundTripTemperatureEvent(t, ipc.CompactTemperatureEvent(cur, prev)))
	if ok {
		t.Fatal("stale catalog must trigger a resync")
	}
	if !reflect.DeepEqual(got.CpuSensors, stale.CpuSensors) {
		t.Fatalf("stale CPU list should be kept untouched until the snapshot arrives: %+v", got.CpuSensors)
	}
	if got.CPUTemp != cur.CPUTemp || !reflect.DeepEqual(got.GpuSensors, cur.GpuSensors) {
		t.Fatalf("unaffected fields must still update: %+v", got)
	}
}

// TestCoreEventSubscriptionFollowsVisibility 覆盖窗口隐藏/恢复与重连后的订阅同步。
func TestCoreEventSubscriptionFollowsVisibility(t *testing.T) {
	t.Setenv(ipc.EnvPipeName, fmt.Sprintf("THRM-guiapp-sub-%d-%d", os.Getpid(), time.Now().UnixNano()))
	server := ipc.NewServer(func(req ipc.Request) ipc.Response { return ipc.Response{Success: true} }, nil)
	if err := server.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	t.Cleanup(server.Stop)

	app := &App{ipcClient: ipc.NewClient(nil)}
	if err := app.ipcClient.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	t.Cleanup(app.ipcClient.Close)

	waitUntil := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitUntil("client registration", server.HasClients)

	// 连上后同步一次：可见时订阅并声明能力。
	app.syncCoreEventSubscription()
	if !server.HasActiveClients() || !server.ActiveClientsSupport(ipc.FeatureTemperatureSensorValues) {
		t.Fatal("visible GUI should be subscribed with its features declared")
	}

	app.SetCoreEventSubscription(false)
	if !app.coreEventsPaused() || server.HasActiveClients() {
		t.Fatal("hidden GUI should unsubscribe")
	}
	// 重复的通知不会重发请求，也不改变状态。
	app.SetCoreEventSubscription(false)
	if server.HasActiveClients() {
		t.Fatal("hidden GUI should stay unsubscribed")
	}

	// 隐藏期间重连：核心把新连接当作已订阅，GUI 必须再退订一次。
	app.ipcClient.Close()
	waitUntil("old connection removal", func() bool { return !server.HasClients() })
	if err := app.ipcClient.Connect(); err != nil {
		t.Fatalf("reconnect error: %v", err)
	}
	waitUntil("reconnected client registration", server.HasClients)
	if !server.HasActiveClients() {
		t.Fatal("core treats new connections as subscribed")
	}
	app.syncCoreEventSubscription()
	if server.HasActiveClients() {
		t.Fatal("reconnect while hidden must re-apply the unsubscription")
	}

	app.SetCoreEventSubscription(true)
	if app.coreEventsPaused() || !server.HasActiveClients() {
		t.Fatal("restored GUI should resubscribe")
	}
}
