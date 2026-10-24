package device

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TIANLI0/THRM/internal/types"
	"github.com/sstallion/go-hid"
)

// BS1 的 USB 身份与报文布局参数。
//
// 与 BS2 系列完全不同：BS2 系列是 VID 0x37D7 / PID 0x1001..0x1004，报文 25 字节、
// Output Report ID 0x02、校验和留在原位。BS1 是 VID 0x04A0 / PID 0x2501，
// 报文 32 字节、Output Report ID 0x01、校验和挪到最后一字节。
//
// 依据：官方 HIDClient.Send 的 IsBS1Serial 分支；已在真机（固件 0.0.6.02）验证。
const (
	bs1HIDVendorID  = 0x04A0
	bs1HIDProductID = 0x2501

	bs1HIDOutputReportID = 0x01
	// bs1HIDInputReportID 是应答报文的首字节。这里只作说明用：解析由
	// deviceproto.ParseFrame 统一处理，它已能识别并跳过这个前缀。
	bs1HIDInputReportID = 0x03
	bs1HIDReportLen     = 32

	// bs1HIDReadBufferSize 是读缓冲区大小。BS1 报文 32 字节，留余量。
	bs1HIDReadBufferSize = 64

	// bs1HIDVendorUsagePageMin 是厂商自定义 HID 集合的 Usage Page 下限。
	// 只接受这类接口，避免误选同一设备上的键盘/鼠标集合。
	bs1HIDVendorUsagePageMin = 0xFF00

	// bs1HIDIdleSleep 是非阻塞读返回"暂无数据"后的休眠间隔。
	//
	// 沿用 Manager 侧的 hidReadPollInterval（100ms），不另设魔数：BS1 的命令量
	// 很低（温度变化才发一条 0x21，心跳 3 秒一次），不需要事务级的毫秒轮询。
	// 若将来发现多命令查询（0x27/0x25/0x45 连发）延迟偏高，再照 Manager 的
	// 做法在事务期间切到 hidBusyReadPollInterval。
	bs1HIDIdleSleep = hidReadPollInterval

	// bs1HIDCloseTimeout 是断开时等待读循环退出的上限。
	bs1HIDCloseTimeout = 3 * time.Second
)

// buildBS1HIDReport 把一帧协议数据封装成 BS1 专属的 32 字节 HID 报文。
//
// 布局（与 BS2 的 deviceproto.BuildReport 不同）：
//
//	[0]      0x01              Output Report ID
//	[1..n-1] 帧去掉校验字节
//	[n]      0x00              原本的校验位置清零
//	[31]     校验和             挪到最后一字节
//
// 其中 n = len(frame)。帧本身（5A A5 cmd len payload checksum）由
// deviceproto.BuildFrame 生成，本函数只负责 HID 这一层的封装。
//
// 返回 nil 表示帧长度非法（BS1 的报文放不下），调用方必须检查。
func buildBS1HIDReport(frame []byte) []byte {
	n := len(frame)
	if n < 2 || n >= bs1HIDReportLen {
		return nil
	}
	report := make([]byte, bs1HIDReportLen)
	report[0] = bs1HIDOutputReportID
	copy(report[1:], frame[:n-1])
	report[n] = 0x00
	report[bs1HIDReportLen-1] = frame[n-1]
	return report
}

// enumerateBS1HID 查找 BS1 的厂商自定义 HID 接口。
//
// 优先按产品名匹配（官方 App 的做法，名字取自 HID 描述符），匹配不到再退回
// VID/PID 的第一个厂商自定义集合——与 Manager 侧 pickHIDCandidates 的取舍
// 保持一致：每个产品 ID 取枚举顺序中的第一个接口。
//
// 只接受 Usage Page >= 0xFF00 的厂商自定义集合，避免误选同一设备上的
// 键盘/鼠标集合（BS1 描述符里可能有多个集合）。
func enumerateBS1HID() (path, productName string, ok bool) {
	var namedPath, namedProduct string
	var fallbackPath, fallbackProduct string

	_ = hid.Enumerate(bs1HIDVendorID, 0, func(info *hid.DeviceInfo) error {
		if info.VendorID != bs1HIDVendorID || info.ProductID != bs1HIDProductID {
			return nil
		}
		if info.UsagePage < bs1HIDVendorUsagePageMin {
			return nil
		}
		if strings.Contains(info.ProductStr, types.BS1DeviceName) {
			if namedPath == "" {
				namedPath, namedProduct = info.Path, info.ProductStr
			}
			return nil
		}
		if fallbackPath == "" {
			fallbackPath, fallbackProduct = info.Path, info.ProductStr
		}
		return nil
	})

	if namedPath != "" {
		return namedPath, namedProduct, true
	}
	if fallbackPath != "" {
		return fallbackPath, fallbackProduct, true
	}
	return "", "", false
}

