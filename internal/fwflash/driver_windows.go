//go:build windows

package fwflash

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// WCH ISP 设备的硬件 ID（下载模式）。不同主板/系统可能枚举成 4348 或 1A86。
var ispHardwareIDs = []string{
	`USB\VID_4348&PID_55E0`,
	`USB\VID_1A86&PID_55E0`,
}

const (
	installFlagForce = 0x00000001 // INSTALLFLAG_FORCE
	inboxWinUSBInf   = `C:\Windows\INF\winusb.inf`
)

var (
	modNewdev                           = windows.NewLazySystemDLL("newdev.dll")
	procUpdateDriverForPlugAndPlayDevsW = modNewdev.NewProc("UpdateDriverForPlugAndPlayDevicesW")

	modCfgmgr32          = windows.NewLazySystemDLL("cfgmgr32.dll")
	procCMLocateDevNodeW = modCfgmgr32.NewProc("CM_Locate_DevNodeW")
)

// devNodePresent 用 CM_Locate_DevNodeW(NORMAL) 判断某设备实例当前是否在线。
func devNodePresent(instanceID string) bool {
	idPtr, err := windows.UTF16PtrFromString(instanceID)
	if err != nil {
		return false
	}
	var devInst uint32
	r, _, _ := procCMLocateDevNodeW.Call(
		uintptr(unsafe.Pointer(&devInst)),
		uintptr(unsafe.Pointer(idPtr)),
		0) // CM_LOCATE_DEVNODE_NORMAL：只定位 present 设备
	return r == 0 // CR_SUCCESS
}

// DriverStatus 描述 ISP 设备与其驱动的当前状态。
type DriverStatus struct {
	DevicePresent bool // 设备是否处于下载模式（已枚举）
	WinUSBBound   bool // 是否已绑定 WinUSB 驱动（可直接刷写）
	InstanceID    string
}

// QueryDriverStatus 只读检查：设备在不在下载模式、驱动绑没绑 WinUSB。无需管理员。
func QueryDriverStatus() DriverStatus {
	for _, hwid := range ispHardwareIDs {
		enumPath := `SYSTEM\CurrentControlSet\Enum\` + hwid
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, enumPath, registry.READ)
		if err != nil {
			continue
		}
		instances, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, inst := range instances {
			instKey := enumPath + `\` + inst
			ik, err := registry.OpenKey(registry.LOCAL_MACHINE, instKey, registry.READ)
			if err != nil {
				continue
			}
			svc, _, _ := ik.GetStringValue("Service")
			ik.Close()
			instanceID := hwid + `\` + inst
			if !devNodePresent(instanceID) {
				continue // 幽灵设备（已拔出），跳过
			}
			return DriverStatus{
				DevicePresent: true,
				WinUSBBound:   strings.EqualFold(svc, "WinUSB"),
				InstanceID:    instanceID,
			}
		}
	}
	return DriverStatus{}
}

// candidateInterfaceGUIDs 从注册表收集 55E0 设备注册过的接口 GUID，
// 末尾追加 libwdi 默认 GUID 作为兜底。这样无论驱动是 Zadig/libwdi 还是
// THRM 用 inbox winusb.inf 装的，都能找到接口路径。
func candidateInterfaceGUIDs() []windows.GUID {
	seen := map[string]bool{}
	var out []windows.GUID
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[strings.ToUpper(s)] {
			return
		}
		if g, err := guidFromString(s); err == nil {
			seen[strings.ToUpper(s)] = true
			out = append(out, g)
		}
	}
	for _, hwid := range ispHardwareIDs {
		enumPath := `SYSTEM\CurrentControlSet\Enum\` + hwid
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, enumPath, registry.READ)
		if err != nil {
			continue
		}
		instances, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, inst := range instances {
			dp := enumPath + `\` + inst + `\Device Parameters`
			dk, err := registry.OpenKey(registry.LOCAL_MACHINE, dp, registry.READ)
			if err != nil {
				continue
			}
			if guids, _, err := dk.GetStringsValue("DeviceInterfaceGUIDs"); err == nil {
				for _, g := range guids {
					add(g)
				}
			}
			if g, _, err := dk.GetStringValue("DeviceInterfaceGUID"); err == nil {
				add(g)
			}
			dk.Close()
		}
	}
	// libwdi 默认 GUID 兜底
	out = append(out, winUSBInterfaceGUID)
	return out
}

// guidFromString 解析 "{xxxxxxxx-xxxx-...}" 形式的 GUID。
func guidFromString(s string) (windows.GUID, error) {
	return windows.GUIDFromString(s)
}

// presentISPHardwareID 返回当前在线的 ISP 设备硬件 ID（用于安装驱动）。
func presentISPHardwareID() (string, bool) {
	st := QueryDriverStatus()
	if !st.DevicePresent {
		return "", false
	}
	// InstanceID 形如 USB\VID_4348&PID_55E0\<inst>，取前两段即硬件 ID。
	parts := strings.Split(st.InstanceID, `\`)
	if len(parts) >= 2 {
		return parts[0] + `\` + parts[1], true
	}
	return "", false
}

// InstallDriver 用微软签名的 inbox winusb.inf 给当前在线的 ISP 设备绑定 WinUSB。
// 需要管理员权限（THRM Core 进程已提权）。设备必须处于下载模式。
func InstallDriver() error {
	hwid, ok := presentISPHardwareID()
	if !ok {
		return ErrDeviceNotFound
	}
	hwidPtr, _ := windows.UTF16PtrFromString(hwid)
	infPtr, _ := windows.UTF16PtrFromString(inboxWinUSBInf)
	var reboot int32
	r, _, e := procUpdateDriverForPlugAndPlayDevsW.Call(
		0,
		uintptr(unsafe.Pointer(hwidPtr)),
		uintptr(unsafe.Pointer(infPtr)),
		uintptr(installFlagForce),
		uintptr(unsafe.Pointer(&reboot)),
	)
	if r == 0 {
		return fmt.Errorf("安装 WinUSB 驱动失败（需要管理员，且设备需处于下载模式）: %v", e)
	}
	return nil
}

// UninstallDriver 移除 THRM 装的 WinUSB 绑定（走系统自带 pnputil，best-effort）。
// 用户也可随时在设备管理器里卸载。
func UninstallDriver() error {
	st := QueryDriverStatus()
	if st.InstanceID == "" {
		return fmt.Errorf("未找到可卸载的设备")
	}
	cmd := exec.Command("pnputil", "/remove-device", st.InstanceID)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("卸载失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
