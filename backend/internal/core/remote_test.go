package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteManagerInstallsLatestSemverAndFindsUpdate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	store, root := newTestStore(t)
	repository := filepath.Join(root, "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "init", "--quiet", "--initial-branch", "main")
	runGitTest(t, repository, "config", "user.email", "tests@codex-tweaks.invalid")
	runGitTest(t, repository, "config", "user.name", "Codex Tweaks Tests")
	writeRemotePackage(t, repository, "1.0.0", nil)
	commitRemotePackage(t, repository, "v1")
	runGitTest(t, repository, "tag", "v1.0.0")

	remoteURL := "https://example.test/codex-tweaks-package.git"
	environment := environmentMap()
	environment["GIT_CONFIG_COUNT"] = "1"
	environment["GIT_CONFIG_KEY_0"] = "url.file://" + repository + "/.insteadOf"
	environment["GIT_CONFIG_VALUE_0"] = remoteURL
	environment["GIT_ALLOW_PROTOCOL"] = "file"
	manager := NewRemoteManager(store, nil, nil, environment)
	source := PackageSource{URL: remoteURL, Selector: NewRemoteSelector(SelectorLatestSemverTag, "")}
	first, err := manager.Install(context.Background(), source, RemoteInstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.PackageID != "remote-sample" || first.Manifest.Version != "1.0.0" || first.Lock.ResolvedReference != "v1.0.0" {
		t.Fatalf("unexpected install: %#v", first)
	}
	packages, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || packages[0].Origin.Kind != OriginManaged {
		t.Fatalf("managed package not loaded: %#v", packages)
	}

	writeRemotePackage(t, repository, "1.1.0", nil)
	commitRemotePackage(t, repository, "v1.1")
	runGitTest(t, repository, "tag", "v1.1.0")
	update, err := manager.CheckForUpdate(context.Background(), "remote-sample")
	if err != nil {
		t.Fatal(err)
	}
	if update.Status != RemoteUpdateAvailable || update.CandidateReference != "v1.1.0" || update.CandidateCommit == update.CurrentCommit {
		t.Fatalf("unexpected update: %#v", update)
	}
}

func TestRemoteManagerRejectsPackageWithoutLockfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	store, root := newTestStore(t)
	repository := filepath.Join(root, "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "init", "--quiet", "--initial-branch", "main")
	runGitTest(t, repository, "config", "user.email", "tests@codex-tweaks.invalid")
	runGitTest(t, repository, "config", "user.name", "Codex Tweaks Tests")
	writeRemotePackage(t, repository, "1.0.0", map[string]string{"demo": "1.0.0"})
	commitRemotePackage(t, repository, "missing lock")
	remoteURL := "https://example.test/invalid.git"
	environment := environmentMap()
	environment["GIT_CONFIG_COUNT"] = "1"
	environment["GIT_CONFIG_KEY_0"] = "url.file://" + repository + "/.insteadOf"
	environment["GIT_CONFIG_VALUE_0"] = remoteURL
	environment["GIT_ALLOW_PROTOCOL"] = "file"
	manager := NewRemoteManager(store, nil, nil, environment)
	_, err := manager.Install(context.Background(), PackageSource{URL: remoteURL, Selector: NewRemoteSelector(SelectorBranch, "main")}, RemoteInstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "package-lock.json") {
		t.Fatalf("expected lockfile rejection, got %v", err)
	}
	registration, loadErr := manager.Registration("remote-sample")
	if loadErr != nil || registration != nil {
		t.Fatalf("invalid package must not be registered: %#v %v", registration, loadErr)
	}
}

func TestRemoteManagerReportsMovedPinnedTagWithoutOfferingOrdinaryUpdate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	store, root := newTestStore(t)
	repository := filepath.Join(root, "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "init", "--quiet", "--initial-branch", "main")
	runGitTest(t, repository, "config", "user.email", "tests@codex-tweaks.invalid")
	runGitTest(t, repository, "config", "user.name", "Codex Tweaks Tests")
	writeRemotePackage(t, repository, "1.0.0", nil)
	commitRemotePackage(t, repository, "first")
	runGitTest(t, repository, "tag", "v1.0.0")

	remoteURL := "https://example.test/pinned.git"
	environment := environmentMap()
	environment["GIT_CONFIG_COUNT"] = "1"
	environment["GIT_CONFIG_KEY_0"] = "url.file://" + repository + "/.insteadOf"
	environment["GIT_CONFIG_VALUE_0"] = remoteURL
	environment["GIT_ALLOW_PROTOCOL"] = "file"
	manager := NewRemoteManager(store, nil, nil, environment)
	source := PackageSource{URL: remoteURL, Selector: NewRemoteSelector(SelectorTag, "v1.0.0")}
	installed, err := manager.Install(context.Background(), source, RemoteInstallOptions{})
	if err != nil {
		t.Fatal(err)
	}

	writeRemotePackage(t, repository, "1.0.1", nil)
	commitRemotePackage(t, repository, "move tag")
	runGitTest(t, repository, "tag", "--force", "v1.0.0")
	update, err := manager.CheckForUpdate(context.Background(), installed.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if update.Status != RemoteUpdatePinnedReferenceChanged || update.Installable() {
		t.Fatalf("moved pinned tag must require explicit review: %#v", update)
	}
}

func writeRemotePackage(t *testing.T, repository, version string, dependencies map[string]string) {
	t.Helper()
	writeTestFile(t, filepath.Join(repository, "src", "index.js"), "export function activate() {}\n")
	if dependencies == nil {
		dependencies = map[string]string{}
	}
	manifest := PackageManifest{Name: "remote-sample", Version: version, Description: "Remote fixture", Dependencies: dependencies, CodexTweaks: PackageConfiguration{APIVersion: 2, Entry: "src/index.js", PackageDependencies: map[string]PackageDependency{}}}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "package.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func commitRemotePackage(t *testing.T, repository, message string) {
	t.Helper()
	runGitTest(t, repository, "add", ".")
	runGitTest(t, repository, "commit", "--quiet", "-m", message)
}

func runGitTest(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}
