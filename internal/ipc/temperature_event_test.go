package ipc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/TIANLI0/THRM/internal/types"
)

// sampleTemperature 构造一帧接近真实桥接输出的温度数据，offset 只改变读数。
func sampleTemperature(offset int) types.TemperatureData {
	cpu := make([]types.TemperatureSensor, 0, 18)
	cpu = append(cpu, types.TemperatureSensor{Key: "cpu/amdcpu/0/temperature/2", Name: "Core (Tctl/Tdie)", Value: 70 + offset})
	for i := range 16 {
		cpu = append(cpu, types.TemperatureSensor{Key: fmt.Sprintf("cpu/amdcpu/0/temperature/%d", i+3), Name: fmt.Sprintf("CCD1 Core #%d", i+1), Value: 60 + i%5 + offset})
	}
	gpuSensors := []types.TemperatureSensor{
		{Key: "gpu/gpu-nvidia/0/temperature/0", Name: "GPU Core", Value: 55 + offset},
		{Key: "gpu/gpu-nvidia/0/temperature/2", Name: "GPU Hot Spot", Value: 66 + offset},
		{Key: "gpu/gpu-nvidia/0/temperature/3", Name: "GPU Memory Junction", Value: 62 + offset},
	}
	gpuPower := []types.PowerSensor{{Key: "gpu/gpu-nvidia/0/power/0", Name: "GPU Package", Value: 87.25 + float64(offset)}}
	return types.TemperatureData{
		CPUTemp:           70 + offset,
		GPUTemp:           55 + offset,
		CPUPower:          45.5 + float64(offset),
		GPUPower:          87.25 + float64(offset),
		MaxTemp:           70 + offset,
		ControlTemp:       70 + offset,
		ControlSource:     "cpu",
		SelectedGpuDevice: "gpu-nvidia/0",
		CpuModel:          "AMD Ryzen 9 7945HX",
		GpuModel:          "NVIDIA GeForce RTX 4070 Laptop GPU",
		CpuSensors:        cpu,
		GpuSensors:        gpuSensors,
		CpuPowerSensors: []types.PowerSensor{
			{Key: "cpu/amdcpu/0/power/0", Name: "Package", Value: 45.5 + float64(offset)},
			{Key: "cpu/amdcpu/0/power/1", Name: "Core #1 (SMU)", Value: 3.125 + float64(offset)},
		},
		GpuPowerSensors: gpuPower,
		GpuDevices: []types.TemperatureGPUDevice{
			{Key: "gpu-nvidia/0", Name: "NVIDIA GeForce RTX 4070 Laptop GPU", Vendor: "nvidia", Sensors: gpuSensors, PowerSensors: gpuPower},
			{Key: "gpu-amd/0", Name: "AMD Radeon(TM) 610M", Vendor: "amd", Sensors: []types.TemperatureSensor{{Key: "gpu/gpu-amd/0/temperature/0", Name: "GPU Core", Value: 48 + offset}}, PowerSensors: nil},
		},
		UpdateTime: int64(1_700_000_000_000 + offset),
		BridgeOk:   true,
	}
}

// overlay 模拟 GUI 的缓存合并：事件里为 nil 的清单沿用缓存。
func overlay(cached, incoming types.TemperatureData) types.TemperatureData {
	if incoming.CpuSensors == nil {
		incoming.CpuSensors = cached.CpuSensors
	}
	if incoming.GpuSensors == nil {
		incoming.GpuSensors = cached.GpuSensors
	}
	if incoming.CpuPowerSensors == nil {
		incoming.CpuPowerSensors = cached.CpuPowerSensors
	}
	if incoming.GpuPowerSensors == nil {
		incoming.GpuPowerSensors = cached.GpuPowerSensors
	}
	if incoming.GpuDevices == nil {
		incoming.GpuDevices = cached.GpuDevices
	}
	return incoming
}

