//go:build windows

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const packagedCodexPowerShell = `$ErrorActionPreference = 'SilentlyContinue'
$package = Get-AppxPackage -Name 'OpenAI.Codex' | Sort-Object Version -Descending | Select-Object -First 1
if ($null -ne $package) {
    $application = ($package | Get-AppxPackageManifest).Package.Applications.Application |
        Where-Object { $_.Executable -match '(^|[\\/])ChatGPT[.]exe$' } |
        Select-Object -First 1
    if ($null -eq $application) {
        $application = ($package | Get-AppxPackageManifest).Package.Applications.Application |
            Where-Object { $_.Executable } |
            Select-Object -First 1
    }
    if ($null -ne $application) {
        Write-Output ($package.PackageFamilyName + '!' + $application.Id)
    }
}`

type windowsPackageActivator func(appUserModelID, arguments string) (uint32, error)

type windowsPlatform struct {
	runner           CommandRunner
	activatePackaged windowsPackageActivator
}

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
	return p.LaunchCodex(ctx, CodexLaunchOptions{})
}

func (p *windowsPlatform) LaunchCodex(ctx context.Context, options CodexLaunchOptions) error {
	launchArguments := codexLaunchArguments(options, runtime.GOOS)
	if executable := p.locateUnpackagedCodex(); executable != "" {
		return p.launchUnpackagedCodex(ctx, executable, launchArguments)
	}

	appUserModelID := p.locatePackagedCodex(ctx)
	if appUserModelID == "" {
		return errors.New("没有找到 Codex Windows 桌面客户端；请确认已为当前 Windows 用户安装 Microsoft Store 版 Codex")
	}
	activate := p.activatePackaged
	if activate == nil {
		activate = activatePackagedApplication
	}
	if _, err := activate(appUserModelID, strings.Join(launchArguments, " ")); err != nil {
		return fmt.Errorf("启动 Codex Windows 应用失败：%w", err)
	}
	return nil
}

func (p *windowsPlatform) launchUnpackagedCodex(ctx context.Context, executable string, launchArguments []string) error {
	arguments := []string{"/D", "/C", "start", "", "/B", executable}
	arguments = append(arguments, launchArguments...)
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

func (p *windowsPlatform) RestartCodex(ctx context.Context, options CodexLaunchOptions) error {
	_, _ = p.runner.Run(ctx, "taskkill.exe", []string{"/IM", "ChatGPT.exe", "/T"}, "", environmentSlice(environmentMap()))
	for range 25 {
		running, _ := p.IsCodexRunning(ctx)
		if !running {
			return p.LaunchCodex(ctx, options)
		}
		if err := waitContext(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
	_, _ = p.runner.Run(ctx, "taskkill.exe", []string{"/F", "/IM", "ChatGPT.exe", "/T"}, "", environmentSlice(environmentMap()))
	return p.LaunchCodex(ctx, options)
}

func (*windowsPlatform) Architecture() string { return runtime.GOARCH }

func (*windowsPlatform) locateUnpackagedCodex() string {
	for _, candidate := range []string{
		os.Getenv("CODEX_APP_PATH"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps", "ChatGPT.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "ChatGPT", "ChatGPT.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Codex", "ChatGPT.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "ChatGPT", "ChatGPT.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Codex", "ChatGPT.exe"),
	} {
		if isExecutable(candidate) {
			return candidate
		}
	}
	if candidate, err := exec.LookPath("ChatGPT.exe"); err == nil && isExecutable(candidate) {
		return candidate
	}
	return ""
}

func (p *windowsPlatform) locatePackagedCodex(ctx context.Context) string {
	result, err := p.runner.Run(
		ctx,
		"powershell.exe",
		[]string{"-NoProfile", "-NonInteractive", "-Command", packagedCodexPowerShell},
		"",
		environmentSlice(environmentMap()),
	)
	if err != nil || result.Status != 0 {
		return ""
	}
	for _, line := range strings.Split(result.Output, "\n") {
		candidate := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		lowercase := strings.ToLower(candidate)
		if strings.HasPrefix(lowercase, "openai.codex_") && strings.Contains(candidate, "!") && !strings.ContainsAny(candidate, " \t") {
			return candidate
		}
	}
	return ""
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
