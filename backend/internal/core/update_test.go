package core

import (
	"strings"
	"testing"
)

func TestUpdateReleaseSelectionContracts(t *testing.T) {
	stable := SelectLatestRelease([]GitHubRelease{
		{TagName: "v1.1.0"}, {TagName: "v9.0.0", Draft: true},
		{TagName: "v2.0.0-beta.1", Prerelease: true}, {TagName: "not-a-version"},
	}, UpdateStable)
	if stable == nil || stable.TagName != "v1.1.0" {
		t.Fatalf("unexpected stable release: %#v", stable)
	}
	beta := SelectLatestRelease([]GitHubRelease{
		{TagName: "v1.1.0"}, {TagName: "v1.2.0-beta.10", Prerelease: true},
		{TagName: "v1.2.0-rc.1", Prerelease: true}, {TagName: "v9.0.0-alpha.1", Prerelease: true},
	}, UpdateBeta)
	if beta == nil || beta.TagName != "v1.2.0-rc.1" {
		t.Fatalf("unexpected beta release: %#v", beta)
	}
}

func TestPreferredDownloadURL(t *testing.T) {
	releasePage, universal, arm64, x86 := "https://example/release", "https://example/universal", "https://example/arm64", "https://example/x86"
	release := GitHubRelease{TagName: "v1.0.0", HTMLURL: &releasePage, Assets: []GitHubAsset{
		{Name: "Codex-Tweaks-v1.0.0.dmg", BrowserDownloadURL: &universal},
		{Name: "Codex-Tweaks-v1.0.0-arm64.dmg", BrowserDownloadURL: &arm64},
		{Name: "Codex-Tweaks-v1.0.0-x86_64.dmg", BrowserDownloadURL: &x86},
	}}
	if got := PreferredDownloadURL(release, "darwin", "arm64"); got == nil || *got != arm64 {
		t.Fatalf("got %#v", got)
	}
	if got := PreferredDownloadURL(release, "darwin", "amd64"); got == nil || *got != x86 {
		t.Fatalf("got %#v", got)
	}
	if !HasNewerVersion(&release, "0.9.0") || HasNewerVersion(&release, "1.0.0") {
		t.Fatal("version comparison changed")
	}
}

func TestPreferredWindowsDownloadUsesMSIThenEXEThenReleasePage(t *testing.T) {
	page, msi, executable := "https://example/release", "https://example/app.msi", "https://example/app.exe"
	withMSI := GitHubRelease{HTMLURL: &page, Assets: []GitHubAsset{
		{Name: "Codex-Tweaks-v1.0.0-x86_64.exe", BrowserDownloadURL: &executable},
		{Name: "Codex-Tweaks-v1.0.0-x86_64.msi", BrowserDownloadURL: &msi},
	}}
	if got := PreferredDownloadURL(withMSI, "windows", "amd64"); got == nil || *got != msi {
		t.Fatalf("MSI preference = %#v", got)
	}
	withEXE := GitHubRelease{HTMLURL: &page, Assets: []GitHubAsset{
		{Name: "Codex-Tweaks-v1.0.0-x86_64.exe", BrowserDownloadURL: &executable},
	}}
	if got := PreferredDownloadURL(withEXE, "windows", "amd64"); got == nil || *got != executable {
		t.Fatalf("EXE fallback = %#v", got)
	}
	if got := PreferredDownloadURL(GitHubRelease{HTMLURL: &page}, "windows", "amd64"); got == nil || *got != page {
		t.Fatalf("release-page fallback = %#v", got)
	}
}

func TestCodexDebuggingArgumentsAreLoopbackOnly(t *testing.T) {
	if !containsString(CodexDebuggingArguments, "--remote-debugging-address=127.0.0.1") || !containsString(CodexDebuggingArguments, "--remote-debugging-port=9335") {
		t.Fatalf("missing loopback arguments: %#v", CodexDebuggingArguments)
	}
	for _, argument := range CodexDebuggingArguments {
		if strings.Contains(argument, "0.0.0.0") {
			t.Fatalf("unsafe argument: %s", argument)
		}
	}
}
