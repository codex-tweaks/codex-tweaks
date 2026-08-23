package core

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestControllerUsesGoDefaultsReadsSkillAndDisablesNewPackages(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "SKILL.md")
	const skill = "# Codex Tweaks package skill\n"
	if err := os.WriteFile(skillPath, []byte(skill), 0o600); err != nil {
		t.Fatal(err)
	}
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			SkillPath:                   skillPath,
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "7",
		},
		nil,
		ControllerDependencies{DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()

	initial := controller.Snapshot()
	if !initial.Enabled || !initial.Update.AutoCheck || initial.Update.CurrentVersion != "0.1.0" {
		t.Fatalf("unexpected Go defaults: %#v", initial)
	}
	contents, err := controller.ReadAuthoringPrompt()
	if err != nil || contents != skill {
		t.Fatalf("skill contents = %q, err = %v", contents, err)
	}

	makeStorePackage(t, controller.store, "new-package", "new-package", "1.0.0", 0, nil, "")
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	afterReload := controller.Snapshot()
	if len(afterReload.Packages) != 1 || !containsString(afterReload.DisabledPackageIDs, "new-package") {
		t.Fatalf("new package must remain disabled: %#v", afterReload.DisabledPackageIDs)
	}
	actions := afterReload.Packages[0].AvailableActions
	if !actions.SetEnabled || !actions.SetPriority || !actions.OpenDirectory || actions.Build {
		t.Fatalf("unexpected Go-provided package actions: %#v", actions)
	}
	presentation := afterReload.Packages[0].Presentation
	if presentation.StatusTitle == "" || presentation.StatusDetail == "" || !presentation.IsPending {
		t.Fatalf("unexpected Go-provided package presentation: %#v", presentation)
	}
	if _, err := os.Stat(controller.configPath); err != nil {
		t.Fatalf("Go state was not persisted: %v", err)
	}
}

func TestControllerRejectsUnsupportedConfigurationSchema(t *testing.T) {
	root := t.TempDir()
	support := filepath.Join(root, "support")
	stateDirectory := filepath.Join(support, "Codex Tweaks", "State")
	if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(stateDirectory, "app-state.json"),
		[]byte(`{"schemaVersion":0}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: support,
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "1",
		},
		nil,
		ControllerDependencies{DisableBackground: true},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported configuration schema version 0") {
		t.Fatalf("expected unsupported schema error, got %v", err)
	}
}

func TestControllerUpdateFailureKeepsLastSuccessfulRelease(t *testing.T) {
	root := t.TempDir()
	requestSeen := make(chan *http.Request, 1)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestSeen <- request.Clone(request.Context())
		return nil, errors.New("test network failure")
	})}
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "1.0.0",
			BuildNumber:                 "1",
		},
		nil,
		ControllerDependencies{HTTPClient: client, DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()
	previous := GitHubRelease{TagName: "v1.1.0", Assets: []GitHubAsset{}}
	controller.mu.Lock()
	controller.latestRelease = &previous
	controller.mu.Unlock()

	controller.CheckAppUpdate(false)
	deadline := time.Now().Add(2 * time.Second)
	for controller.Snapshot().Update.LastError == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	snapshot := controller.Snapshot().Update
	if snapshot.LastError == nil || !strings.Contains(*snapshot.LastError, "test network failure") {
		t.Fatalf("missing update error: %#v", snapshot.LastError)
	}
	if snapshot.LatestRelease == nil || snapshot.LatestRelease.TagName != previous.TagName {
		t.Fatalf("last successful release was replaced: %#v", snapshot.LatestRelease)
	}
	select {
	case request := <-requestSeen:
		if request.Header.Get("Accept") != "application/vnd.github+json" || request.Header.Get("User-Agent") != "Codex-Tweaks/1.0.0" {
			t.Fatalf("update headers changed: %#v", request.Header)
		}
	default:
		t.Fatal("update request was not sent")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
