package ipc

import (
	"slices"

	"github.com/TIANLI0/THRM/internal/types"
)

// FeatureTemperatureSensorValues 声明客户端能还原 TemperatureEvent.SensorValues。
//
// 只有全部前台客户端都声明了该能力，核心才会在传感器清单不变时只发读数；
// 旧版 GUI 不认识 sensorValues，会把省略的清单当成"未变化"而一直显示旧读数。
const FeatureTemperatureSensorValues = "temperature-sensor-values"

// TemperatureEvent 是 temperature-update 事件的载荷。
//
// 嵌入的 TemperatureData 与旧协议逐字段一致；SensorValues 为空时序列化结果与直接
// 发送 TemperatureData 完全相同，旧客户端照常解析。
type TemperatureEvent struct {
	types.TemperatureData
	SensorValues *TemperatureSensorValues `json:"sensorValues,omitempty"`
}

// TemperatureSensorValues 携带"清单（key/name 等）没变、只有读数变了"的传感器读数。
//
// 对应的清单字段在同一帧里为 null。每组读数附带清单指纹，接收方只在本地缓存的清单
// 指纹一致时才按位置套用读数：带着新清单的那一帧若在队列拥塞时被丢弃，按位置套用
// 就会把读数贴到错误的传感器名下，指纹能把这种情况识别出来并改为重新拉取完整快照。
type TemperatureSensorValues struct {
	Cpu        *SensorValues[int]     `json:"cpu,omitempty"`
	Gpu        *SensorValues[int]     `json:"gpu,omitempty"`
	CpuPower   *SensorValues[float64] `json:"cpuPower,omitempty"`
	GpuPower   *SensorValues[float64] `json:"gpuPower,omitempty"`
	GpuDevices *GPUDeviceValues       `json:"gpuDevices,omitempty"`
}

// SensorValues 是一份传感器清单的读数，按清单顺序排列。
type SensorValues[T int | float64] struct {
	Catalog uint64 `json:"catalog"`
	Values  []T    `json:"values"`
}

// GPUDeviceValues 是 GPU 设备列表里每台设备的温度与功耗读数，外层按设备顺序排列。
type GPUDeviceValues struct {
	Catalog uint64      `json:"catalog"`
	Sensors [][]int     `json:"sensors"`
	Power   [][]float64 `json:"power"`
}

// CompactTemperatureEvent 在 previous 的基础上压缩 current：清单（除读数外的全部字段）
// 没变的传感器组只发读数，变了的照常发完整清单。
//
// 与旧的差分一样保留 nil 与空切片的区别（nil 表示"沿用"，[] 表示"清空"）。
// OtherSensors 原样发送，理由见 coreapp.compactTemperatureEventPayload。
func CompactTemperatureEvent(current, previous types.TemperatureData) TemperatureEvent {
	event := TemperatureEvent{TemperatureData: current}
	var values TemperatureSensorValues

	if temperatureCatalogEqual(current.CpuSensors, previous.CpuSensors) {
		event.CpuSensors = nil
		values.Cpu = temperatureValues(current.CpuSensors)
	}
	if temperatureCatalogEqual(current.GpuSensors, previous.GpuSensors) {
		event.GpuSensors = nil
		values.Gpu = temperatureValues(current.GpuSensors)
	}
	if powerCatalogEqual(current.CpuPowerSensors, previous.CpuPowerSensors) {
		event.CpuPowerSensors = nil
		values.CpuPower = powerValues(current.CpuPowerSensors)
	}
	if powerCatalogEqual(current.GpuPowerSensors, previous.GpuPowerSensors) {
		event.GpuPowerSensors = nil
		values.GpuPower = powerValues(current.GpuPowerSensors)
	}
	if gpuDeviceCatalogEqual(current.GpuDevices, previous.GpuDevices) {
		event.GpuDevices = nil
		values.GpuDevices = gpuDeviceValues(current.GpuDevices)
	}

	if values.Cpu != nil || values.Gpu != nil || values.CpuPower != nil || values.GpuPower != nil || values.GpuDevices != nil {
		event.SensorValues = &values
	}
	return event
}

