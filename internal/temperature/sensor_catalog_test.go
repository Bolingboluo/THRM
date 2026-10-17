package temperature

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/TIANLI0/THRM/internal/types"
)

func fullBridgeSampleForTest() types.BridgeTemperatureData {
	return types.BridgeTemperatureData{
		CpuTemp:  60,
		GpuTemp:  50,
		MaxTemp:  60,
		CpuModel: "CPU",
		GpuModel: "GPU",
		CpuSensors: []types.TemperatureSensor{
			{Key: "cpu/Core #1", Name: "Core #1", Value: 58},
			{Key: "cpu/Core #2", Name: "Core #2", Value: 62},
			{Key: "cpu/Package", Name: "Package", Value: 60},
		},
		GpuSensors:      []types.TemperatureSensor{{Key: "gpu/Core", Name: "Core", Value: 50}},
		CpuPowerSensors: []types.PowerSensor{{Key: "cpu/power/Package", Name: "Package", Value: 25}},
		GpuPowerSensors: []types.PowerSensor{{Key: "gpu/power/Power", Name: "Power", Value: 40}},
		GpuDevices: []types.TemperatureGPUDevice{{
			Key: "nvidia:0:GPU", Name: "GPU", Vendor: "nvidia",
			Sensors: []types.TemperatureSensor{{Key: "gpu/Core", Name: "Core", Value: 50}},
		}},
		Success: true,
	}
}

// compactOf 模拟新版桥接对同一次采样给出的精简结果。
func compactOf(full types.BridgeTemperatureData, selectedCPU []string) types.BridgeTemperatureData {
	compact := full
	compact.Compact = true
	compact.CpuSensors = nil
	for _, sensor := range full.CpuSensors {
		for _, key := range selectedCPU {
			if strings.EqualFold(sensor.Key, key) {
				compact.CpuSensors = append(compact.CpuSensors, sensor)
				break
			}
		}
	}
	compact.GpuSensors = nil
	compact.CpuPowerSensors = nil
	compact.GpuPowerSensors = nil
	compact.GpuDevices = nil
	return compact
}

// readThroughCatalog 走一遍 Read 中桥接分支之后的处理（复制、平均、清单缓存）。
func readThroughCatalog(r *Reader, bridgeTemp types.BridgeTemperatureData, selection types.TemperatureSelection) types.TemperatureData {
	selection = types.NormalizeTemperatureSelection(selection)
	var temp types.TemperatureData
	copyBridgeTemperatureMetadata(&temp, bridgeTemp, selection)
	if selection.DisableGpu {
		stripGpuReadings(&temp, selection.TempSource)
	}
	applyMultiSensorCpuAverage(&temp, selection.CpuSensors)
	r.reconcileSensorCatalog(&temp, bridgeReadOutcome{
		compact:     bridgeTemp.Compact,
		fullSuccess: bridgeTemp.Success && !bridgeTemp.Compact,
	}, selection)
	return temp
}

// 精简结果的标量（含多传感器平均）必须与完整结果一致，清单由最近一次完整结果回填。
func TestCompactReadMatchesFullRead(t *testing.T) {
	selection := types.TemperatureSelection{
		TempSource: types.TempSourceCPU,
		CpuSensors: []string{"CPU/core #1", "cpu/Core #2"},
	}
	reader := NewReader(nil, testLogger{})
	full := fullBridgeSampleForTest()
	fullTemp := readThroughCatalog(reader, full, selection)
	if fullTemp.CPUTemp != 60 { // (58+62)/2
		t.Fatalf("multi-sensor average = %d, want 60", fullTemp.CPUTemp)
	}

	compactSelection := selection
	compactSelection.Compact = true
	compactTemp := readThroughCatalog(reader, compactOf(full, selection.CpuSensors), compactSelection)

	if compactTemp.CPUTemp != fullTemp.CPUTemp || compactTemp.GPUTemp != fullTemp.GPUTemp ||
		compactTemp.MaxTemp != fullTemp.MaxTemp || compactTemp.ControlTemp != fullTemp.ControlTemp {
		t.Fatalf("compact scalars differ: compact=%+v full=%+v", compactTemp, fullTemp)
	}
	if !slices.Equal(compactTemp.CpuSensors, full.CpuSensors) || !slices.Equal(compactTemp.GpuSensors, full.GpuSensors) ||
		!slices.Equal(compactTemp.CpuPowerSensors, full.CpuPowerSensors) || !slices.Equal(compactTemp.GpuPowerSensors, full.GpuPowerSensors) ||
		!gpuDevicesEqualForTest(compactTemp.GpuDevices, full.GpuDevices) {
		t.Fatal("compact result must carry the cached full sensor catalog")
	}
}

func gpuDevicesEqualForTest(a, b []types.TemperatureGPUDevice) bool {
	return slices.EqualFunc(a, b, func(x, y types.TemperatureGPUDevice) bool {
		return x.Key == y.Key && x.Name == y.Name && x.Vendor == y.Vendor &&
			slices.Equal(x.Sensors, y.Sensors) && slices.Equal(x.PowerSensors, y.PowerSensors)
	})
}

