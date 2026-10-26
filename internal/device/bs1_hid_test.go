package device

import (
	"bytes"
	"testing"

	"github.com/TIANLI0/THRM/internal/deviceproto"
)

// buildBS1HIDReport 的期望布局来自真机验证的 BS1 报文：
// [0]=0x01 Report ID，帧去掉校验字节，原校验位置补 0，校验和挪到第 31 字节。
func TestBuildBS1HIDReport(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
		// wantHead 是报文从第 0 字节开始应当匹配的内容
		wantHead []byte
		// wantZeroAt 是必须为 0x00 的字节位置
		wantZeroAt int
		// wantChecksum 是必须落在第 31 字节的值
		wantChecksum byte
	}{
		{
			name:         "cmd 0x05 握手帧",
			frame:        []byte{0x5A, 0xA5, 0x05, 0x02, 0x07},
			wantHead:     []byte{0x01, 0x5A, 0xA5, 0x05, 0x02},
			wantZeroAt:   5,
			wantChecksum: 0x07,
		},
		{
			name:         "cmd 0x21 设 2100 RPM",
			frame:        []byte{0x5A, 0xA5, 0x21, 0x04, 0x34, 0x08, 0x61},
			wantHead:     []byte{0x01, 0x5A, 0xA5, 0x21, 0x04, 0x34, 0x08},
			wantZeroAt:   7,
			wantChecksum: 0x61,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := buildBS1HIDReport(tt.frame)
			if report == nil {
				t.Fatalf("buildBS1HIDReport 返回 nil")
			}
			if len(report) != bs1HIDReportLen {
				t.Fatalf("报文长度 = %d，期望 %d", len(report), bs1HIDReportLen)
			}
			if got := report[:len(tt.wantHead)]; !bytes.Equal(got, tt.wantHead) {
				t.Errorf("报文头部 = % X，期望 % X", got, tt.wantHead)
			}
			if report[tt.wantZeroAt] != 0x00 {
				t.Errorf("报文[%d] = 0x%02X，期望 0x00（原校验位置应清零）", tt.wantZeroAt, report[tt.wantZeroAt])
			}
			if report[bs1HIDReportLen-1] != tt.wantChecksum {
				t.Errorf("报文[%d] = 0x%02X，期望校验和 0x%02X", bs1HIDReportLen-1, report[bs1HIDReportLen-1], tt.wantChecksum)
			}
			// 除头部、补零位与末字节校验和外，其余必须是 0。
			for i := len(tt.wantHead); i < bs1HIDReportLen-1; i++ {
				if i == tt.wantZeroAt {
					continue
				}
				if report[i] != 0x00 {
					t.Errorf("报文[%d] = 0x%02X，期望填充 0x00", i, report[i])
				}
			}
		})
	}
}

// 帧太长或太短时必须拒绝，不能写出越界报文。
func TestBuildBS1HIDReportRejectsInvalidLength(t *testing.T) {
	cases := map[string][]byte{
		"空帧":       {},
		"单字节":      {0x5A},
		"刚好 32 字节": make([]byte, bs1HIDReportLen),
		"超过 32 字节": make([]byte, bs1HIDReportLen+8),
	}
	for name, frame := range cases {
		if got := buildBS1HIDReport(frame); got != nil {
			t.Errorf("%s（长度 %d）：期望返回 nil，实际返回 %d 字节", name, len(frame), len(got))
		}
	}
}

// 帧内容不得被改写：封装只是搬位置，不能动协议字节。
func TestBuildBS1HIDReportDoesNotMutateInput(t *testing.T) {
	frame := []byte{0x5A, 0xA5, 0x08, 0x03, 0x02, 0x0D}
	original := append([]byte(nil), frame...)

	_ = buildBS1HIDReport(frame)

	if !bytes.Equal(frame, original) {
		t.Errorf("输入帧被改写：% X → % X", original, frame)
	}
}

// makeBS1HIDInputReport 把一帧协议数据包成设备发上来的 HID 报文
// （首字节是 Input Report ID 0x03），用于喂给读循环的回调。
func makeBS1HIDInputReport(frame []byte) []byte {
	report := make([]byte, bs1HIDReadBufferSize)
	report[0] = bs1HIDInputReportID
	copy(report[1:], frame)
	return report[:1+len(frame)]
}

// HID 路径的 0xEF 状态帧必须解析成 FanData。
//
// 字节取自真机抓包：模式 0x02（挡位模式）、实测与目标转速都是 2100 RPM。
// 这验证了 BS1 走 HID 时读循环的解析与 BLE 通知走的是同一套逻辑。
func TestHandleHIDFrameParsesStatusNotification(t *testing.T) {
	b := NewBLEManager(nil)

	frame := deviceproto.BuildFrame(deviceproto.CmdStatusNotify,
		0x32,       // 挡位/模式字节
		0x02,       // 工作模式
		0x01,       // 保留
		0x34, 0x08, // 实测 2100 RPM（小端）
		0x34, 0x08, // 目标 2100 RPM（小端）
		0x4D, 0x16, // 帧序号
	)
	b.handleHIDFrame(makeBS1HIDInputReport(frame))

	data := b.GetCurrentFanData()
	if data == nil {
		t.Fatalf("0xEF 状态帧未被解析成 FanData")
	}
	if data.CurrentRPM != 2100 {
		t.Errorf("CurrentRPM = %d，期望 2100", data.CurrentRPM)
	}
	if data.TargetRPM != 2100 {
		t.Errorf("TargetRPM = %d，期望 2100", data.TargetRPM)
	}
	if data.CurrentMode != 0x02 {
		t.Errorf("CurrentMode = 0x%02X，期望 0x02", data.CurrentMode)
	}
}

