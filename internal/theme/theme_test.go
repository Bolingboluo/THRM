package theme

import (
	"embed"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

//go:embed testdata
var testEmbedFS embed.FS

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/install/themes", "/tmp/user/themes", testEmbedFS)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestNewManager_NilFS(t *testing.T) {
	m := NewManager("/tmp/install/themes", "/tmp/user/themes", nil)
	if m == nil {
		t.Fatal("NewManager with nil FS returned nil")
	}
}

func TestList_EmptyFS(t *testing.T) {
	m := NewManager("/tmp/install/themes", "/tmp/user/themes", testEmbedFS)
	metas := m.List()
	if len(metas) > 0 {
		t.Logf("List returned %d entries from empty testdata", len(metas))
	}
}

func TestReadCSS_NotFound(t *testing.T) {
	m := NewManager("/tmp/install/themes", "/tmp/user/themes", testEmbedFS)
	_, err := m.ReadCSS("nonexistent")
	if err == nil {
		t.Error("ReadCSS should fail for nonexistent theme")
	}
}

func TestResolveDir(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "install", "themes")
	userDir := filepath.Join(root, "user", "themes")
	installedThemeDir := filepath.Join(installDir, "sample")
	if err := os.MkdirAll(installedThemeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedThemeDir, manifestName), []byte(`{"id":"sample"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(installDir, userDir, fstest.MapFS{
		"sample/theme.json": &fstest.MapFile{Data: []byte(`{"id":"sample"}`)},
	})
	m.EnsureSeeded()
	if got := m.ResolveDir(); got != installDir {
		t.Fatalf("ResolveDir() = %q, want existing install directory %q", got, installDir)
	}
	if _, err := os.Stat(userDir); !os.IsNotExist(err) {
		t.Fatalf("ResolveDir created an empty user directory: %v", err)
	}
}

func TestSourceConstants(t *testing.T) {
	if SourceUser == "" || SourceInstall == "" || SourceBuiltin == "" {
		t.Error("Source constants should not be empty")
	}
}

func TestEnsureSeeded(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "install", "themes")
	userDir := filepath.Join(root, "user", "themes")
	builtin := fstest.MapFS{
		"sample/theme.json": &fstest.MapFile{Data: []byte(`{"id":"sample","name":"Sample","base":"dark"}`)},
		"sample/theme.css":  &fstest.MapFile{Data: []byte(":root {}")},
	}
	m := NewManager(installDir, userDir, builtin)
	m.EnsureSeeded()

	if _, err := os.Stat(filepath.Join(userDir, "sample", manifestName)); err != nil {
		t.Fatalf("builtin theme was not seeded into the user directory: %v", err)
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("theme seeding wrote to the installation directory: %v", err)
	}
}

func TestMetaFields(t *testing.T) {
	meta := Meta{
		ID:   "test",
		Name: "Test Theme",
	}
	if meta.ID == "" {
		t.Error("Meta.ID should not be empty after assignment")
	}
}

func writeTheme(t *testing.T, baseDir, folder, manifest, css string) {
	t.Helper()
	dir := filepath.Join(baseDir, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, styleName), []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 主题 id 来自清单而不是文件夹名：ReadCSS 曾直接拼 <dir>/<id>/theme.css，
// 文件夹改名后列表里有这个主题却读不到样式；复制出来还没改 id 的文件夹
// 则让列表展示副本的清单、样式却读自原文件夹。
func TestReadCSSResolvesFolderFromManifestID(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "install", "themes")
	userDir := filepath.Join(root, "user", "themes")
	writeTheme(t, userDir, "mytheme", `{"id":"cool","name":"Cool"}`, "cool-css")
	writeTheme(t, userDir, "thrm", `{"id":"thrm","name":"Orig"}`, "orig-css")
	writeTheme(t, userDir, "a-copy", `{"id":"thrm","name":"CopyA"}`, "copy-a-css")
	writeTheme(t, userDir, "thrm - Copy", `{"id":"thrm","name":"CopyB"}`, "copy-b-css")
	writeTheme(t, installDir, "thrm", `{"id":"thrm","name":"Installed"}`, "install-css")

	m := NewManager(installDir, userDir, fstest.MapFS{
		"thrm/theme.json": &fstest.MapFile{Data: []byte(`{"id":"thrm","name":"Builtin"}`)},
		"thrm/theme.css":  &fstest.MapFile{Data: []byte("builtin-css")},
	})

	names := map[string]string{}
	for _, meta := range m.List() {
		names[meta.ID] = meta.Name
	}
	if names["cool"] != "Cool" || names["thrm"] != "Orig" {
		t.Fatalf("List() = %v, want cool=Cool thrm=Orig", names)
	}
	for id, want := range map[string]string{"cool": "cool-css", "thrm": "orig-css"} {
		got, err := m.ReadCSS(id)
		if err != nil || got != want {
			t.Errorf("ReadCSS(%q) = %q, %v; want %q", id, got, err, want)
		}
	}
}

func TestReadCSSFallsBackBySourcePriority(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "install", "themes")
	userDir := filepath.Join(root, "user", "themes")
	writeTheme(t, installDir, "inst", `{"id":"sample","name":"Installed"}`, "install-css")

	m := NewManager(installDir, userDir, fstest.MapFS{
		"sample/theme.json": &fstest.MapFile{Data: []byte(`{"id":"sample"}`)},
		"sample/theme.css":  &fstest.MapFile{Data: []byte("builtin-css")},
		"other/theme.json":  &fstest.MapFile{Data: []byte(`{"id":"other"}`)},
		"other/theme.css":   &fstest.MapFile{Data: []byte("other-css")},
	})
	if got, err := m.ReadCSS("sample"); err != nil || got != "install-css" {
		t.Errorf("ReadCSS(sample) = %q, %v; want install-css", got, err)
	}
	if got, err := m.ReadCSS("other"); err != nil || got != "other-css" {
		t.Errorf("ReadCSS(other) = %q, %v; want other-css", got, err)
	}
}
