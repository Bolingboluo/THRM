//go:build windows

package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sys/windows"
	"gopkg.in/natefinch/lumberjack.v2"
)

func newPlatformBackend(atom zap.AtomicLevel, installDir, _ string, persistent bool) (platformBackend, error) {
	if !persistent {
		return platformBackend{
			cores: []zapcore.Core{consoleCore(atom, zapcore.AddSync(os.Stderr), false)},
		}, nil
	}

	logDir := defaultLogDir(installDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return platformBackend{}, backendError("创建日志目录失败", err)
	}

	appLogRotate := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, fmt.Sprintf("app_%s.log", time.Now().Format("2006-01-02"))),
		MaxSize:    10,
		MaxBackups: 7,
		MaxAge:     7,
		Compress:   true,
	}
	debugLogRotate := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, fmt.Sprintf("debug_%s.log", time.Now().Format("2006-01-02"))),
		MaxSize:    10,
		MaxBackups: 7,
		MaxAge:     7,
		Compress:   true,
	}

	fileEncoder := zapcore.NewJSONEncoder(logEncoderConfig())
	appCore := zapcore.NewCore(
		fileEncoder,
		zapcore.AddSync(appLogRotate),
		zap.LevelEnablerFunc(func(level zapcore.Level) bool {
			return level >= zapcore.InfoLevel
		}),
	)
	// Debug 文件只收 Debug 级日志，避免 Info 及以上日志重复落盘。
	debugCore := zapcore.NewCore(
		fileEncoder,
		zapcore.AddSync(debugLogRotate),
		zap.LevelEnablerFunc(func(level zapcore.Level) bool {
			return atom.Enabled(level) && level < zapcore.InfoLevel
		}),
	)
	cores := []zapcore.Core{appCore, debugCore}
	// 控制台输出只在 stdout 真的是交互式控制台（开发时从终端运行）时才加。
	// 核心进程启动时会把 stdout/stderr 指向 fatal_*.log 用来接住运行时崩溃输出，
	// 此时再加控制台 core，每条 Info 及以上日志都会带着 ANSI 颜色码、无缓冲地
	// 再写一遍进那个从不轮转的文件。
	if isInteractiveConsole(os.Stdout) {
		cores = append(cores, consoleCore(atom, zapcore.AddSync(os.Stdout), true))
	}

	return platformBackend{
		cores:   cores,
		logDir:  logDir,
		closers: []io.Closer{appLogRotate, debugLogRotate},
	}, nil
}

// isInteractiveConsole 报告 f 是否连接到控制台。文件、管道、NUL 设备与无效句柄都返回 false。
func isInteractiveConsole(f *os.File) bool {
	if f == nil {
		return false
	}
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}

func defaultLogDir(installDir string) string {
	return filepath.Join(installDir, "logs")
}
