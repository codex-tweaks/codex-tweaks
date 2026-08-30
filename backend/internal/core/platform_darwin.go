//go:build darwin

package core

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"time"
)

type darwinPlatform struct{ runner CommandRunner }

func NewPlatform(runner CommandRunner) Platform {
	if runner == nil {
		runner = SystemCommandRunner{}
	}
	return &darwinPlatform{runner: runner}
}

func (p *darwinPlatform) IsCodexRunning(ctx context.Context) (bool, error) {
	result, err := p.runner.Run(
		ctx,
		"/usr/bin/lsappinfo",
		[]string{"find", "bundleID=" + CodexBundleIdentifier},
		"",
		environmentSlice(environmentMap()),
	)
	if err != nil {
		return false, err
	}
	return result.Status == 0 && strings.TrimSpace(result.Output) != "", nil
}

func (p *darwinPlatform) ActivateCodex(ctx context.Context) error {
	result, err := p.runner.Run(ctx, "/usr/bin/open", []string{"-b", CodexBundleIdentifier}, "", environmentSlice(environmentMap()))
	if err != nil {
		return err
	}
	return requireCommandSuccess(result, "激活 Codex")
}

func (p *darwinPlatform) LaunchCodex(ctx context.Context, options CodexLaunchOptions) error {
	launchArguments := codexLaunchArguments(options, runtime.GOOS)
	arguments := append([]string{"-n", "-b", CodexBundleIdentifier, "--args"}, launchArguments...)
	result, err := p.runner.Run(ctx, "/usr/bin/open", arguments, "", environmentSlice(environmentMap()))
	if err != nil {
		return err
	}
	if result.Status == 0 {
		return nil
	}
	const fallback = "/Applications/ChatGPT.app"
	if directoryExists(fallback) {
		arguments = append([]string{"-n", fallback, "--args"}, launchArguments...)
		result, err = p.runner.Run(ctx, "/usr/bin/open", arguments, "", environmentSlice(environmentMap()))
		if err != nil {
			return err
		}
		if result.Status == 0 {
			return nil
		}
	}
	return errors.New("没有找到 ChatGPT.app（Codex 桌面客户端），或 Codex 启动失败。")
}

func (p *darwinPlatform) RestartCodex(ctx context.Context, options CodexLaunchOptions) error {
	_, _ = p.runner.Run(ctx, "/usr/bin/killall", []string{"-TERM", "ChatGPT"}, "", environmentSlice(environmentMap()))
	for range 25 {
		running, _ := p.IsCodexRunning(ctx)
		if !running {
			return p.LaunchCodex(ctx, options)
		}
		if err := waitContext(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
	_, _ = p.runner.Run(ctx, "/usr/bin/killall", []string{"-KILL", "ChatGPT"}, "", environmentSlice(environmentMap()))
	for range 15 {
		running, _ := p.IsCodexRunning(ctx)
		if !running {
			return p.LaunchCodex(ctx, options)
		}
		if err := waitContext(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
	return errors.New("Codex 未能正常退出，请手动退出后重试")
}

func (*darwinPlatform) Architecture() string { return runtime.GOARCH }

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
