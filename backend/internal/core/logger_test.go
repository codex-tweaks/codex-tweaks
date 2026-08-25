package core

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLoggerClearAndNewestPreview(t *testing.T) {
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("before-clear")
	contents, err := logger.ReadPreviewNewestFirst()
	if err != nil || !strings.Contains(contents, "before-clear") {
		t.Fatalf("log missing: %q %v", contents, err)
	}
	if err := logger.Clear(); err != nil {
		t.Fatal(err)
	}
	contents, _ = logger.ReadPreviewNewestFirst()
	if contents != "" {
		t.Fatalf("log not cleared: %q", contents)
	}
	logger.Error("after-clear")
	contents, _ = logger.ReadPreviewNewestFirst()
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

func TestLoggerReadPreviewNewestFirstLimitsLines(t *testing.T) {
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var contents strings.Builder
	lineCount := logPreviewMaxLines + 5
	for index := range lineCount {
		fmt.Fprintf(&contents, "line-%04d\n", index)
	}
	if err := os.WriteFile(logger.Path, []byte(contents.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	preview, err := logger.ReadPreviewNewestFirst()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(preview, "\n")
	if len(lines) != logPreviewMaxLines {
		t.Fatalf("preview lines = %d, want %d", len(lines), logPreviewMaxLines)
	}
	if lines[0] != fmt.Sprintf("line-%04d", lineCount-1) {
		t.Fatalf("newest line = %q", lines[0])
	}
	if lines[len(lines)-1] != "line-0005" {
		t.Fatalf("oldest preview line = %q", lines[len(lines)-1])
	}
}

func TestLoggerReadPreviewNewestFirstLimitsBytes(t *testing.T) {
	logger, err := NewLogger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var contents strings.Builder
	line := strings.Repeat("x", 1023)
	for index := range 512 {
		fmt.Fprintf(&contents, "%03d-%s\n", index, line)
	}
	if err := os.WriteFile(logger.Path, []byte(contents.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	preview, err := logger.ReadPreviewNewestFirst()
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) > logPreviewMaxBytes {
		t.Fatalf("preview bytes = %d, want at most %d", len(preview), logPreviewMaxBytes)
	}
	if !strings.HasPrefix(preview, "511-") {
		t.Fatalf("preview does not start with newest line: %.16q", preview)
	}
	if strings.Contains(preview, "000-") {
		t.Fatal("preview still contains the oldest line")
	}
}

func BenchmarkLoggerReadPreviewNewestFirstLargeFile(b *testing.B) {
	logger, err := NewLogger(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	file, err := os.OpenFile(logger.Path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		b.Fatal(err)
	}
	line := strings.Repeat("x", 127) + "\n"
	for range 100_000 {
		if _, err := file.WriteString(line); err != nil {
			b.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		if _, err := logger.ReadPreviewNewestFirst(); err != nil {
			b.Fatal(err)
		}
	}
}
