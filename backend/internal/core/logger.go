package core

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logPreviewMaxLines = 1_000
	logPreviewMaxBytes = 256 * 1024
)

type Logger struct {
	Path string
	mu   sync.Mutex
}

func NewLogger(applicationSupport string) (*Logger, error) {
	if applicationSupport == "" {
		var err error
		applicationSupport, err = os.UserConfigDir()
		if err != nil {
			return nil, err
		}
	}
	directory := filepath.Join(applicationSupport, "Codex Tweaks", "Logs")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	logger := &Logger{Path: filepath.Join(directory, "codex-tweaks.log")}
	if err := logger.EnsureExists(); err != nil {
		return nil, err
	}
	return logger, nil
}

func (l *Logger) Info(message string)  { l.append("INFO", message) }
func (l *Logger) Error(message string) { l.append("ERROR", message) }

func (l *Logger) EnsureExists() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ensureExistsLocked()
}

func (l *Logger) ReadPreviewNewestFirst() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureExistsLocked(); err != nil {
		return "", err
	}
	file, err := os.Open(l.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	readSize := info.Size()
	if readSize > logPreviewMaxBytes {
		readSize = logPreviewMaxBytes
	}
	if readSize == 0 {
		return "", nil
	}
	start := info.Size() - readSize
	contents := make([]byte, int(readSize))
	read, err := file.ReadAt(contents, start)
	if err != nil && err != io.EOF {
		return "", err
	}
	contents = contents[:read]
	if start > 0 {
		if newline := bytes.IndexByte(contents, '\n'); newline >= 0 {
			contents = contents[newline+1:]
		}
	}
	if len(contents) == 0 {
		return "", nil
	}
	contents = bytes.TrimSuffix(contents, []byte{'\n'})
	lines := strings.Split(strings.ToValidUTF8(string(contents), "�"), "\n")
	if len(lines) > logPreviewMaxLines {
		lines = lines[len(lines)-logPreviewMaxLines:]
	}
	for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
		lines[left], lines[right] = lines[right], lines[left]
	}
	return strings.Join(lines, "\n"), nil
}

func (l *Logger) Clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureExistsLocked(); err != nil {
		return err
	}
	return os.WriteFile(l.Path, nil, 0o600)
}

func (l *Logger) append(level, message string) {
	log.Printf("[%s] %s", level, message)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureExistsLocked(); err != nil {
		return
	}
	file, err := os.OpenFile(l.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s [%s] %s\n", time.Now().UTC().Format(time.RFC3339Nano), level, message)
}

func (l *Logger) ensureExistsLocked() error {
	file, err := os.OpenFile(l.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(l.Path, 0o600)
}
