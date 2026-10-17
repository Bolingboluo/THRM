package temperature

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/TIANLI0/THRM/internal/types"
)

const (
	DefaultHistoryCapacity              = 720
	DefaultHistorySampleInterval        = 5 * time.Second
	DefaultHistoryRetentionHours        = types.DefaultTemperatureHistoryRetentionHours
	MaxHistoryRetentionHours            = types.MaxTemperatureHistoryRetentionHours
	DefaultHistoryRelativePath          = "telemetry/history.bin"
	historyBinaryMagic                  = "THST"
	historyBinaryVersionLegacy   uint16 = 1
	historyBinaryVersionPower    uint16 = 2
	historyBinaryVersionFans     uint16 = 3 // v3: 追加笔记本内置 CPU/GPU 风扇转速
	historyBinaryVersionEvents   uint16 = 4 // v4: 追加时间轴事件（整文件快照的最后一版）
	historyBinaryVersion         uint16 = 5 // v5: 文件头之后是只追加的记录流
	historyEnabledFlag           uint8  = 1

	// v5 文件头与 v1–v4 布局相同：magic(4) version(2) flags(1) reserved(1)
	// sampleInterval(4) count(4) updatedAt(8)。v5 的 count 只是压缩时的点数，读取时不使用。
	historyHeaderSize = 24
	// v5 记录：tag(1) + 载荷长度(4) + 载荷 + CRC32(4，覆盖 tag/长度/载荷)。
	historyRecordOverhead   = 1 + 4 + 4
	historyRecordPoint      = uint8(1)
	historyRecordEvent      = uint8(2)
	historyPointPayloadSize = 36 // timestamp + CPU/GPU 温度 + 风扇转速 + CPU/GPU 功耗 + 笔记本 CPU/GPU 风扇

	dirtyFlushThreshold = 6
	dirtyFlushInterval  = 30 * time.Second

	// 事件比采样点稀疏得多，固定上限即可覆盖最长 24 小时保留窗口；
	// 超出后按时间从最旧的开始丢弃。
	timelineEventCapacity = 240
	// 同类事件在此窗口内重复到达视为同一次（例如 Windows 唤醒会同时发出
	// PBT_APMRESUMESUSPEND 与 PBT_APMRESUMEAUTOMATIC 两个通知）。
	timelineEventDedupeWindow = 3 * time.Second

	// 新样本比最新记录早超过该值、且墙钟本身也早于最新记录时，视为系统时钟回拨。
	historyClockRollbackTolerance = time.Minute
)

type HistoryRecorder struct {
	mutex          sync.RWMutex
	logger         types.Logger
	filePath       string
	enabled        bool
	capacity       int
	sampleInterval time.Duration
	points         []types.TemperatureHistoryPoint
	next           int
	filled         bool
	lastSampleAt   int64
	events         []types.TimelineEvent // 按时间升序，与 points 共享保留窗口
	lastEventType  string                // 去重用，独立于 events：关闭记录时也要折叠重复通知
	lastEventKey   string
	lastEventAt    int64

	dirtyCount  int
	lastFlushAt time.Time

	// 落盘状态。
	//
	// 文件是"文件头 + 只追加的记录流"：每个新采样点/事件编码成一条记录攒在 pending 里，
	// 落盘时只把这一小段追加到文件尾，而不是像 v4 那样每 30 秒把整个环形缓冲（24 小时
	// 保留时约 600 KB）重写一遍。追加表达不了"删除"，所以清空、改保留时长、时钟回拨丢点
	// 这几种情况置 needCompact，下一次落盘按内存快照整体重写（压缩）；文件里的记录数
	// 超过 compactionLimitLocked 时同样压缩，把被环形缓冲挤掉的旧点清出去。
	//
	// 以下字段除 flushMutex 外都受 mutex 保护；flushMutex 串行化所有磁盘写入，
	// "取走 pending"与"写盘"在同一把 flushMutex 之下完成，因此写入顺序与记录顺序一致，
	// 不会出现旧内容覆盖新内容的情况。
	flushMutex     sync.Mutex
	pending        []byte // 尚未写入文件的记录
	pendingRecords int
	fileRecords    int  // 文件中（上次压缩以来）的记录数
	needCompact    bool // 文件内容与"文件头 + 追加记录"的模型不一致，下次落盘必须整体重写
	unsaved        bool // 内存里有尚未落盘的变化
	writerActive   bool // 后台写盘协程正在运行
	writeRequested bool // 后台协程运行期间又有新的写盘请求
}

