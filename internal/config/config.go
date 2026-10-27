// Package config 提供配置管理功能
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/logger"
	"github.com/TIANLI0/THRM/internal/types"
)

// Manager 配置管理器
//
// 注意：内部所有对 config 字段的读写均通过 mu 保护，避免并发场景下的数据竞争。
// 公共方法（Get/Set/Update/Save/Load）是并发安全的；不要在持有 mu 的情况下调用
// 这些公共方法（会自死锁）；内部需要无锁版本时使用以 Locked 结尾的私有方法。
type Manager struct {
	mu         sync.RWMutex
	config     types.AppConfig
	revision   uint64
	installDir string
	logger     types.Logger
}

// NewManager 创建新的配置管理器
func NewManager(installDir string, logger types.Logger) *Manager {
	return &Manager{
		installDir: installDir,
		logger:     logger,
	}
}

// Load 加载配置
func (m *Manager) Load(isAutoStart bool) types.AppConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	defaultConfigPath := configFilePath(m.GetDefaultConfigDir())

	m.logInfo("尝试从默认目录加载配置: %s", defaultConfigPath)

	if defaultConfigPath != "" && m.tryLoadFromPathLocked(defaultConfigPath) {
		m.config.ConfigPath = defaultConfigPath
		m.logInfo("从默认目录加载配置成功: %s", defaultConfigPath)
		m.bumpRevisionLocked()
		return m.config.Clone()
	}

	for _, legacyConfigDir := range m.GetLegacyConfigDirs() {
		legacyConfigPath := configFilePath(legacyConfigDir)
		m.logInfo("尝试从旧目录加载配置: %s", legacyConfigPath)
		if m.tryLoadFromPathLocked(legacyConfigPath) {
			m.config.ConfigPath = defaultConfigPath
			m.logInfo("从旧目录加载配置成功，将迁移到新目录: %s", legacyConfigPath)
			if err := m.saveLocked(); err != nil {
				m.logError("迁移旧目录配置失败: %v", err)
			}
			m.bumpRevisionLocked()
			return m.config.Clone()
		}
	}

	// 只为旧版便携安装保留读取兼容。Linux 上即使读取成功，也会迁移到
	// XDG_CONFIG_HOME，后续保存绝不会再写安装目录。
	installConfigPath := configFilePath(filepath.Join(m.installDir, "config"))
	if m.installDir != "" {
		m.logInfo("尝试从安装目录加载旧配置: %s", installConfigPath)
		if m.tryLoadFromPathLocked(installConfigPath) {
			if shouldMigrateInstallConfig() {
				m.config.ConfigPath = defaultConfigPath
				m.logInfo("从安装目录加载配置成功，将迁移到用户配置目录: %s", installConfigPath)
				if err := m.saveLocked(); err != nil {
					m.config.ConfigPath = installConfigPath
					m.logError("迁移安装目录配置失败: %v", err)
				}
			} else {
				m.config.ConfigPath = installConfigPath
				m.logInfo("从安装目录加载配置成功: %s", installConfigPath)
			}
			m.bumpRevisionLocked()
			return m.config.Clone()
		}
	}

	m.logError("所有配置目录加载失败，使用默认配置")

	m.config = types.GetDefaultConfig(isAutoStart)
	m.config.ConfigPath = defaultConfigPath
	if err := m.saveLocked(); err != nil {
		m.logError("保存默认配置失败: %v", err)
	}
	m.bumpRevisionLocked()

	return m.config.Clone()
}

// tryLoadFromPathLocked 尝试从指定路径加载配置（调用方需持有 m.mu）
func (m *Manager) tryLoadFromPathLocked(configPath string) bool {
	if _, err := os.Stat(configPath); err != nil {
		m.logDebug("配置文件不存在: %s", configPath)
		return false
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		m.logError("读取配置文件失败 %s: %v", configPath, err)
		return false
	}

	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		m.logError("解析配置文件元数据失败 %s: %v", configPath, err)
		m.preserveCorruptConfig(configPath)
		return false
	}

	var config types.AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		m.logError("解析配置文件失败 %s: %v", configPath, err)
		m.preserveCorruptConfig(configPath)
		return false
	}

	applyMissingHotkeyDefaults(&config, rawConfig)
	applyMissingSmartControlDefaults(&config, rawConfig)
	applyMissingLegionFnQDefaults(&config, rawConfig)
	applyMissingThemeDefaults(&config, rawConfig)
	applyMissingTemperatureDefaults(&config, rawConfig)
	applyMissingFanFeatureDefaults(&config, rawConfig)
	applyMissingRTSSDefaults(&config, rawConfig)
	applyMissingUILanguageDefaults(&config, rawConfig)

	m.config = config
	return true
}

