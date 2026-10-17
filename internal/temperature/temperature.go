// Package temperature 提供温度读取功能
package temperature

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TIANLI0/THRM/internal/bridge"
	"github.com/TIANLI0/THRM/internal/types"
	"github.com/shirou/gopsutil/v4/sensors"
)

const (
	helperCommandTimeout = 1200 * time.Millisecond
	gpuVendorCacheTTL    = 30 * time.Second
	// 同一条桥接告警的最短重复间隔。
	bridgeWarnInterval = time.Minute
)

// pawnIoRemediationHint 与桥接侧 bridge/TempBridge/Program.cs 的 PawnIoRemediationHint
// 保持一致。CPU 温度只能由 PawnIO 读取，没有可信的替代来源，因此唯一有意义的指引
// 就是修复 PawnIO 本身——安装包随 THRM 一起分发，用户不需要自己去下载。
const pawnIoRemediationHint = "请在 THRM 内点击“重装 PawnIO”（应用已自带安装包），完成后重启 THRM；" +
	"若仍不可用，请关闭可能独占硬件传感器的其它监控软件后重试"

var (
	execHelperCommand = execCommandHiddenWithTimeout
	readTimeNow       = time.Now
)

// Reader 温度读取器
type Reader struct {
	bridgeManager *bridge.Manager
	logger        types.Logger

	cacheMutex      sync.RWMutex
	cachedGPUVendor string
	cachedVendorAt  time.Time

	// 桥接不可用时的降级读取节流状态，见 fallback.go。
	fallback fallbackState

	// 桥接告警限频：桥接长期不可用时每个采样周期都会走到告警，不限频的话
	// 一天要同步写几万行一模一样的日志。Read 也会被 IPC 的测试读取并发调用，需加锁。
	warnMutex        sync.Mutex
	lastBridgeWarn   string
	lastBridgeWarnAt time.Time

	// 最近一次完整读取的传感器清单，精简读取（selection.Compact）时用它回填结果，
	// 见 Read。Read 会被控温循环与 IPC 测试读取并发调用，需加锁。
	catalogMutex sync.Mutex
	catalog      sensorCatalog
}

// sensorCatalog 是一次完整桥接读取得到的传感器清单。
//
// 精简读取只带回标量读数，但 a.currentTemp 仍会被 GetTemperature/GetDeviceStatus 原样
// 交给刚打开的 GUI：清单若是空的，传感器下拉框会变空、首页会误判"没有 GPU 传感器"，
// 直到下一次采样（后台空闲时最长 10 秒以上）。因此用上一次完整结果回填清单；
// GUI 连上后的第一次采样就是完整读取，会立刻刷新这些读数。
// 清单内容依赖 GPU 选择与 GPU 监测开关，三者变化时缓存失效，下一次先读完整结果。
type sensorCatalog struct {
	valid           bool
	gpuDevice       string
	gpuSensor       string
	disableGpu      bool
	cpuSensors      []types.TemperatureSensor
	gpuSensors      []types.TemperatureSensor
	cpuPowerSensors []types.PowerSensor
	gpuPowerSensors []types.PowerSensor
	gpuDevices      []types.TemperatureGPUDevice
}

func (c *sensorCatalog) matches(selection types.TemperatureSelection) bool {
	return c.valid && c.gpuDevice == selection.GpuDevice && c.gpuSensor == selection.GpuSensor &&
		c.disableGpu == selection.DisableGpu
}

// bridgeReadOutcome 记录一次 Read 中桥接返回的结果形态，供清单缓存使用。
type bridgeReadOutcome struct {
	compact     bool // 桥接返回的是精简结果
	fullSuccess bool // 桥接返回了完整且有效的结果
}

// warnBridge 记录桥接告警：内容变化时立即记录，相同内容每 bridgeWarnInterval 最多一次。
func (r *Reader) warnBridge(format string, v ...any) {
	if r.logger == nil {
		return
	}
	msg := fmt.Sprintf(format, v...)
	now := time.Now()
	r.warnMutex.Lock()
	if msg == r.lastBridgeWarn && now.Sub(r.lastBridgeWarnAt) < bridgeWarnInterval {
		r.warnMutex.Unlock()
		return
	}
	r.lastBridgeWarn = msg
	r.lastBridgeWarnAt = now
	r.warnMutex.Unlock()
	r.logger.Warn("%s", msg)
}

// NewReader 创建新的温度读取器
func NewReader(bridgeManager *bridge.Manager, logger types.Logger) *Reader {
	return &Reader{
		bridgeManager: bridgeManager,
		logger:        logger,
	}
}