// wire 让事件经过一次真实的 JSON 编解码。
func wire(t *testing.T, event TemperatureEvent) (TemperatureEvent, int) {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TemperatureEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded, len(data)
}

func TestTemperatureEventRoundTripIsExact(t *testing.T) {
	prev := sampleTemperature(0)
	cases := map[string]func() types.TemperatureData{
		"values only": func() types.TemperatureData { return sampleTemperature(3) },
		"nothing changed": func() types.TemperatureData {
			return sampleTemperature(0)
		},
		"cpu catalog changed": func() types.TemperatureData {
			cur := sampleTemperature(1)
			cur.CpuSensors = append(cur.CpuSensors, types.TemperatureSensor{Key: "cpu/new", Name: "New", Value: 1})
			return cur
		},
		"gpu device went to sleep": func() types.TemperatureData {
			cur := sampleTemperature(2)
			cur.GpuDevices = cur.GpuDevices[:1]
			return cur
		},
		"nested gpu power list became empty": func() types.TemperatureData {
			cur := sampleTemperature(2)
			cur.GpuDevices[1].PowerSensors = []types.PowerSensor{}
			return cur
		},
		"lists cleared": func() types.TemperatureData {
			cur := sampleTemperature(2)
			cur.GpuSensors = []types.TemperatureSensor{}
			cur.GpuPowerSensors = []types.PowerSensor{}
			return cur
		},
		"extended sensors": func() types.TemperatureData {
			cur := sampleTemperature(2)
			cur.OtherSensors = []types.TemperatureSensor{{Key: "storage/0", Name: "SSD", Value: 41}}
			return cur
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			cur := build()
			event, _ := wire(t, CompactTemperatureEvent(cur, prev))
			resolved, ok := ResolveTemperatureEvent(event, prev)
			if !ok {
				t.Fatal("resolve reported a catalog mismatch against the true previous frame")
			}
			got := overlay(prev, resolved)
			// 旧格式下 GUI 得到的就是 overlay(prev, cur)：变化了的清单整份覆盖。
			want := overlay(prev, cur)
			if !reflect.DeepEqual(got, want) {
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				t.Fatalf("reconstructed frame differs\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
		})
	}
}

// TestTemperatureEventDetectsStaleCatalog 覆盖带新清单的那一帧被丢弃的情形：
// 读数不能按位置贴到旧清单上，必须报告对不上，让 GUI 拉完整快照。
func TestTemperatureEventDetectsStaleCatalog(t *testing.T) {
	stale := sampleTemperature(0)
	prev := sampleTemperature(1)
	// 数量不变、名字变了：只比长度会误判为可套用。
	prev.CpuSensors[1].Name = "Renamed sensor"
	cur := sampleTemperature(2)
	cur.CpuSensors[1].Name = "Renamed sensor"

	event, _ := wire(t, CompactTemperatureEvent(cur, prev))
	if event.SensorValues == nil || event.SensorValues.Cpu == nil {
		t.Fatal("expected CPU values to be sent compactly")
	}
	resolved, ok := ResolveTemperatureEvent(event, stale)
	if ok {
		t.Fatal("stale catalog must be reported")
	}
	if resolved.CpuSensors != nil {
		t.Fatal("values must not be applied onto a stale catalog")
	}
	// 其余清单没变，照常还原。
	if resolved.GpuSensors == nil || resolved.GpuSensors[0].Value != cur.GpuSensors[0].Value {
		t.Fatal("unaffected groups should still be reconstructed")
	}
}

func TestTemperatureEventWithoutValuesMatchesLegacyEncoding(t *testing.T) {
	cur := sampleTemperature(0)
	legacy, err := json.Marshal(cur)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := json.Marshal(TemperatureEvent{TemperatureData: cur})
	if err != nil {
		t.Fatal(err)
	}
	if string(legacy) != string(wrapped) {
		t.Fatalf("TemperatureEvent without SensorValues must encode exactly like TemperatureData\n got: %s\nwant: %s", wrapped, legacy)
	}
}
