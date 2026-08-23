//go:build windows

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsPlatformPrefersExplicitCodexExecutable(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "ChatGPT.exe")
	if err := os.WriteFile(executable, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_APP_PATH", executable)
	platform := &windowsPlatform{}
	if got := platform.locateCodex(); got != executable {
		t.Fatalf("locateCodex() = %q, want %q", got, executable)
	}
}

func TestWindowsPlatformLaunchIncludesEveryCDPArgument(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "ChatGPT.exe")
	if err := os.WriteFile(executable, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_APP_PATH", executable)
	var invokedExecutable string
	var invokedArguments []string
	runner := windowsCommandRunnerFunc(func(
		_ context.Context,
		command string,
		arguments []string,
		_ string,
		_ []string,
	) (CommandResult, error) {
		invokedExecutable = command
		invokedArguments = append([]string(nil), arguments...)
		return CommandResult{}, nil
	})
	platform := &windowsPlatform{runner: runner}
	if err := platform.LaunchCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if invokedExecutable != "cmd.exe" || !containsString(invokedArguments, executable) {
		t.Fatalf("unexpected launch: %q %#v", invokedExecutable, invokedArguments)
	}
	for _, argument := range CodexDebuggingArguments {
		if !containsString(invokedArguments, argument) {
			t.Fatalf("launch omitted %q: %#v", argument, invokedArguments)
		}
	}
}

func TestNewestExecutableUsesModificationTime(t *testing.T) {
	directory := t.TempDir()
	older := filepath.Join(directory, "older.exe")
	newer := filepath.Join(directory, "newer.exe")
	for _, path := range []string{older, newer} {
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatal(err)
	}
	if got := newestExecutable([]string{older, newer}); got != newer {
		t.Fatalf("newestExecutable() = %q, want %q", got, newer)
	}
}

type windowsCommandRunnerFunc func(
	context.Context,
	string,
	[]string,
	string,
	[]string,
) (CommandResult, error)

func (function windowsCommandRunnerFunc) Run(
	ctx context.Context,
	executable string,
	arguments []string,
	directory string,
	environment []string,
) (CommandResult, error) {
	return function(ctx, executable, arguments, directory, environment)
}