func applyMissingRTSSDefaults(cfg *types.AppConfig, rawConfig map[string]json.RawMessage) {
	if cfg == nil {
		return
	}

	defaults := types.GetDefaultRTSSConfig()
	rawRTSS, ok := rawConfig["rtss"]
	if !ok {
		cfg.RTSS = defaults
		return
	}

	var rtssConfig map[string]json.RawMessage
	if err := json.Unmarshal(rawRTSS, &rtssConfig); err != nil {
		cfg.RTSS = defaults
		return
	}
	if _, ok := rtssConfig["enabled"]; !ok {
		cfg.RTSS.Enabled = defaults.Enabled
	}
	if _, ok := rtssConfig["updateIntervalMs"]; !ok {
		cfg.RTSS.UpdateIntervalMS = defaults.UpdateIntervalMS
	}
	if _, ok := rtssConfig["positionMode"]; !ok {
		cfg.RTSS.PositionMode = defaults.PositionMode
	}
	if _, ok := rtssConfig["positionX"]; !ok {
		cfg.RTSS.PositionX = defaults.PositionX
	}
	if _, ok := rtssConfig["positionY"]; !ok {
		cfg.RTSS.PositionY = defaults.PositionY
	}
	cfg.RTSS, _ = types.NormalizeRTSSConfig(cfg.RTSS)
}

func applyMissingHotkeyDefaults(cfg *types.AppConfig, rawConfig map[string]json.RawMessage) {
	if cfg == nil {
		return
	}

	defaults := types.GetDefaultConfig(false)
	if _, ok := rawConfig["manualGearToggleHotkey"]; !ok {
		cfg.ManualGearToggleHotkey = defaults.ManualGearToggleHotkey
	}
	if _, ok := rawConfig["autoControlToggleHotkey"]; !ok {
		cfg.AutoControlToggleHotkey = defaults.AutoControlToggleHotkey
	}
	if _, ok := rawConfig["curveProfileToggleHotkey"]; !ok {
		cfg.CurveProfileToggleHotkey = defaults.CurveProfileToggleHotkey
	}
}

func applyMissingSmartControlDefaults(cfg *types.AppConfig, rawConfig map[string]json.RawMessage) {
	if cfg == nil {
		return
	}

	defaults := types.GetDefaultSmartControlConfig(cfg.FanCurve)
	rawSmartControl, ok := rawConfig["smartControl"]
	if !ok {
		cfg.SmartControl.FilterTransientSpike = defaults.FilterTransientSpike
		cfg.SmartControl.LearningBias = defaults.LearningBias
		return
	}

	var smartControlConfig map[string]json.RawMessage
	if err := json.Unmarshal(rawSmartControl, &smartControlConfig); err != nil {
		return
	}

	if _, ok := smartControlConfig["filterTransientSpike"]; !ok {
		cfg.SmartControl.FilterTransientSpike = defaults.FilterTransientSpike
	}
	if _, ok := smartControlConfig["learningBias"]; !ok {
		cfg.SmartControl.LearningBias = defaults.LearningBias
	}
}

func applyMissingLegionFnQDefaults(cfg *types.AppConfig, rawConfig map[string]json.RawMessage) {
	if cfg == nil {
		return
	}
	if _, ok := rawConfig["legionFnQ"]; !ok {
		cfg.LegionFnQ = types.GetDefaultLegionFnQConfig()
		return
	}
	cfg.LegionFnQ = types.NormalizeLegionFnQConfig(cfg.LegionFnQ)
}

func applyMissingThemeDefaults(cfg *types.AppConfig, rawConfig map[string]json.RawMessage) {
	if cfg == nil {
		return
	}

	defaultThemeMode := types.ThemeModeSystem
	if _, ok := rawConfig["themeMode"]; !ok {
		cfg.ThemeMode = defaultThemeMode
		return
	}

	cfg.ThemeMode = types.NormalizeThemeMode(cfg.ThemeMode)
}