func NewHistoryRecorder(filePath string, capacity int, sampleInterval time.Duration, logger types.Logger) *HistoryRecorder {
	if capacity <= 0 {
		capacity = DefaultHistoryCapacity
	}
	if sampleInterval <= 0 {
		sampleInterval = DefaultHistorySampleInterval
	}

	recorder := &HistoryRecorder{
		logger:         logger,
		filePath:       filePath,
		capacity:       capacity,
		sampleInterval: sampleInterval,
		enabled:        true,
		points:         make([]types.TemperatureHistoryPoint, capacity),
		// 文件还没按 v5 校验过（不存在、旧格式或损坏）之前不能往后追加。
		needCompact: true,
	}
	recorder.load()
	return recorder
}

func (r *HistoryRecorder) SetEnabled(enabled bool) error {
	r.mutex.Lock()
	r.enabled = enabled
	if !enabled {
		r.clearLocked()
	}
	r.mutex.Unlock()
	return r.persist(true)
}

func (r *HistoryRecorder) IsEnabled() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.enabled
}

// pointsPerHourLocked 返回按当前采样间隔一小时可容纳的样本数。
func (r *HistoryRecorder) pointsPerHourLocked() int {
	if r.sampleInterval <= 0 {
		return DefaultHistoryCapacity
	}
	per := int(time.Hour / r.sampleInterval)
	if per <= 0 {
		return DefaultHistoryCapacity
	}
	return per
}

// RetentionHours 返回当前后台保留时长(小时)。
func (r *HistoryRecorder) RetentionHours() int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	hours := r.capacity / r.pointsPerHourLocked()
	if hours < 1 {
		hours = 1
	}
	return hours
}

// SetRetentionHours 调整后台保留时长；扩缩环形缓冲区并保留最新样本后落盘。
func (r *HistoryRecorder) SetRetentionHours(hours int) error {
	if hours < 1 {
		hours = 1
	}
	if hours > MaxHistoryRetentionHours {
		hours = MaxHistoryRetentionHours
	}

	r.mutex.Lock()
	newCapacity := r.pointsPerHourLocked() * hours
	if newCapacity <= 0 || newCapacity == r.capacity {
		r.mutex.Unlock()
		return nil
	}
	ordered := r.snapshotPointsLocked()
	r.capacity = newCapacity
	r.points = make([]types.TemperatureHistoryPoint, newCapacity)
	r.next = 0
	r.filled = false
	r.applyLoadedPointsLocked(ordered)
	// 保留窗口变了，事件的过期界限跟着变（缩短时要立刻丢弃窗口外的旧事件）。
	r.pruneEventsLocked(0)
	r.mutex.Unlock()
	return r.persist(true)
}

// Flush 同步把尚未落盘的记录写入文件：会等待正在进行的后台写盘结束，返回时内存中
// 已记录的全部内容都已写到盘上。退出与系统挂起前调用。
func (r *HistoryRecorder) Flush() error {
	return r.persist(false)
}

func (r *HistoryRecorder) Add(temp types.TemperatureData, fanData *types.FanData) (types.TemperatureHistoryPoint, bool) {
	if temp.CPUTemp <= 0 && temp.GPUTemp <= 0 {
		return types.TemperatureHistoryPoint{}, false
	}

	timestamp := normalizeTimestampMillis(temp.UpdateTime)
	if timestamp <= 0 {
		timestamp = time.Now().UnixMilli()
	}

	fanRPM := 0
	if fanData != nil {
		fanRPM = int(fanData.CurrentRPM)
	}

	point := types.TemperatureHistoryPoint{
		Timestamp: timestamp,
		CPUTemp:   temp.CPUTemp,
		GPUTemp:   temp.GPUTemp,
		CPUPower:  normalizeHistoryPower(temp.CPUPower),
		GPUPower:  normalizeHistoryPower(temp.GPUPower),
		FanRPM:    fanRPM,
		CPUFanRPM: temp.CPUFanRPM,
		GPUFanRPM: temp.GPUFanRPM,
	}

	r.mutex.Lock()
	if !r.enabled {
		r.mutex.Unlock()
		return types.TemperatureHistoryPoint{}, false
	}
	// 系统时钟回拨（NTP 校时、双系统 RTC、CMOS 掉电）后新样本早于最新记录，负的间隔
	// 若也按"太快"丢弃，记录会停到墙钟重新追上旧的最新时间为止，可能长达数小时。
	// 这时丢掉落在"未来"的点和事件再接着记，缓冲区保持按时间升序。
	// 只看样本时间戳不够：一次过期的读数也会早于最新记录，但那时墙钟并没有回拨。
	rollback := historyClockRollbackTolerance.Milliseconds()
	if r.lastSampleAt-timestamp > rollback && r.lastSampleAt-time.Now().UnixMilli() > rollback {
		r.discardAfterLocked(timestamp)
	}
	if r.lastSampleAt > 0 && timestamp-r.lastSampleAt < r.sampleInterval.Milliseconds() {
		r.mutex.Unlock()
		return types.TemperatureHistoryPoint{}, false
	}

	r.points[r.next] = point
	r.lastSampleAt = timestamp
	r.next = (r.next + 1) % r.capacity
	if r.next == 0 {
		r.filled = true
	}

	r.queueRecordLocked(historyRecordPoint, func(buf []byte) []byte {
		return appendHistoryPointPayload(buf, point)
	})
	r.markDirtyLocked()
	r.mutex.Unlock()
	return point, true
}

