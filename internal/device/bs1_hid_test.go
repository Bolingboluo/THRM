package device

import (
	"bytes"
	"testing"
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
