package deviceproto

import (
	"bytes"
	"testing"
)

func TestChecksum(t *testing.T) {
	tests := []struct {
		name    string
		cmd     byte
		payload []byte
		want    byte
	}{
		{"no payload", 0x08, []byte{}, byte((0x08 + 2) & 0xFF)},
		{"single byte", 0x08, []byte{0x01}, byte((0x08 + 3 + 0x01) & 0xFF)},
		{"known vector set gear", 0x08, []byte{0x01}, byte((0x08 + 3 + 0x01) & 0xFF)},
		{"multi byte payload", 0x26, []byte{0x00, 0x00, 0xD0, 0x07}, byte((0x26 + 6 + 0xD0 + 0x07) & 0xFF)},
		{"large payload", 0x01, []byte{0xFF, 0xFF, 0xFF}, byte((0x01 + 5 + 0xFF*3) & 0xFF)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Checksum(tt.cmd, tt.payload...)
			if got != tt.want {
				t.Errorf("Checksum(0x%02X, %v) = 0x%02X, want 0x%02X", tt.cmd, tt.payload, got, tt.want)
			}
		})
	}
}

func TestBuildFrame(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	if len(frame) < 5 {
		t.Fatalf("frame too short: %d bytes", len(frame))
	}
	if frame[0] != Magic0 {
		t.Errorf("Magic0 = 0x%02X, want 0x%02X", frame[0], Magic0)
	}
	if frame[1] != Magic1 {
		t.Errorf("Magic1 = 0x%02X, want 0x%02X", frame[1], Magic1)
	}
	if frame[2] != 0x08 {
		t.Errorf("command = 0x%02X, want 0x08", frame[2])
	}
	if frame[3] != 3 {
		t.Errorf("length = %d, want 3", frame[3])
	}
	if frame[4] != 0x01 {
		t.Errorf("payload[0] = 0x%02X, want 0x01", frame[4])
	}
	expectedCS := Checksum(0x08, 0x01)
	if frame[5] != expectedCS {
		t.Errorf("checksum = 0x%02X, want 0x%02X", frame[5], expectedCS)
	}
}

func TestBuildFrame_EmptyPayload(t *testing.T) {
	frame := BuildFrame(0x01)
	if len(frame) != 5 {
		t.Errorf("empty payload frame len = %d, want 5", len(frame))
	}
	if frame[3] != 2 {
		t.Errorf("length = %d, want 2 (cmd + zero payload)", frame[3])
	}
}

func TestParseFrame_WithoutReportID(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	parsed, ok := ParseFrame(frame)
	if !ok {
		t.Fatal("ParseFrame failed on valid frame")
	}
	if parsed.Command != 0x08 {
		t.Errorf("Command = 0x%02X, want 0x08", parsed.Command)
	}
	if parsed.ReportID != 0 {
		t.Errorf("ReportID = 0x%02X, want 0", parsed.ReportID)
	}
	if !parsed.ChecksumOK {
		t.Error("ChecksumOK should be true")
	}
	if len(parsed.Payload) != 1 || parsed.Payload[0] != 0x01 {
		t.Errorf("Payload = %v, want [0x01]", parsed.Payload)
	}
}

func TestParseFrame_WithReportID(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	report := BuildReport(frame, 0)
	parsed, ok := ParseFrame(report)
	if !ok {
		t.Fatal("ParseFrame failed on valid report")
	}
	if parsed.ReportID != ReportID {
		t.Errorf("ReportID = 0x%02X, want 0x%02X", parsed.ReportID, ReportID)
	}
	if parsed.Command != 0x08 {
		t.Errorf("Command = 0x%02X, want 0x08", parsed.Command)
	}
	if !parsed.ChecksumOK {
		t.Error("ChecksumOK should be true")
	}
}

func TestParseFrame_TooShort(t *testing.T) {
	_, ok := ParseFrame([]byte{0x5A, 0xA5, 0x01})
	if ok {
		t.Error("ParseFrame should fail on short data")
	}
}

