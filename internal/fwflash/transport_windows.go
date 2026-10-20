//go:build windows

package fwflash

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrDeviceNotFound 表示没找到处于 ISP 下载模式的 WCH 设备（4348/1A86:55E0）。
// 上层应提示用户：按住下载按键后插入 USB。
var ErrDeviceNotFound = errors.New("fwflash: 未检测到处于下载模式的设备（请按住按键后插入 USB）")

// ErrDriverMissing 表示设备在但未绑定 WinUSB 驱动。
var ErrDriverMissing = errors.New("fwflash: 设备已连接但未安装 WinUSB 驱动")

// libwdi/Zadig 为 WCH ISP 设备注册的默认接口 GUID：{A7CF86DA-49BA-4B6C-9BB9-077394139BA1}
var winUSBInterfaceGUID = windows.GUID{
	Data1: 0xA7CF86DA, Data2: 0x49BA, Data3: 0x4B6C,
	Data4: [8]byte{0x9B, 0xB9, 0x07, 0x73, 0x94, 0x13, 0x9B, 0xA1},
}

const (
	endpointOut = 0x02
	endpointIn  = 0x82

	digcfPresent         = 0x02
	digcfDeviceInterface = 0x10

	fileFlagOverlapped = 0x40000000
)

var (
	modSetupapi = windows.NewLazySystemDLL("setupapi.dll")
	modWinUSB   = windows.NewLazySystemDLL("winusb.dll")

	procSetupDiGetClassDevsW             = modSetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces      = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList     = modSetupapi.NewProc("SetupDiDestroyDeviceInfoList")

	procWinUsbInitialize          = modWinUSB.NewProc("WinUsb_Initialize")
	procWinUsbFree                = modWinUSB.NewProc("WinUsb_Free")
	procWinUsbWritePipe           = modWinUSB.NewProc("WinUsb_WritePipe")
	procWinUsbReadPipe            = modWinUSB.NewProc("WinUsb_ReadPipe")
	procWinUsbAbortPipe           = modWinUSB.NewProc("WinUsb_AbortPipe")
	procWinUsbGetOverlappedResult = modWinUSB.NewProc("WinUsb_GetOverlappedResult")
)

type spDeviceInterfaceData struct {
	cbSize             uint32
	interfaceClassGUID windows.GUID
	flags              uint32
	reserved           uintptr
}

// usbTransport 是基于 WinUSB 的 ISP 通道（重叠 I/O）。
type usbTransport struct {
	fileHandle windows.Handle
	winusb     uintptr
}

// OpenUSB 查找并打开处于下载模式的 WCH ISP 设备。WinUSB 要求句柄以重叠 I/O 打开。
func OpenUSB() (Transport, error) {
	path, err := findDevicePath()
	if err != nil {
		return nil, err
	}

	p, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|fileFlagOverlapped, 0)
	if err != nil {
		return nil, fmt.Errorf("打开设备失败: %w", err)
	}

	var winusb uintptr
	r, _, e := procWinUsbInitialize.Call(uintptr(h), uintptr(unsafe.Pointer(&winusb)))
	if r == 0 {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("%w: WinUsb_Initialize 失败: %v", ErrDriverMissing, e)
	}
	return &usbTransport{fileHandle: h, winusb: winusb}, nil
}