// queueRecordLocked 把一条新记录排进待追加队列。
func (r *HistoryRecorder) queueRecordLocked(tag uint8, appendPayload func([]byte) []byte) {
	r.unsaved = true
	if r.filePath == "" {
		return
	}
	r.pending = appendHistoryRecord(r.pending, tag, appendPayload)
	r.pendingRecords++
}

// markDirtyLocked 累计一次变化，攒够一批（或距上次落盘足够久）时交给后台协程写盘。
// 写盘不在调用方（控温循环）上进行：磁盘偶发卡顿（杀毒扫描、机械盘唤醒）不应拖住控温。
func (r *HistoryRecorder) markDirtyLocked() {
	r.dirtyCount++
	now := time.Now()
	if r.dirtyCount < dirtyFlushThreshold && now.Sub(r.lastFlushAt) < dirtyFlushInterval {
		return
	}
	r.dirtyCount = 0
	r.lastFlushAt = now
	if r.filePath == "" {
		return
	}
	if r.writerActive {
		r.writeRequested = true
		return
	}
	r.writerActive = true
	go r.backgroundWrite()
}

func (r *HistoryRecorder) backgroundWrite() {
	defer func() {
		if recovered := recover(); recovered != nil {
			r.logError("温度历史后台写盘异常: %v", recovered)
			r.mutex.Lock()
			r.writerActive = false
			r.writeRequested = false
			r.mutex.Unlock()
		}
	}()
	for {
		if err := r.persist(false); err != nil {
			r.logError("保存温度历史失败: %v", err)
		}
		r.mutex.Lock()
		if !r.writeRequested {
			r.writerActive = false
			r.mutex.Unlock()
			return
		}
		r.writeRequested = false
		r.mutex.Unlock()
	}
}

