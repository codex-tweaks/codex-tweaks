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
