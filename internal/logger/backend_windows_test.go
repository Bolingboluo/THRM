//go:build windows

package logger

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestIsInteractiveConsoleRejectsFiles(t *testing.T) {
	if isInteractiveConsole(nil) {
		t.Fatal("nil file must not be treated as a console")
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "out.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isInteractiveConsole(f) {
		t.Fatal("a regular file must not be treated as a console")
	}
}

// 核心进程把 stdout 指向 fatal_*.log：日志只能进轮转的 app 日志，不能再带着颜色码
// 往 fatal 日志里写一份。
func TestPersistentBackendSkipsConsoleWhenStdoutIsAFile(t *testing.T) {
	dir := t.TempDir()
	fatal, err := os.Create(filepath.Join(dir, "fatal.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer fatal.Close()

	originalStdout := os.Stdout
	os.Stdout = fatal
	defer func() { os.Stdout = originalStdout }()

	atom := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	backend, err := newPlatformBackend(atom, dir, CoreIdentifier, true)
	if err != nil {
		t.Fatalf("newPlatformBackend: %v", err)
	}
	defer func() {
		for _, closer := range backend.closers {
			_ = closer.Close()
		}
	}()
	if len(backend.cores) != 2 {
		t.Fatalf("expected only the app/debug file cores, got %d cores", len(backend.cores))
	}

	logger := zap.New(zapcore.NewTee(backend.cores...))
	logger.Info("hello")
	_ = logger.Sync()

	info, err := fatal.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("log lines leaked into the fatal log (%d bytes)", info.Size())
	}
}