// ResolveTemperatureEvent 把事件还原成完整的 TemperatureData：只带读数的传感器组
// 用 cached 里的清单加上新读数重建。
//
// 返回的数据里仍可能有 nil 清单（未变化且无读数的组，或指纹对不上的组），调用方照旧
// 用缓存补齐。ok=false 表示至少一组读数因清单指纹不一致而无法套用，调用方应当重新
// 拉取一次完整快照。
func ResolveTemperatureEvent(event TemperatureEvent, cached types.TemperatureData) (types.TemperatureData, bool) {
	resolved := event.TemperatureData
	values := event.SensorValues
	if values == nil {
		return resolved, true
	}
	ok := true
	if resolved.CpuSensors == nil && values.Cpu != nil {
		if list, applied := applyTemperatureValues(cached.CpuSensors, values.Cpu); applied {
			resolved.CpuSensors = list
		} else {
			ok = false
		}
	}
	if resolved.GpuSensors == nil && values.Gpu != nil {
		if list, applied := applyTemperatureValues(cached.GpuSensors, values.Gpu); applied {
			resolved.GpuSensors = list
		} else {
			ok = false
		}
	}
	if resolved.CpuPowerSensors == nil && values.CpuPower != nil {
		if list, applied := applyPowerValues(cached.CpuPowerSensors, values.CpuPower); applied {
			resolved.CpuPowerSensors = list
		} else {
			ok = false
		}
	}
	if resolved.GpuPowerSensors == nil && values.GpuPower != nil {
		if list, applied := applyPowerValues(cached.GpuPowerSensors, values.GpuPower); applied {
			resolved.GpuPowerSensors = list
		} else {
			ok = false
		}
	}
	if resolved.GpuDevices == nil && values.GpuDevices != nil {
		if list, applied := applyGPUDeviceValues(cached.GpuDevices, values.GpuDevices); applied {
			resolved.GpuDevices = list
		} else {
			ok = false
		}
	}
	return resolved, ok
}

// 读数为空的组（清单为空）不发：接收方本来就会沿用缓存里的空清单。

func temperatureValues(list []types.TemperatureSensor) *SensorValues[int] {
	if len(list) == 0 {
		return nil
	}
	values := make([]int, len(list))
	for i := range list {
		values[i] = list[i].Value
	}
	return &SensorValues[int]{Catalog: temperatureCatalog(list), Values: values}
}

func powerValues(list []types.PowerSensor) *SensorValues[float64] {
	if len(list) == 0 {
		return nil
	}
	values := make([]float64, len(list))
	for i := range list {
		values[i] = list[i].Value
	}
	return &SensorValues[float64]{Catalog: powerCatalog(list), Values: values}
}

func gpuDeviceValues(list []types.TemperatureGPUDevice) *GPUDeviceValues {
	if len(list) == 0 {
		return nil
	}
	out := &GPUDeviceValues{
		Catalog: gpuDeviceCatalog(list),
		Sensors: make([][]int, len(list)),
		Power:   make([][]float64, len(list)),
	}
	for i, device := range list {
		sensors := make([]int, len(device.Sensors))
		for j := range device.Sensors {
			sensors[j] = device.Sensors[j].Value
		}
		power := make([]float64, len(device.PowerSensors))
		for j := range device.PowerSensors {
			power[j] = device.PowerSensors[j].Value
		}
		out.Sensors[i] = sensors
		out.Power[i] = power
	}
	return out
}

func applyTemperatureValues(cached []types.TemperatureSensor, values *SensorValues[int]) ([]types.TemperatureSensor, bool) {
	if len(cached) != len(values.Values) || temperatureCatalog(cached) != values.Catalog {
		return nil, false
	}
	list := slices.Clone(cached)
	for i := range list {
		list[i].Value = values.Values[i]
	}
	return list, true
}

func applyPowerValues(cached []types.PowerSensor, values *SensorValues[float64]) ([]types.PowerSensor, bool) {
	if len(cached) != len(values.Values) || powerCatalog(cached) != values.Catalog {
		return nil, false
	}
	list := slices.Clone(cached)
	for i := range list {
		list[i].Value = values.Values[i]
	}
	return list, true
}