// Read 读取温度。
//
// selection.Compact 只是请求：还没有与当前选择匹配的完整清单可供回填时，本次照常
// 读取完整结果。精简结果的标量读数与完整读取完全一致，传感器清单由缓存回填。
func (r *Reader) Read(selection types.TemperatureSelection) types.TemperatureData {
	selection = types.NormalizeTemperatureSelection(selection)
	if selection.Compact {
		r.catalogMutex.Lock()
		selection.Compact = r.catalog.matches(selection)
		r.catalogMutex.Unlock()
	}
	temp, outcome := r.read(selection)
	r.reconcileSensorCatalog(&temp, outcome, selection)
	return temp
}

// reconcileSensorCatalog 用完整结果刷新清单缓存，或用缓存回填精简结果的清单。
// OtherSensors 不经缓存：精简结果里它照常是完整的（只在散热收益测试期间非空）。
func (r *Reader) reconcileSensorCatalog(temp *types.TemperatureData, outcome bridgeReadOutcome, selection types.TemperatureSelection) {
	r.catalogMutex.Lock()
	defer r.catalogMutex.Unlock()
	switch {
	case outcome.compact:
		if !r.catalog.matches(selection) {
			// 期间有一次按不同选择的完整读取刷新了缓存（极少见）：清单宁可留空也不能张冠李戴。
			temp.CpuSensors = nil
			return
		}
		temp.CpuSensors = r.catalog.cpuSensors
		temp.GpuSensors = r.catalog.gpuSensors
		temp.CpuPowerSensors = r.catalog.cpuPowerSensors
		temp.GpuPowerSensors = r.catalog.gpuPowerSensors
		temp.GpuDevices = r.catalog.gpuDevices
	case outcome.fullSuccess:
		r.catalog = sensorCatalog{
			valid:           true,
			gpuDevice:       selection.GpuDevice,
			gpuSensor:       selection.GpuSensor,
			disableGpu:      selection.DisableGpu,
			cpuSensors:      temp.CpuSensors,
			gpuSensors:      temp.GpuSensors,
			cpuPowerSensors: temp.CpuPowerSensors,
			gpuPowerSensors: temp.GpuPowerSensors,
			gpuDevices:      temp.GpuDevices,
		}
	}
}

func (r *Reader) read(selection types.TemperatureSelection) (types.TemperatureData, bridgeReadOutcome) {
	temp := types.TemperatureData{
		UpdateTime:    time.Now().UnixMilli(),
		BridgeOk:      true,
		ControlSource: selection.TempSource,
	}
	var outcome bridgeReadOutcome

	if r.bridgeManager != nil && r.bridgeManager.IsSupported() {
		bridgeTemp := r.bridgeManager.GetTemperature(selection)
		outcome.compact = bridgeTemp.Compact
		copyBridgeTemperatureMetadata(&temp, bridgeTemp, selection)
		if selection.DisableGpu {
			stripGpuReadings(&temp, selection.TempSource)
		}
		// 桥接正在后台完成启动握手：这是过渡态，既不该走开销高昂的降级读取，
		// 也不该被上层当成故障触发自愈重启。原样上报让上层识别。
		if !bridgeTemp.Success && bridge.IsStarting(bridgeTemp.Error) {
			temp.BridgeOk = false
			temp.BridgeMsg = bridgeTemp.Error
			return temp, outcome
		}
		if bridgeTemp.Success {
			if bridgeTemp.CpuTemp == 0 && (selection.DisableGpu || bridgeTemp.GpuTemp == 0) {
				temp.BridgeOk = false
				temp.BridgeMsg = "桥接程序未返回有效温度（CPU/GPU 均为 0）；" + pawnIoRemediationHint
				r.warnBridge("桥接程序返回空温度数据，使用备用方法")

				fallback := r.readFallback(selection.DisableGpu)
				temp.CPUTemp = fallback.cpuTemp
				temp.GPUTemp = fallback.gpuTemp
				temp.GPUPower = fallback.gpuPower
				temp.MaxTemp = max(temp.CPUTemp, temp.GPUTemp)
				temp.ControlTemp = resolveControlTemp(temp.CPUTemp, temp.GPUTemp, selection.TempSource)
				return temp, outcome
			}

			// 用户多选 CPU 传感器时，按所选传感器温度的算术平均覆盖 CPU 基准温度。
			// 精简结果的 CpuSensors 恰好只含所选传感器，平均值与完整结果相同。
			applyMultiSensorCpuAverage(&temp, selection.CpuSensors)

			temp.BridgeOk = true
			temp.BridgeMsg = ""
			outcome.fullSuccess = !bridgeTemp.Compact
			return temp, outcome
		}

		r.warnBridge("桥接程序读取温度失败: %s, 使用备用方法", bridgeTemp.Error)
		temp.BridgeOk = false
		temp.BridgeMsg = bridgeTemp.Error
		if strings.TrimSpace(temp.BridgeMsg) == "" {
			temp.BridgeMsg = "CPU/GPU 温度读取失败；" + pawnIoRemediationHint
		}
	}
	// 桥接程序为 Windows 专用（LibreHardwareMonitor/PawnIO）。在不支持的平台（如 Linux）
	// 上，内置传感器读取才是正常路径，不应作为错误提示，因此保持 BridgeOk 默认的 true。

	// 读取内置传感器（停用 GPU 监测时完全跳过 GPU，避免 nvidia-smi 等轮询唤醒独显）。
	// readFallback 内部带刷新节流与失败退避，桥接长期不可用时不会每个采样周期都起进程。
	fallback := r.readFallback(selection.DisableGpu)
	temp.CPUTemp = fallback.cpuTemp
	temp.GPUTemp = fallback.gpuTemp
	temp.GPUPower = fallback.gpuPower

	// 计算最高温度
	temp.MaxTemp = max(temp.CPUTemp, temp.GPUTemp)
	temp.ControlTemp = resolveControlTemp(temp.CPUTemp, temp.GPUTemp, selection.TempSource)

	return temp, outcome
}