// SampleDue 报告用 temp 调用 Add 时是否会记录一个新样本，只读、不改变记录器状态。
// 采样前代价较高的附加读数（如笔记本风扇转速的 WMI 调用）据此只在真正要记录时才读取。
//
// 判定与 Add 保持一致；时钟回拨的情形直接返回 true（宁可多读一次，不漏记录）。
func (r *HistoryRecorder) SampleDue(temp types.TemperatureData) bool {
	if r == nil || (temp.CPUTemp <= 0 && temp.GPUTemp <= 0) {
		return false
	}
	timestamp := normalizeTimestampMillis(temp.UpdateTime)
	if timestamp <= 0 {
		timestamp = time.Now().UnixMilli()
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if !r.enabled {
		return false
	}
	rollback := historyClockRollbackTolerance.Milliseconds()
	if r.lastSampleAt-timestamp > rollback && r.lastSampleAt-time.Now().UnixMilli() > rollback {
		return true
	}
	return r.lastSampleAt <= 0 || timestamp-r.lastSampleAt >= r.sampleInterval.Milliseconds()
}

// discardAfterLocked 丢弃时间戳晚于 timestamp 的采样点与事件：时钟回拨后它们落在
// "未来"，留着会让缓冲区乱序，前端按最新时间戳裁剪保留窗口时也会以它们为准。
func (r *HistoryRecorder) discardAfterLocked(timestamp int64) {
	points := slices.DeleteFunc(r.snapshotPointsLocked(), func(p types.TemperatureHistoryPoint) bool {
		return p.Timestamp > timestamp
	})
	r.applyLoadedPointsLocked(points)
	r.events = slices.DeleteFunc(r.events, func(e types.TimelineEvent) bool {
		return e.Timestamp > timestamp
	})
	r.dirtyCount++
	// 追加记录无法表达删除，下次落盘按内存快照整体重写。
	r.needCompact = true
	r.unsaved = true
}

// AddEvent 记录一次时间轴事件。返回落库后的事件与是否真的记录了。
//
// 记录在核心侧而非 GUI 侧：GUI 多数时间是关闭的，只在前端内存里攒事件意味着
// "关着界面时断的线"永远看不到——温度曲线明明有一小时，标记却只有开界面之后的
// 那几分钟。
func (r *HistoryRecorder) AddEvent(eventType, labelKey string) (types.TimelineEvent, bool) {
	if eventType == "" || labelKey == "" {
		return types.TimelineEvent{}, false
	}

	event := types.TimelineEvent{
		Timestamp: time.Now().UnixMilli(),
		Type:      eventType,
		LabelKey:  labelKey,
	}

	r.mutex.Lock()
	// 去重状态独立于 events 维护：关闭后台记录时事件不入缓冲，但重复通知
	// （例如唤醒时 Windows 连发的两个电源事件）依然要折叠成一次，
	// 否则会向 GUI 广播出两个重叠的标记。
	// 间隔为负说明时钟回拨过，不是重复通知。
	if gap := event.Timestamp - r.lastEventAt; r.lastEventType == event.Type && r.lastEventKey == event.LabelKey &&
		r.lastEventAt > 0 && gap >= 0 && gap < timelineEventDedupeWindow.Milliseconds() {
		r.mutex.Unlock()
		return types.TimelineEvent{}, false
	}
	r.lastEventType = event.Type
	r.lastEventKey = event.LabelKey
	r.lastEventAt = event.Timestamp

	if r.enabled {
		r.events = append(r.events, event)
		// 按保留窗口/数量裁掉的旧事件留在文件里也无妨：加载时用同一规则再裁一遍，
		// 结果与这里逐次裁剪完全一致，等下次压缩时再从文件中清除。
		r.pruneEventsLocked(event.Timestamp)

		r.queueRecordLocked(historyRecordEvent, func(buf []byte) []byte {
			return appendHistoryEventPayload(buf, event)
		})
		r.markDirtyLocked()
	}
	r.mutex.Unlock()
	return event, true
}

func compareEventTime(a, b types.TimelineEvent) int {
	switch {
	case a.Timestamp < b.Timestamp:
		return -1
	case a.Timestamp > b.Timestamp:
		return 1
	default:
		return 0
	}
}

// retentionMillisLocked 返回采样点覆盖的时间跨度，事件按同一窗口过期，
// 避免图表里出现早于最旧曲线数据、无处可标的孤立标记。
func (r *HistoryRecorder) retentionMillisLocked() int64 {
	if r.sampleInterval <= 0 || r.capacity <= 0 {
		return int64(DefaultHistoryRetentionHours) * time.Hour.Milliseconds()
	}
	return int64(r.capacity) * r.sampleInterval.Milliseconds()
}

// pruneEventsLocked 按保留窗口与容量上限裁剪事件，并保证按时间升序。
func (r *HistoryRecorder) pruneEventsLocked(newestTimestamp int64) {
	if len(r.events) == 0 {
		return
	}
	if !slices.IsSortedFunc(r.events, compareEventTime) {
		slices.SortStableFunc(r.events, compareEventTime)
	}

	if newestTimestamp <= 0 {
		newestTimestamp = r.events[len(r.events)-1].Timestamp
	}
	cutoff := newestTimestamp - r.retentionMillisLocked()
	start := 0
	for start < len(r.events) && r.events[start].Timestamp < cutoff {
		start++
	}
	if len(r.events)-start > timelineEventCapacity {
		start = len(r.events) - timelineEventCapacity
	}
	if start > 0 {
		r.events = append(r.events[:0], r.events[start:]...)
	}
}

func (r *HistoryRecorder) snapshotEventsLocked() []types.TimelineEvent {
	if len(r.events) == 0 {
		return nil
	}
	return slices.Clone(r.events)
}

func (r *HistoryRecorder) Snapshot() types.TemperatureHistoryPayload {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	retentionHours := r.capacity / r.pointsPerHourLocked()
	if retentionHours < 1 {
		retentionHours = 1
	}
	return types.TemperatureHistoryPayload{
		Enabled:               r.enabled,
		SampleIntervalSeconds: int(r.sampleInterval / time.Second),
		RetentionHours:        retentionHours,
		Points:                r.snapshotPointsLocked(),
		Events:                r.snapshotEventsLocked(),
	}
}

func normalizeTimestampMillis(timestamp int64) int64 {
	if timestamp <= 0 {
		return 0
	}
	if timestamp < 1_000_000_000_000 {
		return timestamp * 1000
	}
	return timestamp
}

func normalizeHistoryPower(watts float64) float64 {
	if math.IsNaN(watts) || math.IsInf(watts, 0) || watts <= 0 || watts > 2000 {
		return 0
	}
	return float64(math.Round(watts*10)) / 10
}

func (r *HistoryRecorder) load() {
	if r.filePath == "" {
		return
	}

	if err := r.loadBinaryFile(r.filePath); err == nil {
		return
	} else if !os.IsNotExist(err) {
		r.logError("解析温度历史文件失败: %v", err)
	}
}

func (r *HistoryRecorder) loadBinaryFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return r.loadBinaryData(data)
}