// bs1HIDTransport 是 BS1 的 USB HID 传输实现。
//
// 句柄生命周期纪律（与 Manager.monitorDeviceData 相同）：
// 读循环是句柄的唯一拥有者，退出时由它自己关闭句柄。其它协程只能通过 stop
// 通道请求停止，绝不能直接 Close——读操作进行中被关闭会触发 hidapi 的
// use-after-free 崩溃（典型场景：休眠唤醒后句柄失效，读卡住时执行断开）。
type bs1HIDTransport struct {
	dev *hid.Device

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// newBS1HIDTransport 打开指定路径的 HID 接口并设为非阻塞读。
//
// 非阻塞是必须的：失效的 Windows HID 句柄可能让阻塞读永久不返回，
// 那样断连回调与自动重连都会被卡死。
func newBS1HIDTransport(path string) (*bs1HIDTransport, error) {
	dev, err := hid.OpenPath(path)
	if err != nil {
		return nil, fmt.Errorf("打开 BS1 HID 接口失败: %w", err)
	}
	if err := dev.SetNonblock(true); err != nil {
		_ = dev.Close()
		return nil, fmt.Errorf("设置 BS1 HID 非阻塞模式失败: %w", err)
	}
	return &bs1HIDTransport{
		dev:  dev,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}, nil
}

// Write 发送一帧协议数据（不等待应答）。
func (t *bs1HIDTransport) Write(frame []byte) error {
	report := buildBS1HIDReport(frame)
	if report == nil {
		return fmt.Errorf("帧长度 %d 超出 BS1 HID 报文容量", len(frame))
	}
	_, err := t.dev.Write(report)
	return err
}

// Run 是读循环，必须在独立协程里启动。
//
// 它独占句柄：无论因停止信号还是读错误退出，都由它自己关闭句柄。
// 返回 nil 表示收到停止信号（主动断开），非 nil 表示读失败（设备掉线）。
func (t *bs1HIDTransport) Run(onFrame func(raw []byte)) (err error) {
	defer close(t.done)
	defer func() {
		// 读循环内的 panic 不能带崩整个进程（休眠唤醒后蓝牙/HID 栈可能异常）。
		if r := recover(); r != nil {
			err = fmt.Errorf("BS1 HID 读循环 panic: %v", r)
		}
	}()
	defer func() {
		if closeErr := t.dev.Close(); closeErr != nil {
			// 关闭失败不影响返回值判定，只记录在 err 里（若尚未有错误）。
			if err == nil {
				err = fmt.Errorf("关闭 BS1 HID 句柄失败: %w", closeErr)
			}
		}
	}()

	buffer := make([]byte, bs1HIDReadBufferSize)
	for {
		select {
		case <-t.stop:
			return nil
		default:
		}

		n, readErr := t.dev.Read(buffer)
		if readErr != nil {
			if readErr == hid.ErrTimeout {
				time.Sleep(bs1HIDIdleSleep)
				continue
			}
			return fmt.Errorf("读取 BS1 HID 数据失败: %w", readErr)
		}
		if n <= 0 {
			time.Sleep(bs1HIDIdleSleep)
			continue
		}

		raw := make([]byte, n)
		copy(raw, buffer[:n])
		onFrame(raw)
	}
}

// Close 请求读循环停止，并等待它退出（句柄由读循环自己关闭）。
//
// 返回 false 表示超时未退出——此时句柄仍归读循环所有，不能强行关闭，
// 否则会触发 use-after-free。调用方应记录日志并放弃这次连接。
func (t *bs1HIDTransport) Close(timeout time.Duration) bool {
	t.stopOnce.Do(func() { close(t.stop) })
	select {
	case <-t.done:
		return true
	case <-time.After(timeout):
		return false
	}
}
