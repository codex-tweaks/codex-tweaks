//go:build windows

package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type windowsPlatform struct{ runner CommandRunner }

func NewPlatform(runner CommandRunner) Platform {
	if runner == nil {
		runner = SystemCommandRunner{}
	}
	return &windowsPlatform{runner: runner}
}

func (p *windowsPlatform) IsCodexRunning(ctx context.Context) (bool, error) {
	result, err := p.runner.Run(ctx, "tasklist.exe", []string{"/FI", "IMAGENAME eq ChatGPT.exe", "/FO", "CSV", "/NH"}, "", environmentSlice(environmentMap()))
	if err != nil {
		return false, err
	}
	return result.Status == 0 && strings.Contains(strings.ToLower(result.Output), "chatgpt.exe"), nil
}

func (p *windowsPlatform) ActivateCodex(ctx context.Context) error {
	// Starting the registered executable activates an existing single-instance
	// Electron app and is also the safe fallback when no window is present.
	return p.LaunchCodex(ctx)
}

func (p *windowsPlatform) LaunchCodex(ctx context.Context) error {
	executable := p.locateCodex()
	if executable == "" {
		return errors.New("没有找到 ChatGPT.exe（Codex 桌面客户端）")
	}
	arguments := []string{"/D", "/C", "start", "", "/B", executable}
	arguments = append(arguments, CodexDebuggingArguments...)
	result, err := p.runner.Run(
		ctx,
		"cmd.exe",
		arguments,
		filepath.Dir(executable),
		environmentSlice(environmentMap()),
	)
	if err != nil {
		return err
	}
	return requireCommandSuccess(result, "启动 Codex")
}

func (p *windowsPlatform) RestartCodex(ctx context.Context) error {
	_, _ = p.runner.Run(ctx, "taskkill.exe", []string{"/IM", "ChatGPT.exe", "/T"}, "", environmentSlice(environmentMap()))
	for range 25 {
		running, _ := p.IsCodexRunning(ctx)
		if !running {
			return p.LaunchCodex(ctx)
		}
		if err := waitContext(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
	_, _ = p.runner.Run(ctx, "taskkill.exe", []string{"/F", "/IM", "ChatGPT.exe", "/T"}, "", environmentSlice(environmentMap()))
	return p.LaunchCodex(ctx)
}

func (*windowsPlatform) Architecture() string { return runtime.GOARCH }

func (*windowsPlatform) locateCodex() string {
	for _, candidate := range []string{
		os.Getenv("CODEX_APP_PATH"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps", "ChatGPT.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "ChatGPT", "ChatGPT.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "ChatGPT", "ChatGPT.exe"),
	} {
		if isExecutable(candidate) {
			return candidate
		}
	}
	if candidate, err := exec.LookPath("ChatGPT.exe"); err == nil && isExecutable(candidate) {
		return candidate
	}
	for _, programFiles := range uniqueNonEmptyStrings(
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramW6432"),
	) {
		matches, _ := filepath.Glob(filepath.Join(
			programFiles,
			"WindowsApps",
			"OpenAI.Codex_*__2p2nqsd0c76g0",
			"app",
			"ChatGPT.exe",
		))
		if candidate := newestExecutable(matches); candidate != "" {
			return candidate
		}
	}
	return ""
}

func newestExecutable(candidates []string) string {
	var newest string
	var newestTime time.Time
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest = candidate
			newestTime = info.ModTime()
		}
	}
	return newest
}

func uniqueNonEmptyStrings(values ...string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
