//go:build !windows

package fwflash

import "errors"

// OpenUSB 在非 Windows 平台不可用（ISP 刷写依赖 WinUSB）。
func OpenUSB() (Transport, error) {
	return nil, errors.New("fwflash: USB ISP 刷写目前仅支持 Windows")
}
