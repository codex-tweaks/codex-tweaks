package core

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLoggerClearAndNewestFirst(t *testing.T) {
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("before-clear")
	contents, err := logger.ReadNewestFirst()
	if err != nil || !strings.Contains(contents, "before-clear") {
		t.Fatalf("log missing: %q %v", contents, err)
	}
	if err := logger.Clear(); err != nil {
		t.Fatal(err)
	}
	contents, _ = logger.ReadNewestFirst()
	if contents != "" {
		t.Fatalf("log not cleared: %q", contents)
	}
	logger.Error("after-clear")
	contents, _ = logger.ReadNewestFirst()
	if !strings.Contains(contents, "after-clear") {
		t.Fatalf("log no longer writable: %q", contents)
	}
	info, err := os.Stat(logger.Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o", info.Mode().Perm())
	}
}
