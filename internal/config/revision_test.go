package config

import (
	"testing"

	"github.com/TIANLI0/THRM/internal/types"
)

// 热路径按修订号缓存配置快照，任何改动配置内容的操作都必须推进修订号。
func TestRevisionTracksConfigChanges(t *testing.T) {
	isolateUserDirs(t, t.TempDir())
	m := NewManager("", testLogger{})
	m.Set(types.GetDefaultConfig(false))

	_, rev := m.GetWithRevision()
	if got := m.Revision(); got != rev {
		t.Fatalf("Revision() = %d, GetWithRevision = %d", got, rev)
	}

	// 首次保存写入了实际保存路径：配置内容变了，修订号必须推进。
	if err := m.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	afterFirstSave := m.Revision()
	if afterFirstSave == rev {
		t.Fatal("recording a new ConfigPath must bump the revision")
	}
	if cfg, got := m.GetWithRevision(); got != afterFirstSave || cfg.ConfigPath == "" {
		t.Fatalf("snapshot at revision %d has ConfigPath %q", got, cfg.ConfigPath)
	}

	// 路径不变的保存不改内容，修订号保持不变（避免无谓地让缓存失效）。
	if err := m.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if got := m.Revision(); got != afterFirstSave {
		t.Fatalf("saving to the same path changed the revision: %d -> %d", afterFirstSave, got)
	}

	m.Mutate(func(cfg *types.AppConfig) bool {
		cfg.AutoControl = !cfg.AutoControl
		return true
	})
	if m.Revision() == afterFirstSave {
		t.Fatal("Mutate must bump the revision")
	}
}