func (r *HistoryRecorder) loadBinaryData(data []byte) error {
	reader := bytes.NewReader(data)
	magic := make([]byte, len(historyBinaryMagic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return err
	}
	if string(magic) != historyBinaryMagic {
		return io.ErrUnexpectedEOF
	}

	var version uint16
	if err := binary.Read(reader, binary.LittleEndian, &version); err != nil {
		return err
	}
	if version < historyBinaryVersionLegacy || version > historyBinaryVersion {
		return fmt.Errorf("unsupported history version: %d", version)
	}

	var flags uint8
	var reserved uint8
	var sampleIntervalSeconds uint32
	var count uint32
	var updatedAt int64
	if err := binary.Read(reader, binary.LittleEndian, &flags); err != nil {
		return err
	}
	if err := binary.Read(reader, binary.LittleEndian, &reserved); err != nil {
		return err
	}
	if err := binary.Read(reader, binary.LittleEndian, &sampleIntervalSeconds); err != nil {
		return err
	}
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return err
	}
	if err := binary.Read(reader, binary.LittleEndian, &updatedAt); err != nil {
		return err
	}

	var points []types.TemperatureHistoryPoint
	var events []types.TimelineEvent
	fileRecords := 0
	// clean 表示文件正好是"v5 文件头 + 完整记录流"，可以直接接着往后追加。
	// 旧格式（v1–v4）与带残缺尾巴的文件都要在下次落盘时整体重写一次。
	clean := false
	if version >= historyBinaryVersion {
		var torn bool
		points, events, fileRecords, torn = decodeHistoryRecords(data[historyHeaderSize:])
		clean = !torn
		if torn && r.logger != nil {
			// 断电或进程被杀时最后一条记录可能只写了一半，丢掉它即可，不影响之前的记录。
			r.logger.Warn("温度历史文件尾部记录不完整，已忽略残缺部分（共读取 %d 条记录）", fileRecords)
		}
	} else {
		var err error
		points, events, err = decodeLegacyHistoryBody(reader, version, count)
		if err != nil {
			return err
		}
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.enabled = flags&historyEnabledFlag != 0
	if sampleIntervalSeconds > 0 {
		r.sampleInterval = time.Duration(sampleIntervalSeconds) * time.Second
	}
	r.applyLoadedPointsLocked(points)
	r.events = events
	r.pruneEventsLocked(0)
	r.fileRecords = fileRecords
	r.needCompact = !clean
	return nil
}

// decodeLegacyHistoryBody 解析 v1–v4 整文件快照格式的采样点与事件。
func decodeLegacyHistoryBody(reader *bytes.Reader, version uint16, count uint32) ([]types.TemperatureHistoryPoint, []types.TimelineEvent, error) {
	// count 直接来自文件，损坏时可以是任意 uint32。不校验就拿去 make，会在启动时
	// 申请几十 GB，这种 OOM 无法 recover，之后每次启动都会崩。
	recordSize := 20 // timestamp + CPU/GPU 温度 + 风扇转速
	if version >= historyBinaryVersionPower {
		recordSize += 8
	}
	if version >= historyBinaryVersionFans {
		recordSize += 8
	}
	if uint64(count)*uint64(recordSize) > uint64(reader.Len()) {
		return nil, nil, fmt.Errorf("history point count out of range: %d", count)
	}
	points := make([]types.TemperatureHistoryPoint, 0, count)
	for i := uint32(0); i < count; i++ {
		var timestamp int64
		var cpuTemp int32
		var gpuTemp int32
		var fanRPM int32
		var cpuPower float32
		var gpuPower float32
		if err := binary.Read(reader, binary.LittleEndian, &timestamp); err != nil {
			return nil, nil, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &cpuTemp); err != nil {
			return nil, nil, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &gpuTemp); err != nil {
			return nil, nil, err
		}
		if err := binary.Read(reader, binary.LittleEndian, &fanRPM); err != nil {
			return nil, nil, err
		}
		var cpuFanRPM int32
		var gpuFanRPM int32
		if version >= historyBinaryVersionPower {
			if err := binary.Read(reader, binary.LittleEndian, &cpuPower); err != nil {
				return nil, nil, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &gpuPower); err != nil {
				return nil, nil, err
			}
		}
		if version >= historyBinaryVersionFans {
			if err := binary.Read(reader, binary.LittleEndian, &cpuFanRPM); err != nil {
				return nil, nil, err
			}
			if err := binary.Read(reader, binary.LittleEndian, &gpuFanRPM); err != nil {
				return nil, nil, err
			}
		}
		points = append(points, types.TemperatureHistoryPoint{
			Timestamp: normalizeTimestampMillis(timestamp),
			CPUTemp:   int(cpuTemp),
			GPUTemp:   int(gpuTemp),
			CPUPower:  normalizeHistoryPower(float64(cpuPower)),
			GPUPower:  normalizeHistoryPower(float64(gpuPower)),
			FanRPM:    int(fanRPM),
			CPUFanRPM: int(cpuFanRPM),
			GPUFanRPM: int(gpuFanRPM),
		})
	}

	// v4 起在采样点之后追加时间轴事件。旧版本文件读到这里就结束，事件为空。
	var events []types.TimelineEvent
	if version >= historyBinaryVersionEvents {
		var eventCount uint32
		if err := binary.Read(reader, binary.LittleEndian, &eventCount); err != nil {
			return nil, nil, err
		}
		if eventCount > timelineEventCapacity {
			return nil, nil, fmt.Errorf("history event count out of range: %d", eventCount)
		}
		events = make([]types.TimelineEvent, 0, eventCount)
		for i := uint32(0); i < eventCount; i++ {
			var timestamp int64
			if err := binary.Read(reader, binary.LittleEndian, &timestamp); err != nil {
				return nil, nil, err
			}
			eventType, err := readHistoryString(reader)
			if err != nil {
				return nil, nil, err
			}
			labelKey, err := readHistoryString(reader)
			if err != nil {
				return nil, nil, err
			}
			if eventType == "" || labelKey == "" {
				continue
			}
			events = append(events, types.TimelineEvent{
				Timestamp: normalizeTimestampMillis(timestamp),
				Type:      eventType,
				LabelKey:  labelKey,
			})
		}
	}
	return points, events, nil
}

// readHistoryString 读取 uint16 长度前缀的 UTF-8 字符串。
func readHistoryString(reader io.Reader) (string, error) {
	var length uint16
	if err := binary.Read(reader, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func (r *HistoryRecorder) applyLoadedPointsLocked(points []types.TemperatureHistoryPoint) {
	if len(points) > r.capacity {
		points = points[len(points)-r.capacity:]
	}
	for i := range r.points {
		r.points[i] = types.TemperatureHistoryPoint{}
	}
	copy(r.points, points)
	r.next = len(points)
	if r.next >= r.capacity {
		r.next = 0
		r.filled = true
	} else {
		r.filled = len(points) == r.capacity
	}
	if len(points) > 0 {
		r.lastSampleAt = points[len(points)-1].Timestamp
	} else {
		r.lastSampleAt = 0
	}
}

func (r *HistoryRecorder) snapshotPointsLocked() []types.TemperatureHistoryPoint {
	points := make([]types.TemperatureHistoryPoint, 0, r.pointCountLocked())
	if r.filled {
		points = append(points, r.points[r.next:]...)
		points = append(points, r.points[:r.next]...)
	} else {
		points = append(points, r.points[:r.next]...)
	}
	return points
}

func (r *HistoryRecorder) pointCountLocked() int {
	if r.filled {
		return r.capacity
	}
	return r.next
}

// compactionLimitLocked 是文件中允许累积的记录数上限：超过后下次落盘压缩重写。
// 取保留容量的两倍，24 小时保留时文件在约 0.8–1.6 MB 之间，每天整体重写约一次。
func (r *HistoryRecorder) compactionLimitLocked() int {
	return 2 * (r.capacity + timelineEventCapacity)
}

// encodeSnapshotLocked 按当前内存状态编码完整的 v5 文件（压缩用），并返回其中的记录数。
func (r *HistoryRecorder) encodeSnapshotLocked() ([]byte, int) {
	pointCount := r.pointCountLocked()
	var flags uint8
	if r.enabled {
		flags |= historyEnabledFlag
	}
	buf := make([]byte, 0, historyHeaderSize+pointCount*(historyRecordOverhead+historyPointPayloadSize)+len(r.events)*64)
	buf = append(buf, historyBinaryMagic...)
	buf = binary.LittleEndian.AppendUint16(buf, historyBinaryVersion)
	buf = append(buf, flags, 0) // flags + reserved
	buf = binary.LittleEndian.AppendUint32(buf, uint32(r.sampleInterval/time.Second))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(pointCount))
	buf = binary.LittleEndian.AppendUint64(buf, uint64(time.Now().UnixMilli()))
	appendPoint := func(p types.TemperatureHistoryPoint) {
		buf = appendHistoryRecord(buf, historyRecordPoint, func(b []byte) []byte {
			return appendHistoryPointPayload(b, p)
		})
	}
	if r.filled {
		for _, p := range r.points[r.next:] {
			appendPoint(p)
		}
		for _, p := range r.points[:r.next] {
			appendPoint(p)
		}
	} else {
		for _, p := range r.points[:r.next] {
			appendPoint(p)
		}
	}
	for _, event := range r.events {
		buf = appendHistoryRecord(buf, historyRecordEvent, func(b []byte) []byte {
			return appendHistoryEventPayload(b, event)
		})
	}
	return buf, pointCount + len(r.events)
}

// appendHistoryRecord 追加一条 v5 记录：tag + 载荷长度 + 载荷 + CRC32。
// CRC 让加载时能识别断电留下的半条记录或被填零的尾部。
func appendHistoryRecord(buf []byte, tag uint8, appendPayload func([]byte) []byte) []byte {
	start := len(buf)
	buf = append(buf, tag, 0, 0, 0, 0)
	buf = appendPayload(buf)
	binary.LittleEndian.PutUint32(buf[start+1:start+5], uint32(len(buf)-start-5))
	return binary.LittleEndian.AppendUint32(buf, crc32.ChecksumIEEE(buf[start:]))
}

// appendHistoryPointPayload 与 v2–v4 的单点布局相同：timestamp, CPU/GPU 温度, 风扇转速,
// CPU/GPU 功耗, 笔记本 CPU/GPU 风扇转速。
func appendHistoryPointPayload(buf []byte, p types.TemperatureHistoryPoint) []byte {
	buf = binary.LittleEndian.AppendUint64(buf, uint64(normalizeTimestampMillis(p.Timestamp)))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(int32(p.CPUTemp)))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(int32(p.GPUTemp)))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(int32(p.FanRPM)))
	buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(normalizeHistoryPower(p.CPUPower))))
	buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(normalizeHistoryPower(p.GPUPower))))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(int32(p.CPUFanRPM)))
	return binary.LittleEndian.AppendUint32(buf, uint32(int32(p.GPUFanRPM)))
}

