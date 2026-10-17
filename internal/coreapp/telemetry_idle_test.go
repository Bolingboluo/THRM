package coreapp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/TIANLI0/THRM/internal/ipc"
	"github.com/TIANLI0/THRM/internal/types"
)

func TestHIDIdlePollingAllowed(t *testing.T) {
	cases := []struct {
		name   string
		active bool
		rtss   types.RTSSConfig
		want   bool
	}{
		{"foreground GUI needs live RPM", true, types.RTSSConfig{}, false},
		{"background without RTSS", false, types.RTSSConfig{Enabled: false, UpdateIntervalMS: 250}, true},
		{"RTSS at 250ms needs fast polling", false, types.RTSSConfig{Enabled: true, UpdateIntervalMS: 250}, false},
		{"RTSS at 500ms would alias to 1s", false, types.RTSSConfig{Enabled: true, UpdateIntervalMS: 500}, false},
		{"RTSS at 1s tolerates idle polling", false, types.RTSSConfig{Enabled: true, UpdateIntervalMS: 1000}, true},
		{"RTSS at 2s tolerates idle polling", false, types.RTSSConfig{Enabled: true, UpdateIntervalMS: 2000}, true},
		{"invalid RTSS interval falls back to the 1s default", false, types.RTSSConfig{Enabled: true, UpdateIntervalMS: 0}, true},
	}
	for _, tc := range cases {
		if got := hidIdlePollingAllowed(tc.active, tc.rtss); got != tc.want {
			t.Errorf("%s: hidIdlePollingAllowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func payloadFixture(offset int) types.TemperatureData {
	cpu := make([]types.TemperatureSensor, 0, 17)
	for i := range 17 {
		cpu = append(cpu, types.TemperatureSensor{Key: fmt.Sprintf("cpu/intelcpu/0/temperature/%d", i), Name: fmt.Sprintf("CPU Core #%d", i+1), Value: 60 + i%7 + offset})
	}
	gpu := []types.TemperatureSensor{
		{Key: "gpu/gpu-nvidia/0/temperature/0", Name: "GPU Core", Value: 55 + offset},
		{Key: "gpu/gpu-nvidia/0/temperature/2", Name: "GPU Hot Spot", Value: 64 + offset},
	}
	gpuPower := []types.PowerSensor{{Key: "gpu/gpu-nvidia/0/power/0", Name: "GPU Package", Value: 80.5 + float64(offset)}}
	return types.TemperatureData{
		CPUTemp:         66 + offset,
		GPUTemp:         55 + offset,
		CpuModel:        "Intel Core i9-13980HX",
		GpuModel:        "NVIDIA GeForce RTX 4080 Laptop GPU",
		CpuSensors:      cpu,
		GpuSensors:      gpu,
		CpuPowerSensors: []types.PowerSensor{{Key: "cpu/intelcpu/0/power/0", Name: "CPU Package", Value: 55.25 + float64(offset)}},
		GpuPowerSensors: gpuPower,
		GpuDevices: []types.TemperatureGPUDevice{
			{Key: "gpu-nvidia/0", Name: "NVIDIA GeForce RTX 4080 Laptop GPU", Vendor: "nvidia", Sensors: gpu, PowerSensors: gpuPower},
		},
		UpdateTime: int64(1_700_000_000_000 + offset),
		BridgeOk:   true,
	}
}

// TestTemperatureEventPayloadSelection 覆盖三种编码的选择，并量化新格式省下的载荷。
func TestTemperatureEventPayloadSelection(t *testing.T) {
	prev := payloadFixture(0)
	cur := payloadFixture(1) // 典型的一帧：清单不变，读数全变

	if got := temperatureEventPayload(cur, prev, true, true); !reflect.DeepEqual(got, cur) {
		t.Fatal("full frame must carry the complete data")
	}
	legacy := temperatureEventPayload(cur, prev, false, false)
	if !reflect.DeepEqual(legacy, compactTemperatureEventPayload(cur, prev)) {
		t.Fatal("clients without sensor-values support must get the legacy encoding")
	}
	compact, ok := temperatureEventPayload(cur, prev, false, true).(ipc.TemperatureEvent)
	if !ok || compact.SensorValues == nil {
		t.Fatal("clients with sensor-values support must get the compact encoding")
	}

	fullBytes, _ := json.Marshal(cur)
	legacyBytes, _ := json.Marshal(legacy)
	compactBytes, _ := json.Marshal(compact)
	t.Logf("temperature-update payload: full=%dB legacy-compact=%dB sensor-values=%dB", len(fullBytes), len(legacyBytes), len(compactBytes))
	// 旧格式在读数变化时整份重发清单（legacy 与 full 一样大），新格式只发读数。
	if len(compactBytes)*2 > len(legacyBytes) {
		t.Fatalf("sensor-values encoding should be well under half the legacy size: %d vs %d", len(compactBytes), len(legacyBytes))
	}
}
