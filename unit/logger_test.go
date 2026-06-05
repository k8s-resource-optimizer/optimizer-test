package unit_test

import (
	"errors"
	"testing"

	"intelligent-cluster-optimizer/pkg/logger"
)

func TestNewLogger_ProductionLevel(t *testing.T) {
	l, err := logger.NewLogger("info", false)
	if err != nil {
		t.Fatalf("NewLogger(info, false) error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLogger_DebugLevel(t *testing.T) {
	l, err := logger.NewLogger("debug", true)
	if err != nil {
		t.Fatalf("NewLogger(debug, true) error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLogger_WarnLevel(t *testing.T) {
	l, err := logger.NewLogger("warn", false)
	if err != nil {
		t.Fatalf("NewLogger(warn, false) error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLogger_ErrorLevel(t *testing.T) {
	l, err := logger.NewLogger("error", false)
	if err != nil {
		t.Fatalf("NewLogger(error, false) error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLogger_InvalidLevel_FallsBackToInfo(t *testing.T) {
	// Invalid level silently falls back to info level (no error returned)
	l, err := logger.NewLogger("invalid-level", false)
	if err != nil {
		t.Fatalf("NewLogger with invalid level returned unexpected error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger even with invalid level")
	}
}

func TestNewProductionLogger(t *testing.T) {
	l, err := logger.NewProductionLogger()
	if err != nil {
		t.Fatalf("NewProductionLogger error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil production logger")
	}
}

func TestNewDevelopmentLogger(t *testing.T) {
	l, err := logger.NewDevelopmentLogger()
	if err != nil {
		t.Fatalf("NewDevelopmentLogger error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil development logger")
	}
}

func TestInitGlobalLogger_And_GetLogger(t *testing.T) {
	if err := logger.InitGlobalLogger("info", false); err != nil {
		t.Fatalf("InitGlobalLogger error: %v", err)
	}
	l := logger.GetLogger()
	if l == nil {
		t.Fatal("expected non-nil global logger after init")
	}
}

func TestLogger_WithFields(t *testing.T) {
	l, _ := logger.NewLogger("info", false)
	l2 := l.WithFields("key", "value", "count", 42)
	if l2 == nil {
		t.Fatal("expected non-nil logger from WithFields")
	}
}

func TestLogger_WithError(t *testing.T) {
	l, _ := logger.NewLogger("info", false)
	err := errors.New("something failed")
	l2 := l.WithError(err)
	if l2 == nil {
		t.Fatal("expected non-nil logger from WithError")
	}
}

func TestLogger_Sync(t *testing.T) {
	l, _ := logger.NewLogger("info", false)
	_ = l.Sync()
}

func TestGlobalLogger_PackageFunctions_NoPanic(t *testing.T) {
	_ = logger.InitGlobalLogger("info", false)

	logger.Debug("debug message")
	logger.Debugf("debug %s", "formatted")
	logger.Info("info message")
	logger.Infof("info %s", "formatted")
	logger.Warn("warn message")
	logger.Warnf("warn %s", "formatted")
	logger.Error("error message")
	logger.Errorf("error %s", "formatted")
}

func TestGlobalLogger_WithFields_NoPanic(t *testing.T) {
	_ = logger.InitGlobalLogger("debug", true)
	l := logger.WithFields("ns", "default", "workload", "stress-cyclic")
	if l == nil {
		t.Fatal("expected non-nil logger from package-level WithFields")
	}
}

func TestGlobalLogger_WithError_NoPanic(t *testing.T) {
	_ = logger.InitGlobalLogger("info", false)
	l := logger.WithError(errors.New("test error"))
	if l == nil {
		t.Fatal("expected non-nil logger from package-level WithError")
	}
}

func TestGlobalLogger_Sync_NoPanic(t *testing.T) {
	_ = logger.InitGlobalLogger("info", false)
	_ = logger.Sync()
}

func TestGetLogger_BeforeInit_ReturnsNop(t *testing.T) {
	l := logger.GetLogger()
	if l == nil {
		t.Fatal("GetLogger should never return nil")
	}
}