func TestParseFrame_BadMagic(t *testing.T) {
	_, ok := ParseFrame([]byte{0x00, 0x00, 0x01, 0x02, 0x00})
	if ok {
		t.Error("ParseFrame should fail on bad magic")
	}
}

func TestParseFrame_BadChecksum(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	frame[5] ^= 0xFF
	parsed, ok := ParseFrame(frame)
	if !ok {
		t.Fatal("ParseFrame should succeed even with bad checksum (returns ChecksumOK=false)")
	}
	if parsed.ChecksumOK {
		t.Error("ChecksumOK should be false for corrupted frame")
	}
}

func TestBuildReport(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	report := BuildReport(frame, 0)
	if report[0] != ReportID {
		t.Errorf("report[0] = 0x%02X, want 0x%02X", report[0], ReportID)
	}
	for i := range len(frame) {
		if report[1+i] != frame[i] {
			t.Errorf("report[%d] = 0x%02X, want 0x%02X", 1+i, report[1+i], frame[i])
		}
	}
}

func TestBuildReport_SpecifiedLen(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	report := BuildReport(frame, 30)
	if len(report) != 30 {
		t.Errorf("len = %d, want 30", len(report))
	}
	if report[0] != ReportID {
		t.Errorf("report[0] = 0x%02X", report[0])
	}
}

func TestRoundTrip(t *testing.T) {
	original := BuildFrame(0x26, []byte{0x00, 0x00, 0xD0, 0x07}...)
	parsed, ok := ParseFrame(original)
	if !ok {
		t.Fatal("round-trip parse failed")
	}
	rebuilt := BuildFrame(parsed.Command, parsed.Payload...)
	if len(rebuilt) != len(original) {
		t.Fatalf("round-trip length mismatch: %d vs %d", len(rebuilt), len(original))
	}
	for i := range original {
		if rebuilt[i] != original[i] {
			t.Errorf("byte[%d] = 0x%02X, want 0x%02X", i, rebuilt[i], original[i])
		}
	}
}