// appendHistoryEventPayload 与 v4 的单个事件布局相同：timestamp + 两个长度前缀字符串。
func appendHistoryEventPayload(buf []byte, event types.TimelineEvent) []byte {
	buf = binary.LittleEndian.AppendUint64(buf, uint64(normalizeTimestampMillis(event.Timestamp)))
	buf = appendHistoryString(buf, event.Type)
	return appendHistoryString(buf, event.LabelKey)
}

func decodeHistoryPointPayload(payload []byte) types.TemperatureHistoryPoint {
	le := binary.LittleEndian
	return types.TemperatureHistoryPoint{
		Timestamp: normalizeTimestampMillis(int64(le.Uint64(payload[0:8]))),
		CPUTemp:   int(int32(le.Uint32(payload[8:12]))),
		GPUTemp:   int(int32(le.Uint32(payload[12:16]))),
		FanRPM:    int(int32(le.Uint32(payload[16:20]))),
		CPUPower:  normalizeHistoryPower(float64(math.Float32frombits(le.Uint32(payload[20:24])))),
		GPUPower:  normalizeHistoryPower(float64(math.Float32frombits(le.Uint32(payload[24:28])))),
		CPUFanRPM: int(int32(le.Uint32(payload[28:32]))),
		GPUFanRPM: int(int32(le.Uint32(payload[32:36]))),
	}
}

