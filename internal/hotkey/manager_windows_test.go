//go:build windows

package hotkey

import (
	"strings"
	"testing"
	"time"
)

func TestParseShortcutVirtualKeys(t *testing.T) {
	tests := []struct {
		in   string
		mods Modifier
		key  Key
	}{
		{"Ctrl+Alt+Shift+M", ModCtrl | ModAlt | ModShift, 'M'},
		{" ctrl + win + 7 ", ModCtrl | ModWin, '7'},
		{"Control+F1", ModCtrl, 0x70},
		{"Alt+F12", ModAlt, 0x7B},
		{"Ctrl+Ctrl+A", ModCtrl, 'A'},
	}
	for _, tt := range tests {
		mods, key, err := ParseShortcut(tt.in)
		if err != nil {
			t.Fatalf("ParseShortcut(%q) error: %v", tt.in, err)
		}
		var mask Modifier
		for _, m := range mods {
			mask |= m
		}
		if mask != tt.mods || key != tt.key {
			t.Fatalf("ParseShortcut(%q) = %#x/%#x, want %#x/%#x", tt.in, mask, key, tt.mods, tt.key)
		}
	}

	for _, bad := range []string{"", "M", "Ctrl", "Ctrl+A+B", "Ctrl+F13", "Ctrl+F0", "Ctrl+Esc"} {
		if _, _, err := ParseShortcut(bad); err == nil {
			t.Fatalf("ParseShortcut(%q) 应当报错", bad)
		}
	}
}

// 用一个几乎不会被占用的组合，避免和用户机器上的快捷键冲突。
const testShortcut = "Ctrl+Alt+Shift+Win+F9"

// simulatePress 往消息循环投一条 WM_HOTKEY，走的是与真实按键完全相同的分派路径。
func simulatePress(t *testing.T, m *Manager, action Action) {
	t.Helper()
	m.mutex.Lock()
	b, ok := m.bindings[action]
	loop := m.loop
	m.mutex.Unlock()
	if !ok || loop == nil {
		t.Fatalf("%s 没有注册", action)
	}
	if r, _, err := procPostThreadMessage.Call(uintptr(loop.threadID), wmHotkey, uintptr(b.id), 0); r == 0 {
		t.Fatalf("PostThreadMessage: %v", err)
	}
}

func TestManagerDispatchesHotkeyAndRebindsFromCallback(t *testing.T) {
	type firing struct {
		action   Action
		shortcut string
	}
	fired := make(chan firing, 4)
	var m *Manager
	m = NewManager(nil, func(action Action, shortcut string) {
		// 动作回调里改绑定（例如切换模式后重新读配置）不能死锁。
		if err := m.UpdateBindings("", testShortcut, ""); err != nil {
			t.Errorf("回调内 UpdateBindings: %v", err)
		}
		fired <- firing{action, shortcut}
	})
	defer m.Stop()

	if err := m.UpdateBindings(testShortcut, "", ""); err != nil {
		t.Skipf("测试快捷键被系统占用，跳过: %v", err)
	}
	simulatePress(t, m, ActionToggleManualGear)

	select {
	case got := <-fired:
		if got.action != ActionToggleManualGear || got.shortcut != testShortcut {
			t.Fatalf("onAction = %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WM_HOTKEY 没有触发动作（可能死锁）")
	}

	// 回调已把快捷键改绑到另一个动作，旧 id 已注销。
	simulatePress(t, m, ActionToggleAutoMode)
	select {
	case got := <-fired:
		if got.action != ActionToggleAutoMode {
			t.Fatalf("改绑后触发的动作 = %s", got.action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("改绑后的快捷键没有触发")
	}
}

func TestManagerReportsConflictAndStopsLoop(t *testing.T) {
	first := NewManager(nil, nil)
	defer first.Stop()
	if err := first.UpdateBindings(testShortcut, "", ""); err != nil {
		t.Skipf("测试快捷键被系统占用，跳过: %v", err)
	}

	// 同一组合已被注册（无论是本进程还是别的程序），RegisterHotKey 必须报错而不是静默成功。
	second := NewManager(nil, nil)
	err := second.UpdateBindings(testShortcut, "", "")
	if err == nil || !strings.Contains(err.Error(), "注册失败") {
		t.Fatalf("重复注册应当报错，得到 %v", err)
	}
	second.Stop()

	loop := first.loop
	first.Stop()
	select {
	case <-loop.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 之后消息循环线程没有退出")
	}
	if err := first.UpdateBindings(testShortcut, "", ""); err == nil {
		t.Fatal("Stop 之后不应还能注册")
	}

	// 注销后组合键应当已归还系统，可以被重新注册。
	third := NewManager(nil, nil)
	defer third.Stop()
	if err := third.UpdateBindings(testShortcut, "", ""); err != nil {
		t.Fatalf("Stop 后组合键没有释放: %v", err)
	}
}

func TestManagerWithoutShortcutsStartsNoThread(t *testing.T) {
	m := NewManager(nil, nil)
	defer m.Stop()
	if err := m.UpdateBindings("", " ", ""); err != nil {
		t.Fatal(err)
	}
	if m.loop != nil {
		t.Fatal("没有配置快捷键时不应启动消息循环线程")
	}
}
