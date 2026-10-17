package coreapp

import "github.com/TIANLI0/THRM/internal/types"

// trayProfileName 是托盘只需要的曲线方案标识与名称（不含曲线点）。
type trayProfileName struct {
	ID   string
	Name string
}

// trayConfigSnapshot 是托盘回调用到的少量配置字段。
type trayConfigSnapshot struct {
	AutoControl             bool
	ActiveFanCurveProfileID string
	DisableGpuMonitoring    bool
	FanCurveProfiles        []trayProfileName
}

// trayConfigView 在配置读锁内只拷出托盘需要的字段。托盘每 5 秒刷新一次状态，
// 用 Get 会每次深拷贝整份配置（含各方案曲线、学习偏移、散热收益报告）。
func (a *CoreApp) trayConfigView() trayConfigSnapshot {
	var snapshot trayConfigSnapshot
	a.configManager.View(func(cfg *types.AppConfig) {
		snapshot.AutoControl = cfg.AutoControl
		snapshot.ActiveFanCurveProfileID = cfg.ActiveFanCurveProfileID
		snapshot.DisableGpuMonitoring = cfg.DisableGpuMonitoring
		snapshot.FanCurveProfiles = make([]trayProfileName, len(cfg.FanCurveProfiles))
		for i, profile := range cfg.FanCurveProfiles {
			snapshot.FanCurveProfiles[i] = trayProfileName{ID: profile.ID, Name: profile.Name}
		}
	})
	return snapshot
}