// stripGpuReadings 清空 GPU 相关读数与元数据。停用 GPU 监测时即便桥接程序
// （如旧版本）仍返回 GPU 数据，也保证对外一致为“无 GPU 读数”，控温基准回退 CPU。
func stripGpuReadings(temp *types.TemperatureData, tempSource string) {
	temp.GPUTemp = 0
	temp.GPUPower = 0
	temp.GpuModel = ""
	temp.GpuSensors = nil
	temp.GpuPowerSensors = nil
	temp.GpuDevices = nil
	temp.SelectedGpuDevice = ""
	temp.MaxTemp = temp.CPUTemp
	temp.ControlTemp = resolveControlTemp(temp.CPUTemp, 0, tempSource)
}

func copyBridgeTemperatureMetadata(temp *types.TemperatureData, bridgeTemp types.BridgeTemperatureData, selection types.TemperatureSelection) {
	if temp == nil {
		return
	}

	temp.CPUTemp = bridgeTemp.CpuTemp
	temp.GPUTemp = bridgeTemp.GpuTemp
	temp.CPUPower = bridgeTemp.CpuPower
	temp.GPUPower = bridgeTemp.GpuPower
	temp.MaxTemp = bridgeTemp.MaxTemp
	temp.ControlTemp = bridgeTemp.ControlTemp
	temp.ControlSource = bridgeTemp.ControlSource
	temp.SelectedGpuDevice = bridgeTemp.SelectedGpuDevice
	if temp.ControlSource == "" {
		temp.ControlSource = selection.TempSource
	}
	if temp.ControlTemp == 0 {
		temp.ControlTemp = resolveControlTemp(temp.CPUTemp, temp.GPUTemp, temp.ControlSource)
	}
	temp.CPUTempError = bridgeTemp.CpuTempError
	temp.CpuModel = bridgeTemp.CpuModel
	temp.GpuModel = bridgeTemp.GpuModel
	temp.CpuSensors = bridgeTemp.CpuSensors
	temp.GpuSensors = bridgeTemp.GpuSensors
	temp.CpuPowerSensors = bridgeTemp.CpuPowerSensors
	temp.GpuPowerSensors = bridgeTemp.GpuPowerSensors
	temp.GpuDevices = bridgeTemp.GpuDevices
	temp.OtherSensors = bridgeTemp.OtherSensors
	if bridgeTemp.UpdateTime > 0 {
		temp.UpdateTime = bridgeTemp.UpdateTime
		if temp.UpdateTime < 1_000_000_000_000 {
			temp.UpdateTime *= 1000
		}
	}
}

