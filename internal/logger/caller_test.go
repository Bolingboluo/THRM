package logger

import (
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type wrappingService struct{ logger Logger }

func (s *wrappingService) logInfo(format string, v ...any) {
	Wrapped(s.logger).Info(format, v...)
}

func TestCallerPointsAtBusinessCode(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	zl := newZapLogger(core)
	l := &CustomLogger{logger: zl, sugar: zl.Sugar(), atom: zap.NewAtomicLevelAt(zapcore.DebugLevel)}

	l.Info("direct") // 直接调用：调用位置应是这一行
	svc := &wrappingService{logger: l}
	svc.logInfo("wrapped") // 经包装函数：调用位置应是这一行，而不是 logInfo 内部

	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("got %d entries", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Caller.File, "caller_test.go") || e.Caller.Function != "github.com/TIANLI0/THRM/internal/logger.TestCallerPointsAtBusinessCode" {
			t.Fatalf("%q 的调用位置是 %s (%s)，应指向测试函数本身", e.Message, e.Caller.TrimmedPath(), e.Caller.Function)
		}
	}

	if Wrapped(l) != Wrapped(l) {
		t.Fatal("Wrapped 应复用同一个实例")
	}
	var nilLogger Logger
	if Wrapped(nilLogger) != nil {
		t.Fatal("nil 应原样返回")
	}
}