// doPipe 在给定管道上做一次重叠 I/O（读或写），带超时与超时中止。
func (u *usbTransport) doPipe(proc *windows.LazyProc, pipe byte, buf []byte, timeout time.Duration) (uint32, error) {
	ev, err := windows.CreateEvent(nil, 1, 0, nil) // 手动复位、初始未触发
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(ev)
	ov := windows.Overlapped{HEvent: ev}

	var transferred uint32
	r, _, e := proc.Call(u.winusb, uintptr(pipe),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&transferred)), uintptr(unsafe.Pointer(&ov)))
	if r != 0 {
		return transferred, nil // 同步即完成
	}
	if e != windows.ERROR_IO_PENDING {
		return 0, fmt.Errorf("管道 0x%02X I/O 失败: %v", pipe, e)
	}

	ms := uint32(timeout / time.Millisecond)
	if ms == 0 {
		ms = 1
	}
	w, _ := windows.WaitForSingleObject(ev, ms)
	if w != windows.WAIT_OBJECT_0 {
		procWinUsbAbortPipe.Call(u.winusb, uintptr(pipe))
		return 0, fmt.Errorf("管道 0x%02X 超时", pipe)
	}
	r, _, e = procWinUsbGetOverlappedResult.Call(u.winusb,
		uintptr(unsafe.Pointer(&ov)), uintptr(unsafe.Pointer(&transferred)), 0)
	if r == 0 {
		return 0, fmt.Errorf("管道 0x%02X 取结果失败: %v", pipe, e)
	}
	return transferred, nil
}

// findDevicePath 逐个尝试注册表里发现的接口 GUID，返回首个可枚举到的设备路径。
func findDevicePath() (string, error) {
	guids := candidateInterfaceGUIDs()
	for i := range guids {
		if path, ok := findDevicePathForGUID(&guids[i]); ok {
			return path, nil
		}
	}
	return "", ErrDeviceNotFound
}

// findDevicePathForGUID 枚举给定接口 GUID 的首个 present 设备接口路径。
func findDevicePathForGUID(guid *windows.GUID) (string, bool) {
	r, _, _ := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(guid)), 0, 0,
		uintptr(digcfPresent|digcfDeviceInterface))
	hDevInfo := windows.Handle(r)
	if hDevInfo == windows.InvalidHandle {
		return "", false
	}
	defer procSetupDiDestroyDeviceInfoList.Call(uintptr(hDevInfo))

	for i := uint32(0); ; i++ {
		var ifData spDeviceInterfaceData
		ifData.cbSize = uint32(unsafe.Sizeof(ifData))
		r, _, _ := procSetupDiEnumDeviceInterfaces.Call(
			uintptr(hDevInfo), 0,
			uintptr(unsafe.Pointer(guid)),
			uintptr(i), uintptr(unsafe.Pointer(&ifData)))
		if r == 0 {
			break // 枚举结束
		}

		// 先问所需长度
		var required uint32
		procSetupDiGetDeviceInterfaceDetailW.Call(
			uintptr(hDevInfo), uintptr(unsafe.Pointer(&ifData)),
			0, 0, uintptr(unsafe.Pointer(&required)), 0)
		if required == 0 {
			continue
		}
		buf := make([]byte, required)
		// SP_DEVICE_INTERFACE_DETAIL_DATA_W.cbSize：64 位上为 8。
		*(*uint32)(unsafe.Pointer(&buf[0])) = 8
		r, _, _ = procSetupDiGetDeviceInterfaceDetailW.Call(
			uintptr(hDevInfo), uintptr(unsafe.Pointer(&ifData)),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(required),
			0, 0)
		if r == 0 {
			continue
		}
		// 路径从 cbSize(4 字节) 之后开始，是 UTF-16 以 NUL 结尾。
		pathPtr := (*uint16)(unsafe.Pointer(&buf[4]))
		return windows.UTF16PtrToString(pathPtr), true
	}
	return "", false
}

func (u *usbTransport) SendRaw(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	_, err := u.doPipe(procWinUsbWritePipe, endpointOut, b, 2*time.Second)
	return err
}

func (u *usbTransport) RecvRaw(timeout time.Duration) ([]byte, error) {
	buf := make([]byte, 64)
	n, err := u.doPipe(procWinUsbReadPipe, endpointIn, buf, timeout)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (u *usbTransport) Close() error {
	if u.winusb != 0 {
		procWinUsbFree.Call(u.winusb)
		u.winusb = 0
	}
	if u.fileHandle != 0 {
		windows.CloseHandle(u.fileHandle)
		u.fileHandle = 0
	}
	return nil
}
