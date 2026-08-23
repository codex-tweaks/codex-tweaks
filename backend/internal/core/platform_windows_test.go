//go:build windows

package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	if got := platform.locateUnpackagedCodex(); got != executable {
		t.Fatalf("locateUnpackagedCodex() = %q, want %q", got, executable)
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

func TestWindowsPlatformDiscoversAndActivatesPackagedCodex(t *testing.T) {
	t.Setenv("CODEX_APP_PATH", "")
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("ProgramFiles", t.TempDir())

	const appUserModelID = "OpenAI.Codex_2p2nqsd0c76g0!App"
	var invokedCommand string
	var activatedID string
	var activatedArguments string
	runner := windowsCommandRunnerFunc(func(
		_ context.Context,
		command string,
		arguments []string,
		_ string,
		_ []string,
	) (CommandResult, error) {
		invokedCommand = command
		if command != "powershell.exe" || !containsString(arguments, packagedCodexPowerShell) {
			t.Fatalf("unexpected discovery command: %q %#v", command, arguments)
		}
		return CommandResult{Output: "\ufeff" + appUserModelID + "\r\n"}, nil
	})
	platform := &windowsPlatform{
		runner: runner,
		activatePackaged: func(id, arguments string) (uint32, error) {
			activatedID = id
			activatedArguments = arguments
			return 42, nil
		},
	}
	if err := platform.LaunchCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if invokedCommand != "powershell.exe" || activatedID != appUserModelID {
		t.Fatalf("discovery/activation mismatch: command=%q id=%q", invokedCommand, activatedID)
	}
	for _, argument := range CodexDebuggingArguments {
		if !strings.Contains(activatedArguments, argument) {
			t.Fatalf("activation omitted %q: %q", argument, activatedArguments)
		}
	}
}

func TestWindowsPackagedCodexActivationIntegration(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_WINDOWS_APP_INTEGRATION") != "1" {
		t.Skip("set CODEX_TWEAKS_WINDOWS_APP_INTEGRATION=1 on a Windows machine with Codex installed")
	}
	platform := NewPlatform(SystemCommandRunner{}).(*windowsPlatform)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if appUserModelID := platform.locatePackagedCodex(ctx); appUserModelID == "" {
		t.Fatal("Microsoft Store Codex package was not discovered")
	}
	if err := platform.RestartCodex(ctx); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	var lastError error
	for ctx.Err() == nil {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, CodexCDPTargetsURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
			lastError = fmt.Errorf("CDP returned HTTP %d", response.StatusCode)
		} else {
			lastError = err
		}
		if err := waitContext(ctx, 500*time.Millisecond); err != nil {
			break
		}
	}
	t.Fatalf("Codex did not expose %s after packaged activation: %v", CodexCDPTargetsURL, lastError)
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