// decodeHistoryRecords 顺序解析 v5 记录流。遇到不完整、CRC 不符或无法解析的记录即停止，
// 之前的记录照常返回，torn 置真：断电/崩溃只会损坏最后一次追加，不应拖累整份历史，
// 更不能让启动失败。records 是成功解析的记录数（含跳过的未知类型）。
func decodeHistoryRecords(body []byte) (points []types.TemperatureHistoryPoint, events []types.TimelineEvent, records int, torn bool) {
	le := binary.LittleEndian
	offset := 0
	for offset < len(body) {
		remaining := len(body) - offset
		if remaining < historyRecordOverhead {
			return points, events, records, true
		}
		tag := body[offset]
		length := le.Uint32(body[offset+1 : offset+5])
		if tag == 0 || uint64(length) > uint64(remaining-historyRecordOverhead) {
			return points, events, records, true
		}
		end := offset + 5 + int(length)
		if crc32.ChecksumIEEE(body[offset:end]) != le.Uint32(body[end:end+4]) {
			return points, events, records, true
		}
		payload := body[offset+5 : end]
		switch tag {
		case historyRecordPoint:
			if len(payload) != historyPointPayloadSize {
				return points, events, records, true
			}
			points = append(points, decodeHistoryPointPayload(payload))
		case historyRecordEvent:
			if len(payload) < 8 {
				return points, events, records, true
			}
			reader := bytes.NewReader(payload[8:])
			eventType, err := readHistoryString(reader)
			if err != nil {
				return points, events, records, true
			}
			labelKey, err := readHistoryString(reader)
			if err != nil {
				return points, events, records, true
			}
			if eventType != "" && labelKey != "" {
				events = append(events, types.TimelineEvent{
					Timestamp: normalizeTimestampMillis(int64(le.Uint64(payload[0:8]))),
					Type:      eventType,
					LabelKey:  labelKey,
				})
			}
		default:
			// 未知记录类型（更新的版本写入）：校验通过就跳过，不影响其余记录。
		}
		records++
		offset = end + 4
	}
	return points, events, records, false
}