func applyMissingTemperatureDefaults(cfg *types.AppConfig, rawConfig map[string]json.RawMessage) {
	if cfg == nil {
		return
	}

	defaults := types.GetDefaultTemperatureSelection()
	if _, ok := rawConfig["tempSource"]; !ok {
		cfg.TempSource = defaults.TempSource
	}
	if _, ok := rawConfig["gpuDevice"]; !ok {
		cfg.GpuDevice = defaults.GpuDevice
	}
	if _, ok := rawConfig["cpuSensor"]; !ok {
		cfg.CpuSensor = defaults.CpuSensor
	}
	if _, ok := rawConfig["gpuSensor"]; !ok {
		cfg.GpuSensor = defaults.GpuSensor
	}
	cfg.TemperatureHistoryRetentionHours = types.NormalizeTemperatureHistoryRetentionHours(cfg.TemperatureHistoryRetentionHours)
	cfg.TempSource = types.NormalizeTempSource(cfg.TempSource)
	cfg.GpuDevice = types.NormalizeDeviceSelection(cfg.GpuDevice)
	cfg.CpuSensor = types.NormalizeSensorSelection(cfg.CpuSensor)
	cfg.CpuSensors = types.NormalizeSensorSelections(cfg.CpuSensors)
	cfg.GpuSensor = types.NormalizeSensorSelection(cfg.GpuSensor)
}

// Save 保存配置（线程安全）
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

// saveLocked 保存配置（调用方需持有 m.mu 写锁）
func (m *Manager) saveLocked() error {
	defaultConfigDir := m.GetDefaultConfigDir()
	defaultConfigPath := configFilePath(defaultConfigDir)

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		m.logError("序列化配置失败: %v", err)
		return err
	}

	m.logDebug("尝试保存配置到默认目录: %s", defaultConfigPath)

	var defaultSaveErr error
	if defaultConfigDir == "" {
		defaultSaveErr = fmt.Errorf("用户配置目录不可用")
		m.logError("保存配置失败: %v", defaultSaveErr)
	} else if err := os.MkdirAll(defaultConfigDir, 0755); err != nil {
		defaultSaveErr = err
		m.logError("创建默认配置目录失败: %v", err)
	} else if err := writeFileAtomic(defaultConfigPath, data, 0644); err != nil {
		defaultSaveErr = err
		m.logError("保存配置到默认目录失败: %v", err)
	} else {
		m.setConfigPathLocked(defaultConfigPath)
		m.logInfo("配置保存到默认目录成功: %s", defaultConfigPath)
		return nil
	}

	installConfigDir := platformInstallConfigWriteDir(m.installDir)
	if installConfigDir == "" {
		return fmt.Errorf("保存配置到用户配置目录失败: %w", defaultSaveErr)
	}
	installConfigPath := configFilePath(installConfigDir)

	m.logInfo("保存到默认目录失败，尝试保存到安装目录: %s", installConfigPath)

	if err := os.MkdirAll(installConfigDir, 0755); err != nil {
		m.logError("创建安装配置目录失败: %v", err)
		return err
	}

	if err := writeFileAtomic(installConfigPath, data, 0644); err != nil {
		m.logError("保存配置到安装目录失败: %v", err)
		return err
	}

	m.setConfigPathLocked(installConfigPath)
	m.logInfo("配置保存到安装目录成功: %s", installConfigPath)
	return nil
}

// setConfigPathLocked 记录实际保存位置。路径真的变了才算配置变化，需要推进修订号，
// 否则按修订号缓存快照的调用方会一直拿着旧路径。
func (m *Manager) setConfigPathLocked(path string) {
	if m.config.ConfigPath == path {
		return
	}
	m.config.ConfigPath = path
	m.bumpRevisionLocked()
}

// writeFileAtomic 先写同目录临时文件并刷盘，再替换目标文件。
//
// 直接 os.WriteFile 会先截断再写入：写到一半时断电、蓝屏或强制关机，留下的是空文件
// 或半截 JSON，下次启动解析失败就会退回旧目录或默认配置，用户的曲线与学习结果全部丢失。
// 替换是原子的：目标文件要么是旧内容，要么是完整的新内容。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	// Windows 上 os.Rename 使用 MOVEFILE_REPLACE_EXISTING，会直接覆盖已有文件。
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		// 目标被其它进程（杀毒扫描、编辑器）以不允许删除的方式打开时无法替换。
		// 这时退回直接写入：失败返回会让调用方改写安装目录，下次加载反而读到旧文件。
		return os.WriteFile(path, data, perm)
	}
	return nil
}

// preserveCorruptConfig 把解析失败的配置文件改名保留。加载失败后会回退到默认配置并立即
// 保存，不先挪开的话损坏的文件会被覆盖，连手工恢复的机会都没有。
func (m *Manager) preserveCorruptConfig(configPath string) {
	backupPath := configPath + ".corrupt"
	if err := os.Rename(configPath, backupPath); err != nil {
		m.logError("保留损坏的配置文件失败 %s: %v", configPath, err)
		return
	}
	m.logError("已将无法解析的配置文件另存为: %s", backupPath)
}

// GetDefaultConfigDir 获取默认配置目录
func (m *Manager) GetDefaultConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		m.logError("获取用户主目录失败: %v", err)
		homeDir = ""
	}
	return appmeta.UserConfigDir(homeDir)
}