// 没有匹配的完整清单时 Read 不能请求精简结果：清单依赖 GPU 选择与 GPU 监测开关。
func TestCompactRequiresMatchingCatalog(t *testing.T) {
	reader := NewReader(nil, testLogger{})
	selection := types.NormalizeTemperatureSelection(types.TemperatureSelection{Compact: true})
	if reader.catalog.matches(selection) {
		t.Fatal("an empty catalog must not allow compact reads")
	}

	readThroughCatalog(reader, fullBridgeSampleForTest(), types.TemperatureSelection{})
	if !reader.catalog.matches(selection) {
		t.Fatal("a full read with the same selection must allow compact reads")
	}
	for name, changed := range map[string]types.TemperatureSelection{
		"gpu-device":  {GpuDevice: "nvidia:0:GPU"},
		"gpu-sensor":  {GpuSensor: "gpu/Core"},
		"disable-gpu": {DisableGpu: true},
	} {
		if reader.catalog.matches(types.NormalizeTemperatureSelection(changed)) {
			t.Fatalf("%s change must invalidate the cached catalog", name)
		}
	}
	// CPU 传感器选择、控温来源、扩展传感器不影响缓存的清单。
	same := types.NormalizeTemperatureSelection(types.TemperatureSelection{
		TempSource: types.TempSourceGPU, CpuSensor: "cpu/Package", CpuSensors: []string{"cpu/Core #1"}, ExtendedSensors: true,
	})
	if !reader.catalog.matches(same) {
		t.Fatal("CPU sensor / source / extended changes must not invalidate the catalog")
	}
}

// 失败的完整读取不能覆盖缓存；精简结果也不会写入缓存。
func TestCatalogOnlyUpdatedByFullSuccess(t *testing.T) {
	reader := NewReader(nil, testLogger{})
	full := fullBridgeSampleForTest()
	readThroughCatalog(reader, full, types.TemperatureSelection{})

	failed := types.BridgeTemperatureData{Success: false, Error: "boom"}
	readThroughCatalog(reader, failed, types.TemperatureSelection{})
	if !slices.Equal(reader.catalog.cpuSensors, full.CpuSensors) {
		t.Fatal("a failed read replaced the cached catalog")
	}

	compact := compactOf(full, []string{"cpu/Core #1"})
	compact.CpuSensors[0].Value = 99
	readThroughCatalog(reader, compact, types.TemperatureSelection{Compact: true, CpuSensors: []string{"cpu/Core #1"}})
	if reader.catalog.cpuSensors[0].Value != 58 {
		t.Fatal("a compact result must not overwrite the cached catalog")
	}
}

// 停用 GPU 监测时回填的清单同样不带 GPU 内容。
func TestCompactReadKeepsGpuStrippedWhenDisabled(t *testing.T) {
	reader := NewReader(nil, testLogger{})
	selection := types.TemperatureSelection{DisableGpu: true}
	readThroughCatalog(reader, fullBridgeSampleForTest(), selection)

	selection.Compact = true
	temp := readThroughCatalog(reader, compactOf(fullBridgeSampleForTest(), nil), selection)
	if temp.GpuSensors != nil || temp.GpuDevices != nil || temp.GpuPowerSensors != nil || temp.GPUTemp != 0 {
		t.Fatalf("GPU data leaked into a GPU-disabled compact result: %+v", temp)
	}
	if len(temp.CpuSensors) != 3 {
		t.Fatalf("CPU catalog should still be filled, got %d sensors", len(temp.CpuSensors))
	}
}

// 与桥接之间的 JSON 约定：compact=false 时请求与旧版完全相同；桥接（C#，属性名大写）
// 回传的 Compact 能被识别；旧版桥接不带该字段时按完整结果处理。
func TestCompactBridgeJSONContract(t *testing.T) {
	plain, err := json.Marshal(types.TemperatureSelection{TempSource: "max"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "compact") {
		t.Fatalf("non-compact request must not mention compact: %s", plain)
	}
	compact, err := json.Marshal(types.TemperatureSelection{TempSource: "max", Compact: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compact), `"compact":true`) {
		t.Fatalf("compact request must carry the flag: %s", compact)
	}

	var response types.BridgeResponse
	if err := json.Unmarshal([]byte(`{"Success":true,"Error":"","Data":{"CpuTemp":61,"CpuSensors":[{"Key":"cpu/Core #1","Name":"Core #1","Value":61}],"GpuSensors":null,"GpuDevices":null,"Success":true,"Compact":true}}`), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data == nil || !response.Data.Compact || response.Data.GpuSensors != nil || len(response.Data.CpuSensors) != 1 {
		t.Fatalf("compact bridge response not parsed as compact: %+v", response.Data)
	}

	var legacy types.BridgeResponse
	if err := json.Unmarshal([]byte(`{"Success":true,"Data":{"CpuTemp":61,"CpuSensors":[],"GpuDevices":[],"Success":true}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Data.Compact {
		t.Fatal("a response from an old bridge must be treated as full")
	}
}