// appendHistoryString 以 uint16 长度前缀写入字符串。
func appendHistoryString(buf []byte, value string) []byte {
	if len(value) > int(^uint16(0)) {
		value = value[:^uint16(0)]
	}
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(value)))
	return append(buf, value...)
}

// persist 把内存中尚未落盘的变化写入文件，在调用方协程上同步完成。
//
// 常态下只把 pending 里攒下的几条记录追加到文件尾；needCompact、force 或文件记录数
// 超过上限时按内存快照整体重写。"取走待写内容"与"写盘"都在 flushMutex 之下，
// 并发的 Flush 与后台写盘因此严格按顺序落盘，返回时调用前记录的内容必然已经在盘上。
func (r *HistoryRecorder) persist(force bool) error {
	if r.filePath == "" {
		return nil
	}
	r.flushMutex.Lock()
	defer r.flushMutex.Unlock()

	r.mutex.Lock()
	if !force && !r.unsaved {
		r.mutex.Unlock()
		return nil
	}
	compact := force || r.needCompact || r.fileRecords+r.pendingRecords > r.compactionLimitLocked()
	var data []byte
	var records int
	if compact {
		data, records = r.encodeSnapshotLocked()
	} else {
		data, records = r.pending, r.pendingRecords
	}
	r.pending = nil
	r.pendingRecords = 0
	r.needCompact = false
	r.unsaved = false
	r.dirtyCount = 0
	r.lastFlushAt = time.Now()
	r.mutex.Unlock()

	var err error
	if compact {
		err = r.replaceFile(data)
	} else if err = r.appendFile(data); err != nil {
		// 追加失败后文件尾部状态未知（可能只写进去半条），也可能文件已被删除：
		// 立刻按内存快照整体重写，内存里的数据是完整的。
		r.mutex.Lock()
		data, records = r.encodeSnapshotLocked()
		r.pending = nil
		r.pendingRecords = 0
		r.unsaved = false
		r.mutex.Unlock()
		compact = true
		err = r.replaceFile(data)
	}

	r.mutex.Lock()
	switch {
	case err != nil:
		// 盘上状态不可信，下次（攒够一批或 Flush 时）整体重写。
		r.needCompact = true
		r.unsaved = true
	case compact:
		r.fileRecords = records
	default:
		r.fileRecords += records
	}
	r.mutex.Unlock()
	return err
}

// appendFile 把记录追加到已有的 v5 文件尾。文件不存在时报错而不是新建：
// 没有文件头的记录流无法加载，交给调用方改走整体重写。
func (r *HistoryRecorder) appendFile(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	file, err := os.OpenFile(r.filePath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// replaceFile 用临时文件 + 重命名整体替换历史文件（压缩/迁移/清空时使用）。
func (r *HistoryRecorder) replaceFile(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(r.filePath), 0755); err != nil {
		return err
	}
	tmpPath := r.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	// os.Rename 在 Windows 上也会直接替换已存在的目标；先删目标会留出一段没有
	// history.bin 的窗口，这时崩溃或断电就丢掉全部历史。
	if err := os.Rename(tmpPath, r.filePath); err != nil {
		_ = os.Remove(tmpPath)
		if err := os.WriteFile(r.filePath, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (r *HistoryRecorder) clearLocked() {
	for i := range r.points {
		r.points[i] = types.TemperatureHistoryPoint{}
	}
	r.next = 0
	r.filled = false
	r.lastSampleAt = 0
	r.events = nil
}

func (r *HistoryRecorder) logError(format string, args ...any) {
	if r.logger != nil {
		r.logger.Error(format, args...)
	}
}