// 命令应答必须派发给对应的等待者——这是 sendBLECommandAndWait 能工作的前提，
// 换传输后同样成立。
func TestHandleHIDFrameDeliversCommandResponse(t *testing.T) {
	b := NewBLEManager(nil)

	waiter := b.responses.register(deviceproto.CmdQueryGearRPMTable)

	// 0x27 应答：四个硬件挡位槽的转速，各 2 字节小端。
	frame := deviceproto.BuildFrame(deviceproto.CmdQueryGearRPMTable,
		0x14, 0x05, // 1300
		0xA4, 0x08, // 2212
		0xF0, 0x0A, // 2800
		0xDC, 0x0D, // 3548
	)
	b.handleHIDFrame(makeBS1HIDInputReport(frame))

	select {
	case got := <-waiter.result:
		if got.Command != deviceproto.CmdQueryGearRPMTable {
			t.Errorf("派发的命令 = 0x%02X，期望 0x%02X", got.Command, deviceproto.CmdQueryGearRPMTable)
		}
		if !got.ChecksumOK {
			t.Errorf("派发的帧校验和不通过")
		}
	default:
		t.Fatalf("应答未派发给等待者")
	}
}

// 校验和错误的帧必须被丢弃，不能污染状态或误派发。
func TestHandleHIDFrameIgnoresBadChecksum(t *testing.T) {
	b := NewBLEManager(nil)

	frame := deviceproto.BuildFrame(deviceproto.CmdStatusNotify,
		0x32, 0x02, 0x01, 0x34, 0x08, 0x34, 0x08, 0x4D, 0x16)
	frame[len(frame)-1] ^= 0xFF // 破坏校验和

	waiter := b.responses.register(deviceproto.CmdStatusNotify)
	b.handleHIDFrame(makeBS1HIDInputReport(frame))

	if data := b.GetCurrentFanData(); data != nil {
		t.Errorf("校验和错误的帧污染了状态：CurrentRPM = %d", data.CurrentRPM)
	}
	select {
	case <-waiter.result:
		t.Errorf("校验和错误的帧被误派发")
	default:
	}
}

// 非协议数据（比如设备刚上电时的杂散报文）不能让读循环崩掉。
func TestHandleHIDFrameToleratesNonProtocolData(t *testing.T) {
	b := NewBLEManager(nil)

	b.handleHIDFrame([]byte{0x03, 0xFF, 0x00, 0x11})

	if data := b.GetCurrentFanData(); data != nil {
		t.Errorf("非协议数据产生了 FanData")
	}
}

// "设备已枚举但不响应"的结论只对当前 USB 状态有效。
//
// 设备被拔下改接充电头后必须能回退到 BLE；若上一次的结论残留下来，
// connect() 会一直跳过 BLE 扫描，蓝牙就再也连不上了。
func TestConnectHIDResetsUnresponsiveFlagOnEachAttempt(t *testing.T) {
	original := enumerateBS1HID
	enumerateBS1HID = func() (string, string, bool) { return "", "", false }
	t.Cleanup(func() { enumerateBS1HID = original })

	b := NewBLEManager(nil)
	b.hidUnresponsive = true

	if ok, _ := b.ConnectHID(); ok {
		t.Fatal("枚举不到设备时 ConnectHID 不应返回成功")
	}
	if b.HIDUnresponsive() {
		t.Fatal("本次尝试没有设备，上一次的「无响应」结论必须被清掉")
	}
}

// 读循环关掉句柄后的写入必须直接报错，不能落到已释放的原生句柄上。
func TestBS1HIDTransportWriteAfterCloseFails(t *testing.T) {
	transport := &bs1HIDTransport{closed: true}

	frame := deviceproto.BuildFrame(deviceproto.CmdRGBStatus)
	if err := transport.Write(frame); err == nil {
		t.Fatal("句柄关闭后 Write 应返回错误")
	}
}

// 旧读循环晚退出时不能拆掉已经换上的新连接。
func TestHandleHIDTransportLostIgnoresStaleTransport(t *testing.T) {
	b := NewBLEManager(nil)
	disconnects := 0
	b.SetCallbacks(nil, func() { disconnects++ })

	current := &bs1HIDTransport{stop: make(chan struct{})}
	stale := &bs1HIDTransport{stop: make(chan struct{})}
	b.hidTransport = current
	b.isConnected = true

	b.handleHIDTransportLost(stale)
	if !b.IsConnected() || disconnects != 0 {
		t.Fatalf("旧传输退出拆掉了当前连接：connected=%v, 回调 %d 次", b.IsConnected(), disconnects)
	}

	b.handleHIDTransportLost(current)
	if b.IsConnected() || disconnects != 1 {
		t.Fatalf("当前传输退出后应断开并回调一次：connected=%v, 回调 %d 次", b.IsConnected(), disconnects)
	}
}