// GetLegacyConfigDir 获取旧版本配置目录
func (m *Manager) GetLegacyConfigDir() string {
	dirs := m.GetLegacyConfigDirs()
	if len(dirs) == 0 {
		return ""
	}
	return dirs[0]
}

// GetLegacyConfigDirs 获取所有旧版本配置目录，按迁移优先级排序。
func (m *Manager) GetLegacyConfigDirs() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return appmeta.LegacyUserConfigDirs(homeDir)
}

func configFilePath(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "config.json")
}

// Get 获取当前配置（线程安全，返回深拷贝）
//
// 必须是深拷贝：AppConfig 含切片/映射字段，浅拷贝会让调用方与管理器内部状态
// 共享底层数组，任何一方原地改写元素都构成数据竞争。
func (m *Manager) Get() types.AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Clone()
}

// Revision 返回当前配置修订号。任何改动配置内容的操作都会推进修订号，
// 热路径可先比较修订号，只在变化时才调用 GetWithRevision 深拷贝整份配置。
func (m *Manager) Revision() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.revision
}

func (m *Manager) GetWithRevision() (types.AppConfig, uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Clone(), m.revision
}

// Set 设置配置（线程安全）。同样深拷贝入参，避免调用方在 Set 之后继续改写
// 自己手上的切片而间接改到管理器内部状态。
func (m *Manager) Set(config types.AppConfig) {
	m.mu.Lock()
	m.config = config.Clone()
	m.bumpRevisionLocked()
	m.mu.Unlock()
}

// View 在读锁内把当前配置交给 fn 只读访问，省去 Get 的深拷贝，供热路径读取少量字段。
// fn 不得修改 cfg、不得保留其中切片/映射的引用，也不得调用 Manager 的其它方法（会自死锁）。
func (m *Manager) View(fn func(cfg *types.AppConfig)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	fn(&m.config)
}

// Mutate 在锁内对当前配置做读-改-写（不落盘）。fn 返回 false 表示未改动，此时配置与
// 修订号都保持不变。用于后台循环只改自己负责的字段：先 Get 再 Set 会把两次调用之间
// 其它路径（GUI、托盘、快捷键）写入的改动整体回退。返回值为操作后的配置副本。
func (m *Manager) Mutate(fn func(cfg *types.AppConfig) bool) (types.AppConfig, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.config.Clone()
	if !fn(&next) {
		return m.config.Clone(), false
	}
	m.config = next
	m.bumpRevisionLocked()
	return next.Clone(), true
}

// Update 更新配置并保存（线程安全，原子操作）
func (m *Manager) Update(config types.AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config.Clone()
	m.bumpRevisionLocked()
	return m.saveLocked()
}

func (m *Manager) bumpRevisionLocked() {
	m.revision++
}

// 日志辅助方法
func (m *Manager) logInfo(format string, v ...any) {
	if m.logger != nil {
		logger.Wrapped(m.logger).Info(format, v...)
	}
}

func (m *Manager) logError(format string, v ...any) {
	if m.logger != nil {
		logger.Wrapped(m.logger).Error(format, v...)
	}
}

func (m *Manager) logDebug(format string, v ...any) {
	if m.logger != nil {
		logger.Wrapped(m.logger).Debug(format, v...)
	}
}

// GetConfigDir 获取配置目录（保持向后兼容）
func (m *Manager) GetConfigDir() string {
	return m.GetDefaultConfigDir()
}

// GetInstallDir 获取安装目录
func GetInstallDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}

// GetCurrentWorkingDir 获取当前工作目录
func GetCurrentWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}

// ValidateFanCurve 验证风扇曲线是否有效
func ValidateFanCurve(curve []types.FanCurvePoint) error {
	if len(curve) < 2 {
		return fmt.Errorf("风扇曲线至少需要2个点")
	}

	for i, point := range curve {
		if point.Temperature > types.FanCurveMaxTemperature {
			return fmt.Errorf("风扇曲线第%d个点温度超出范围(最高%d°C)", i+1, types.FanCurveMaxTemperature)
		}
		if point.RPM < 0 || point.RPM > 5000 {
			return fmt.Errorf("风扇曲线第%d个点RPM超出范围(0-5000)", i+1)
		}
	}

	for i := 1; i < len(curve); i++ {
		if curve[i].Temperature <= curve[i-1].Temperature {
			return fmt.Errorf("风扇曲线温度点必须递增")
		}
		if curve[i].RPM < curve[i-1].RPM {
			return fmt.Errorf("风扇曲线转速点必须从左到右非递减")
		}
	}

	return nil
}