func TestHex(t *testing.T) {
	tests := []struct {
		input []byte
		want  string
	}{
		{[]byte{0x5A, 0xA5}, "5A A5"},
		{[]byte{0x5A, 0xA5, 0x08, 0x03, 0x01, 0x0C}, "5A A5 08 03 01 0C"},
		{[]byte{}, ""},
		{[]byte{0x00}, "00"},
	}
	for _, tt := range tests {
		got := Hex(tt.input)
		if got != tt.want {
			t.Errorf("Hex(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseHex(t *testing.T) {
	tests := []struct {
		input string
		want  []byte
	}{
		{"5AA5", []byte{0x5A, 0xA5}},
		{"5AA50803010C", []byte{0x5A, 0xA5, 0x08, 0x03, 0x01, 0x0C}},
		{"5A A5 08 03 01 0C", []byte{0x5A, 0xA5, 0x08, 0x03, 0x01, 0x0C}},
		{"0x5A 0xA5", []byte{0x5A, 0xA5}},
		{"5AA5\n", []byte{0x5A, 0xA5}},
		{"5A,A5", []byte{0x5A, 0xA5}},
	}
	for _, tt := range tests {
		got, err := ParseHex(tt.input)
		if err != nil {
			t.Errorf("ParseHex(%q) error: %v", tt.input, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("ParseHex(%q) len = %d, want %d", tt.input, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ParseHex(%q)[%d] = 0x%02X, want 0x%02X", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestParseHex_Invalid(t *testing.T) {
	_, err := ParseHex("GG")
	if err == nil {
		t.Error("ParseHex should fail on non-hex input")
	}
}

// 单数字字段必须只补它自己；整体补 0 会让后面每个字节错开半个字节，
// "5A A5 0C 03 1" 曾被解析成 05 AA 50 C0 31，发出去是另一条命令。
func TestParseHex_SingleDigitFieldPadsOnlyItself(t *testing.T) {
	tests := []struct {
		input string
		want  []byte
	}{
		{"0C 1", []byte{0x0C, 0x01}},
		{"5A A5 0C 03 1", []byte{0x5A, 0xA5, 0x0C, 0x03, 0x01}},
		{"8 1", []byte{0x08, 0x01}},
		{"0x8,0x1", []byte{0x08, 0x01}},
		{"1", []byte{0x01}},
	}
	for _, tt := range tests {
		got, err := ParseHex(tt.input)
		if err != nil {
			t.Errorf("ParseHex(%q) error: %v", tt.input, err)
			continue
		}
		if !bytes.Equal(got, tt.want) {
			t.Errorf("ParseHex(%q) = % X, want % X", tt.input, got, tt.want)
		}
	}
}

// 超过一位的奇数位字段分不清字节边界，猜错就是另一条命令，必须拒绝。
func TestParseHex_RejectsAmbiguousOddField(t *testing.T) {
	for _, input := range []string{"5AA50C031", "5A A5 0C3 01", "123"} {
		if got, err := ParseHex(input); err == nil {
			t.Errorf("ParseHex(%q) = % X, want error", input, got)
		}
	}
}

// 固件的 0x08 分支把 payload 原样写进当前挡位字段，调试控制台必须挡住 1..4 以外的值。
func TestValidateOutboundFrame_FixedGearRange(t *testing.T) {
	for gear := 1; gear <= 4; gear++ {
		if err := ValidateOutboundFrame(BuildFrame(CmdSetFixedGear, byte(gear))); err != nil {
			t.Errorf("0x08 gear %d rejected: %v", gear, err)
		}
	}
	for _, frame := range [][]byte{
		BuildFrame(CmdSetFixedGear),
		BuildFrame(CmdSetFixedGear, 0x00),
		BuildFrame(CmdSetFixedGear, 0x05),
		BuildFrame(CmdSetFixedGear, 0xFF),
	} {
		if err := ValidateOutboundFrame(frame); err == nil {
			t.Errorf("ValidateOutboundFrame(% X) should fail", frame)
		}
	}
	if _, err := NormalizeDebugInput("08 05"); err == nil {
		t.Error(`NormalizeDebugInput("08 05") should fail`)
	}
}

func TestNormalizeDebugInput_WithReportID(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	report := BuildReport(frame, 0)
	result, err := NormalizeDebugInput(Hex(report))
	if err != nil {
		t.Fatalf("NormalizeDebugInput error: %v", err)
	}
	if len(result) != len(frame) {
		t.Errorf("result len = %d, want %d", len(result), len(frame))
	}
}

func TestNormalizeDebugInput_WithoutReportID(t *testing.T) {
	frame := BuildFrame(0x08, 0x01)
	result, err := NormalizeDebugInput(Hex(frame))
	if err != nil {
		t.Fatalf("NormalizeDebugInput error: %v", err)
	}
	if len(result) != len(frame) {
		t.Errorf("result len = %d, want %d", len(result), len(frame))
	}
}

func TestNormalizeDebugInput_RawCmd(t *testing.T) {
	result, err := NormalizeDebugInput("08 01")
	if err != nil {
		t.Fatalf("NormalizeDebugInput error: %v", err)
	}
	if len(result) < 5 {
		t.Errorf("result too short: %d bytes", len(result))
	}
}

func TestNormalizeDebugInput_Empty(t *testing.T) {
	_, err := NormalizeDebugInput("")
	if err == nil {
		t.Error("NormalizeDebugInput should fail on empty input")
	}
}

func TestNormalizeDebugInput_InvalidFrame(t *testing.T) {
	_, err := NormalizeDebugInput("5AA501")
	if err == nil {
		t.Error("NormalizeDebugInput should fail on invalid frame")
	}
}
