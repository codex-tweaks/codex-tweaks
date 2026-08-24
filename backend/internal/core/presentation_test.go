package core

import (
	"runtime"
	"testing"
)

func TestPresentationContractOwnsCopyTokensPlatformAndActions(t *testing.T) {
	contract := NewPresentationContract(PresentationState{
		Status:                   AppStatus{Kind: StatusConnected, TargetCount: 1},
		Enabled:                  true,
		GitAvailable:             true,
		LogAvailable:             true,
		AuthoringPromptAvailable: true,
		UpdateAvailable:          true,
	})
	if contract.Version != PresentationContractVersion || contract.Locale != "zh-CN" {
		t.Fatalf("unexpected contract identity: %#v", contract)
	}
	if contract.Text["app.name"] == "" || contract.Text["packages.status.active"] == "" {
		t.Fatalf("required presentation copy is missing: %#v", contract.Text)
	}
	if contract.Text["update.downloadProgress"] == "" || contract.Text["update.installingProgress"] == "" {
		t.Fatalf("update progress copy is missing: %#v", contract.Text)
	}
	if got := contract.Text["packages.title"]; got != "管理页面增强" {
		t.Fatalf("packages title = %q, want %q", got, "管理页面增强")
	}
	if contract.Tokens.PagePadding <= 0 || contract.Tokens.CardCornerRadius <= 0 {
		t.Fatalf("invalid presentation tokens: %#v", contract.Tokens)
	}
	if contract.Status.Title != "已连接 Codex" || contract.Status.Tone != "success" {
		t.Fatalf("status presentation is not owned by Go: %#v", contract.Status)
	}
	if contract.Platform.CDPEndpoint != CodexCDPEndpoint {
		t.Fatalf("CDP endpoint = %q, want %q", contract.Platform.CDPEndpoint, CodexCDPEndpoint)
	}
	if contract.Platform.RepositoryURL != UpdateRepositoryURL {
		t.Fatalf("repository URL = %q, want %q", contract.Platform.RepositoryURL, UpdateRepositoryURL)
	}
	if !contract.Actions.Reinject || !contract.Actions.InstallRemotePackage || !contract.Actions.ClearLog {
		t.Fatalf("expected connected actions to be available: %#v", contract.Actions)
	}
	if runtime.GOOS == "windows" && contract.Platform.UpdateInstallStrategy != "velopack" {
		t.Fatalf("Windows update strategy = %q", contract.Platform.UpdateInstallStrategy)
	}
}

func TestPresentationContractDisablesConflictingPackageInstallActions(t *testing.T) {
	contract := NewPresentationContract(PresentationState{
		Status:                  AppStatus{Kind: StatusConnected},
		Enabled:                 true,
		GitAvailable:            true,
		InstallingLocalPackage:  true,
		InstallingRemotePackage: false,
	})
	if contract.Actions.InstallLocalPackage || contract.Actions.InstallRemotePackage {
		t.Fatalf("install actions must be disabled while an install is active: %#v", contract.Actions)
	}
}

func TestPresentationContractDisablesInstallActionsDuringPackageExport(t *testing.T) {
	contract := NewPresentationContract(PresentationState{
		Status:           AppStatus{Kind: StatusConnected},
		Enabled:          true,
		GitAvailable:     true,
		ExportingPackage: true,
	})
	if contract.Actions.InstallLocalPackage || contract.Actions.InstallRemotePackage {
		t.Fatalf("install actions must be disabled while an export is active: %#v", contract.Actions)
	}
}

func TestPresentationContractCanBeGeneratedForAnExplicitPlatform(t *testing.T) {
	contract := NewPresentationContractForPlatform(PresentationState{}, "windows", "arm64")
	if contract.Platform.OperatingSystem != "windows" || contract.Platform.Architecture != "arm64" {
		t.Fatalf("unexpected explicit platform: %#v", contract.Platform)
	}
	if contract.Platform.UpdateInstallStrategy != "velopack" || !contract.Actions.InstallAppUpdate {
		t.Fatalf("Windows update presentation is incomplete: %#v %#v", contract.Platform, contract.Actions)
	}
}

func TestPresentationContractUsesSparkleForMacOSUpdates(t *testing.T) {
	contract := NewPresentationContractForPlatform(
		PresentationState{UpdateAvailable: true},
		"darwin",
		"arm64",
	)
	if contract.Platform.UpdateInstallStrategy != "sparkle" || !contract.Actions.InstallAppUpdate {
		t.Fatalf("macOS update presentation is incomplete: %#v %#v", contract.Platform, contract.Actions)
	}
}

func TestPresentationContractKeepsPlatformWindowMetricsIndependent(t *testing.T) {
	macOS := NewPresentationContractForPlatform(PresentationState{}, "darwin", "universal")
	windows := NewPresentationContractForPlatform(PresentationState{}, "windows", "x64")

	if macOS.Tokens.WindowMinWidth != 820 || macOS.Tokens.WindowMinHeight != 560 ||
		macOS.Tokens.WindowDefaultWidth != 920 || macOS.Tokens.WindowDefaultHeight != 640 {
		t.Fatalf("macOS window metrics changed unexpectedly: %#v", macOS.Tokens)
	}
	if windows.Tokens.WindowMinWidth != 1120 || windows.Tokens.WindowMinHeight != 800 ||
		windows.Tokens.WindowDefaultWidth != 1320 || windows.Tokens.WindowDefaultHeight != 920 {
		t.Fatalf("Windows window metrics changed unexpectedly: %#v", windows.Tokens)
	}
}
