//go:build !windows

package fwflash

import "errors"

// DriverStatus 描述 ISP 设备与其驱动状态（非 Windows 恒为空）。
type DriverStatus struct {
	DevicePresent bool
	WinUSBBound   bool
	InstanceID    string
}

// QueryDriverStatus 在非 Windows 平台恒返回空状态。
func QueryDriverStatus() DriverStatus { return DriverStatus{} }

// InstallDriver 仅 Windows 支持。
func InstallDriver() error {
	return errors.New("fwflash: 驱动安装仅支持 Windows")
}

// UninstallDriver 仅 Windows 支持。
func UninstallDriver() error {
	return errors.New("fwflash: 驱动卸载仅支持 Windows")
}