// applyMultiSensorCpuAverage 当用户多选 CPU 传感器时，用所选传感器温度的算术平均
// 覆盖 CPU 基准温度，并同步刷新 MaxTemp 与 ControlTemp。未命中任何传感器时不改动。
func applyMultiSensorCpuAverage(temp *types.TemperatureData, keys []string) {
	if temp == nil {
		return
	}
	avg, ok := averageSelectedCpuTemp(temp.CpuSensors, keys)
	if !ok {
		return
	}
	temp.CPUTemp = avg
	temp.MaxTemp = max(temp.CPUTemp, temp.GPUTemp)
	temp.ControlTemp = resolveControlTemp(temp.CPUTemp, temp.GPUTemp, temp.ControlSource)
}

// averageSelectedCpuTemp 从已采集的传感器列表中按 key 匹配并计算算术平均(四舍五入)。
// keys 为空或未命中任何传感器时返回 ok=false。
func averageSelectedCpuTemp(sensors []types.TemperatureSensor, keys []string) (int, bool) {
	if len(keys) == 0 || len(sensors) == 0 {
		return 0, false
	}
	sum, n := 0, 0
	for _, key := range keys {
		for i := range sensors {
			if strings.EqualFold(sensors[i].Key, key) {
				sum += sensors[i].Value
				n++
				break
			}
		}
	}
	if n == 0 {
		return 0, false
	}
	return (sum + n/2) / n, true
}

func resolveControlTemp(cpuTemp, gpuTemp int, source string) int {
	switch types.NormalizeTempSource(source) {
	case types.TempSourceCPU:
		if cpuTemp <= 0 && gpuTemp > 0 {
			return gpuTemp
		}
		return cpuTemp
	case types.TempSourceGPU:
		if gpuTemp <= 0 && cpuTemp > 0 {
			return cpuTemp
		}
		return gpuTemp
	default:
		return max(cpuTemp, gpuTemp)
	}
}

// readSensorCPUTemp 通过 gopsutil 读取 CPU 温度。纯进程内操作，开销可忽略，
// 因此不参与降级路径的节流缓存。读不到返回 0。
func (r *Reader) readSensorCPUTemp() int {
	sensorTemps, err := sensors.SensorsTemperatures()
	if err != nil {
		return 0
	}
	for _, sensor := range sensorTemps {
		// 查找ACPI ThermalZone TZ00_0或类似的CPU温度传感器
		if strings.Contains(strings.ToLower(sensor.SensorKey), "tz00") ||
			strings.Contains(strings.ToLower(sensor.SensorKey), "cpu") ||
			strings.Contains(strings.ToLower(sensor.SensorKey), "core") {
			return int(sensor.Temperature)
		}
	}
	return 0
}

// detectGPUVendor 检测GPU厂商
func (r *Reader) detectGPUVendor() string {
	now := readTimeNow()
	r.cacheMutex.RLock()
	if cached := r.cachedGPUVendor; cached != "" && now.Sub(r.cachedVendorAt) < gpuVendorCacheTTL {
		r.cacheMutex.RUnlock()
		return cached
	}
	r.cacheMutex.RUnlock()

	vendor := "unknown"
	// 尝试NVIDIA
	if _, err := execHelperCommand(helperCommandTimeout, "nvidia-smi", "--version"); err == nil {
		vendor = "nvidia"
	} else if !errors.Is(err, context.DeadlineExceeded) {
		r.logger.Debug("检测GPU厂商失败: %v", err)
	} else {
		r.logger.Debug("检测GPU厂商超时: %v", err)
	}

	r.cacheMutex.Lock()
	r.cachedGPUVendor = vendor
	r.cachedVendorAt = now
	r.cacheMutex.Unlock()

	return vendor
}

// CalculateTargetRPM 根据温度计算目标转速
func CalculateTargetRPM(temperature int, fanCurve []types.FanCurvePoint) int {
	if len(fanCurve) < 2 {
		return 0
	}

	if temperature <= fanCurve[0].Temperature {
		return fanCurve[0].RPM
	}

	lastPoint := fanCurve[len(fanCurve)-1]
	if temperature >= lastPoint.Temperature {
		return lastPoint.RPM
	}

	// 线性插值计算转速
	for i := 0; i < len(fanCurve)-1; i++ {
		p1 := fanCurve[i]
		p2 := fanCurve[i+1]

		if temperature >= p1.Temperature && temperature <= p2.Temperature {
			// 线性插值
			ratio := float64(temperature-p1.Temperature) / float64(p2.Temperature-p1.Temperature)
			rpm := float64(p1.RPM) + ratio*float64(p2.RPM-p1.RPM)
			return int(rpm)
		}
	}

	return 0
}
