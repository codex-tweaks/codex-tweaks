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
	runner            CommandRunner
	activatePackaged  windowsPackageActivator
	restoreNotifyIcon func(context.Context, codexNotifyIconTarget) error
	repairLifetime    context.Context
	repairLogger      *Logger
}

// The notification area repair keeps running after LaunchCodex returned, so it must not depend on
// the context of that call; the controller hands over the application lifetime and the log instead.
func (p *windowsPlatform) useBackgroundRepairContext(ctx context.Context, logger *Logger) {
	p.repairLifetime = ctx
	p.repairLogger = logger
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
	return p.LaunchCodex(ctx)
}

func (p *windowsPlatform) LaunchCodex(ctx context.Context) error {
	if executable := p.locateUnpackagedCodex(); executable != "" {
		if err := p.launchUnpackagedCodex(ctx, executable); err != nil {
			return err
		}
		// cmd.exe starts Codex detached, so the executable is the only thing known about the new
		// instance. The full path keeps the repair away from another ChatGPT.exe on the machine.
		p.scheduleNotifyIconRestore(codexNotifyIconTarget{executablePath: executable})
		return nil
	}

	appUserModelID := p.locatePackagedCodex(ctx)
	if appUserModelID == "" {
		return errors.New("没有找到 Codex Windows 桌面客户端；请确认已为当前 Windows 用户安装 Microsoft Store 版 Codex")
	}
	activate := p.activatePackaged
	if activate == nil {
		activate = activatePackagedApplication
	}
	processID, err := activate(appUserModelID, strings.Join(CodexDebuggingArguments, " "))
	if err != nil {
		return fmt.Errorf("启动 Codex Windows 应用失败：%w", err)
	}
	// The activation reports the process it started or brought to the front, which is the instance
	// whose icon has to come back.
	p.scheduleNotifyIconRestore(codexNotifyIconTarget{processID: processID})
	return nil
}

// Every launch path schedules the repair, not only RestartCodex: the shell keeps the stale record
// after any abrupt end of Codex - our own restart, a crash, or the user ending the process - and the
// next launch runs into the same refusal. Asking a Codex that already owns its icon to add it again
// is harmless, the shell keeps a single record for it.
//
// Codex adds the icon while it starts up, so the repair has to wait for the new instance. It runs in
// the background because starting Codex must neither block on it nor fail when the icon cannot be
// repaired, and a failure is reported through the log instead of being dropped.
func (p *windowsPlatform) scheduleNotifyIconRestore(target codexNotifyIconTarget) {
	restore := p.restoreNotifyIcon
	if restore == nil {
		restore = restoreCodexNotifyIcon
	}
	lifetime := p.repairLifetime
	if lifetime == nil {
		lifetime = context.Background()
	}
	logger := p.repairLogger
	go func() {
		ctx, cancel := context.WithTimeout(lifetime, codexNotifyIconRepairTimeout)
		defer cancel()
		err := restore(ctx, target)
		switch {
		case err == nil:
		case lifetime.Err() != nil:
			// Codex Tweaks is shutting down; the icon of a Codex we are leaving behind is moot.
		case logger != nil:
			logger.Warn("Codex 通知区域图标补发失败（" + target.describe() + "）：" + err.Error())
		}
	}()
}

func (p *windowsPlatform) launchUnpackagedCodex(ctx context.Context, executable string) error {
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