func applyGPUDeviceValues(cached []types.TemperatureGPUDevice, values *GPUDeviceValues) ([]types.TemperatureGPUDevice, bool) {
	if len(cached) != len(values.Sensors) || len(cached) != len(values.Power) || gpuDeviceCatalog(cached) != values.Catalog {
		return nil, false
	}
	list := slices.Clone(cached)
	for i := range list {
		if len(list[i].Sensors) != len(values.Sensors[i]) || len(list[i].PowerSensors) != len(values.Power[i]) {
			return nil, false
		}
		// 克隆时保留 nil 与空切片的区别，重建结果才能与发送端逐字节一致。
		list[i].Sensors = slices.Clone(list[i].Sensors)
		for j := range list[i].Sensors {
			list[i].Sensors[j].Value = values.Sensors[i][j]
		}
		list[i].PowerSensors = slices.Clone(list[i].PowerSensors)
		for j := range list[i].PowerSensors {
			list[i].PowerSensors[j].Value = values.Power[i][j]
		}
	}
	return list, true
}

// 清单比较：除 Value 外的全部字段相同，且 nil 与空切片的区别也相同。

func temperatureCatalogEqual(a, b []types.TemperatureSensor) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

func powerCatalogEqual(a, b []types.PowerSensor) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

func gpuDeviceCatalogEqual(a, b []types.TemperatureGPUDevice) bool {
	if len(a) != len(b) || (a == nil) != (b == nil) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Name != b[i].Name || a[i].Vendor != b[i].Vendor {
			return false
		}
		if !temperatureCatalogEqual(a[i].Sensors, b[i].Sensors) || !powerCatalogEqual(a[i].PowerSensors, b[i].PowerSensors) {
			return false
		}
	}
	return true
}

// 清单指纹：覆盖清单比较所看的全部字段（含 nil 与空切片的区别），两端用同一套算法。

// catalogHasher 是 64 位 FNV-1a，直接内联以免每个字段都经过 hash.Hash 接口。
type catalogHasher struct{ sum uint64 }

func newCatalogHasher() *catalogHasher {
	return &catalogHasher{sum: 14695981039346656037}
}

func (h *catalogHasher) byte(b byte) {
	h.sum ^= uint64(b)
	h.sum *= 1099511628211
}

func (h *catalogHasher) int(v int) {
	u := uint64(v)
	for range 8 {
		h.byte(byte(u))
		u >>= 8
	}
}

func (h *catalogHasher) string(s string) {
	h.int(len(s))
	for i := 0; i < len(s); i++ {
		h.byte(s[i])
	}
}

func (h *catalogHasher) temperatureList(list []types.TemperatureSensor) {
	if list == nil {
		h.byte(0)
		return
	}
	h.byte(1)
	h.int(len(list))
	for i := range list {
		h.string(list[i].Key)
		h.string(list[i].Name)
	}
}

func (h *catalogHasher) powerList(list []types.PowerSensor) {
	if list == nil {
		h.byte(0)
		return
	}
	h.byte(1)
	h.int(len(list))
	for i := range list {
		h.string(list[i].Key)
		h.string(list[i].Name)
	}
}

func temperatureCatalog(list []types.TemperatureSensor) uint64 {
	h := newCatalogHasher()
	h.temperatureList(list)
	return h.sum
}

func powerCatalog(list []types.PowerSensor) uint64 {
	h := newCatalogHasher()
	h.powerList(list)
	return h.sum
}

func gpuDeviceCatalog(list []types.TemperatureGPUDevice) uint64 {
	h := newCatalogHasher()
	if list == nil {
		h.byte(0)
		return h.sum
	}
	h.byte(1)
	h.int(len(list))
	for i := range list {
		h.string(list[i].Key)
		h.string(list[i].Name)
		h.string(list[i].Vendor)
		h.temperatureList(list[i].Sensors)
		h.powerList(list[i].PowerSensors)
	}
	return h.sum
}
