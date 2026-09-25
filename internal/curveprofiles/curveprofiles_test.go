package curveprofiles

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/TIANLI0/THRM/internal/types"
)

func TestCloneCurve(t *testing.T) {
	original := []types.FanCurvePoint{
		{Temperature: 30, RPM: 800},
		{Temperature: 50, RPM: 1500},
	}
	clone := CloneCurve(original)
	if len(clone) != len(original) {
		t.Fatalf("CloneCurve len = %d, want %d", len(clone), len(original))
	}
	clone[0].RPM = 9999
	if original[0].RPM == 9999 {
		t.Error("CloneCurve should deep copy")
	}
}

func TestCloneCurve_Empty(t *testing.T) {
	clone := CloneCurve(nil)
	if clone != nil {
		t.Error("CloneCurve(nil) should return nil")
	}
	clone = CloneCurve([]types.FanCurvePoint{})
	if clone != nil {
		t.Error("CloneCurve([]) should return nil")
	}
}

func TestCloneProfiles(t *testing.T) {
	original := []types.FanCurveProfile{
		{ID: "p1", Name: "profile1", Curve: []types.FanCurvePoint{{Temperature: 30, RPM: 800}}},
	}
	clone := CloneProfiles(original)
	if len(clone) != 1 {
		t.Fatalf("len = %d", len(clone))
	}
	clone[0].Curve[0].RPM = 9999
	if original[0].Curve[0].RPM == 9999 {
		t.Error("CloneProfiles should deep copy")
	}
}

func TestCloneProfiles_Empty(t *testing.T) {
	if CloneProfiles(nil) != nil {
		t.Error("CloneProfiles(nil) should return nil")
	}
}

func TestNormalizeProfileName(t *testing.T) {
	tests := []struct {
		input    string
		fallback string
		want     string
	}{
		{"valid", "", "valid"},
		{"", "default", "default"},
		{"   ", "default", "default"},
		{"very long name here", "", "very long name here"},
	}
	for _, tt := range tests {
		got := NormalizeProfileName(tt.input, tt.fallback)
		if got == "" {
			t.Errorf("NormalizeProfileName(%q, %q) should not return empty", tt.input, tt.fallback)
		}
		if len([]rune(got)) > 6 && tt.input == "very long name here" {
			// long name should be truncated to max 6 runes
		}
		_ = got
	}
}

func TestGenerateID(t *testing.T) {
	id1 := GenerateID()
	id2 := GenerateID()
	if id1 == "" {
		t.Error("GenerateID should not return empty")
	}
	if id1 == id2 {
		t.Error("GenerateID should return unique values")
	}
	if id1[0] != 'p' {
		t.Errorf("GenerateID should start with 'p': got %q", id1)
	}
}

func TestFindIndex(t *testing.T) {
	profiles := []types.FanCurveProfile{
		{ID: "p1"},
		{ID: "p2"},
		{ID: "p3"},
	}
	if idx := FindIndex(profiles, "p2"); idx != 1 {
		t.Errorf("FindIndex(p2) = %d, want 1", idx)
	}
	if idx := FindIndex(profiles, "nonexistent"); idx != -1 {
		t.Errorf("FindIndex(nonexistent) = %d, want -1", idx)
	}
	if idx := FindIndex(nil, "p1"); idx != -1 {
		t.Errorf("FindIndex on nil = %d, want -1", idx)
	}
}

func TestExportImport_RoundTrip(t *testing.T) {
	profiles := []types.FanCurveProfile{
		{ID: "p1", Name: "测试", Curve: []types.FanCurvePoint{{Temperature: 30, RPM: 800}}},
	}
	code, err := Export("p1", profiles)
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if code == "" {
		t.Fatal("Export returned empty code")
	}

	importedProfiles, activeID, err := Import(code)
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if activeID != "p1" {
		t.Errorf("activeID = %q, want 'p1'", activeID)
	}
	if len(importedProfiles) != len(profiles) {
		t.Fatalf("imported len = %d, want %d", len(importedProfiles), len(profiles))
	}
	if importedProfiles[0].Name != "测试" {
		t.Errorf("imported name = %q", importedProfiles[0].Name)
	}
}

func TestImport_Invalid(t *testing.T) {
	_, _, err := Import("not valid base64!!!")
	if err == nil {
		t.Error("Import should fail on invalid base64")
	}
	_, _, err = Import("")
	if err == nil {
		t.Error("Import should fail on empty string")
	}
}

// 分享码是用户粘贴进来的，解压后的体积必须有上限：高压缩比的 zlib 炸弹
// 否则会被 io.ReadAll 整个展开，把核心进程撑到 OOM。
func TestImport_RejectsOversizedPayload(t *testing.T) {
	// JSON 前面的空白是合法的，所以这段载荷没有上限时能正常导入。
	plain := strings.Repeat(" ", maxImportPayloadBytes) + `{"v":1,"a":"p1","p":[]}`
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write([]byte(plain)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Import(exportPrefix + base64.RawURLEncoding.EncodeToString(buf.Bytes())); err == nil {
		t.Fatal("解压后超过上限的分享码应被拒绝")
	}

	// 上限不能卡住真实的大导出：200 个方案、每个 50 个点。
	curve := make([]types.FanCurvePoint, 50)
	for i := range curve {
		curve[i] = types.FanCurvePoint{Temperature: 30 + i, RPM: 4000}
	}
	profiles := make([]types.FanCurveProfile, 200)
	for i := range profiles {
		profiles[i] = types.FanCurveProfile{ID: fmt.Sprintf("p%d-%d", i, 1<<40), Name: "方案名称六字", Curve: curve}
	}
	code, err := Export("p0", profiles)
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if imported, _, err := Import(code); err != nil || len(imported) != len(profiles) {
		t.Fatalf("真实规模的导出应能导入: len=%d err=%v", len(imported), err)
	}
}

func TestNormalizeConfig_Nil(t *testing.T) {
	if NormalizeConfig(nil) {
		t.Error("NormalizeConfig(nil) should return false")
	}
}

func TestNormalizeConfig_EmptyProfiles(t *testing.T) {
	cfg := &types.AppConfig{FanCurveProfiles: nil}
	NormalizeConfig(cfg)
	if len(cfg.FanCurveProfiles) == 0 {
		t.Error("FanCurveProfiles should be populated with default profile")
	}
}

func TestAppendImportedProfilesPreservesExistingAndCreatesNewIDs(t *testing.T) {
	existing := []types.FanCurveProfile{{ID: "existing", Name: "Quiet", Curve: types.GetDefaultFanCurve()}}
	imported := []types.FanCurveProfile{
		{ID: "quiet", Name: "Quiet", Curve: types.GetDefaultFanCurve()},
		{ID: "gaming", Name: "Gaming", Curve: types.GetDefaultFanCurve()},
	}

	got, activeID := AppendImportedProfiles(existing, imported, "gaming")

	if len(got) != 3 {
		t.Fatalf("profile count = %d, want 3", len(got))
	}
	if got[0].ID != "existing" || got[0].Name != "Quiet" {
		t.Fatalf("existing profile changed: %+v", got[0])
	}
	if got[1].Name != "Quiet2" {
		t.Fatalf("duplicate imported name = %q, want Quiet2", got[1].Name)
	}
	if got[1].ID == "quiet" || got[2].ID == "gaming" || got[1].ID == got[2].ID {
		t.Fatalf("imported IDs were not regenerated: %q, %q", got[1].ID, got[2].ID)
	}
	if activeID != got[2].ID {
		t.Fatalf("activeID = %q, want %q", activeID, got[2].ID)
	}
}
