package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type idleControllerTestPlatform struct{}

func (idleControllerTestPlatform) IsCodexRunning(context.Context) (bool, error) { return false, nil }
func (idleControllerTestPlatform) ActivateCodex(context.Context) error          { return nil }
func (idleControllerTestPlatform) LaunchCodex(context.Context) error            { return nil }
func (idleControllerTestPlatform) RestartCodex(context.Context) error           { return nil }
func (idleControllerTestPlatform) Architecture() string                         { return "amd64" }

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
	if !actions.SetEnabled || !actions.SetPriority || !actions.OpenDirectory || !actions.Export || !actions.Delete || actions.Build {
		t.Fatalf("unexpected Go-provided package actions: %#v", actions)
	}
	presentation := afterReload.Packages[0].Presentation
	if presentation.StatusTitle == "" || presentation.StatusDetail == "" || !presentation.IsPending {
		t.Fatalf("unexpected Go-provided package presentation: %#v", presentation)
	}
	if _, err := os.Stat(controller.configPath); err != nil {
		t.Fatalf("Go state was not persisted: %v", err)
	}

	if err := controller.SetPackageEnabled("new-package", true); err != nil {
		t.Fatal(err)
	}
	afterEnable := controller.Snapshot()
	if containsString(afterEnable.DisabledPackageIDs, "new-package") || afterEnable.EnabledPackageCount != 1 {
		t.Fatalf("enabled package snapshot is inconsistent: %#v", afterEnable.DisabledPackageIDs)
	}
	if afterEnable.Packages[0].Presentation.StatusTitle == PresentationText()["packages.status.disabled"] {
		t.Fatalf("enabled package still has disabled presentation: %#v", afterEnable.Packages[0].Presentation)
	}

	if err := controller.SetPackageEnabled("new-package", false); err != nil {
		t.Fatal(err)
	}
	afterDisable := controller.Snapshot()
	if !containsString(afterDisable.DisabledPackageIDs, "new-package") || afterDisable.EnabledPackageCount != 0 {
		t.Fatalf("disabled package snapshot is inconsistent: %#v", afterDisable.DisabledPackageIDs)
	}
}

func TestControllerDeletesLocalPackageSourceBuildAndSettings(t *testing.T) {
	root := t.TempDir()
	events := make(chan AppSnapshot, 16)
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "1",
		},
		func(snapshot AppSnapshot) { events <- snapshot },
		ControllerDependencies{Platform: idleControllerTestPlatform{}, DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()

	directory := makeStorePackage(t, controller.store, "delete-sample", "delete-sample", "1.0.0", 12, nil, "")
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	pkg, exists := controller.packageByID("delete-sample")
	if !exists {
		t.Fatal("test package was not loaded")
	}
	activateTestBuild(t, controller.store, pkg, "renderer", "")
	priority := -4
	if err := controller.store.SetPriorityOverride(pkg.ID, &priority); err != nil {
		t.Fatal(err)
	}
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	if !controller.Snapshot().Packages[0].AvailableActions.Delete {
		t.Fatal("delete action was not available")
	}

	if err := controller.DeletePackage(pkg.ID); err != nil {
		t.Fatal(err)
	}
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case snapshot := <-events:
			if snapshot.LocalOperationError != nil {
				t.Fatalf("package deletion failed: %s", *snapshot.LocalOperationError)
			}
			if snapshot.LocalOperationMessage == nil || !strings.Contains(*snapshot.LocalOperationMessage, "delete-sample") {
				continue
			}
			if len(snapshot.Packages) != 0 {
				t.Fatalf("deleted package remains in snapshot: %#v", snapshot.Packages)
			}
			if !containsString(snapshot.DisabledPackageIDs, pkg.ID) {
				t.Fatalf("deleted package was not kept disabled for safe reinstall: %#v", snapshot.DisabledPackageIDs)
			}
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("package source still exists: %v", err)
			}
			if _, err := os.Stat(controller.store.packageCacheDirectory(pkg.ID)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("package build cache still exists: %v", err)
			}
			settings, err := controller.store.LoadUserSettings()
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := settings.Packages[pkg.ID]; exists {
				t.Fatalf("package priority setting still exists: %#v", settings.Packages[pkg.ID])
			}
			return
		case <-timeout.C:
			t.Fatal("timed out waiting for package deletion")
		}
	}
}

func TestPackageViewExposesProjectPageOnlyForManagedPackages(t *testing.T) {
	lock := ManagedPackageLock{
		PackageID: "ct-example-package",
		Source: PackageSource{
			URL:      "git@github.com:example/codex-tweaks-package.git",
			Selector: NewRemoteSelector(SelectorLatestSemverTag, ""),
		},
	}
	pkg := Package{ID: lock.PackageID, DirectoryName: lock.PackageID, Origin: ManagedOrigin(lock)}
	managed := packageView(pkg, nil, nil)
	if managed.ProjectPageURL == nil || *managed.ProjectPageURL != "https://github.com/example/codex-tweaks-package" {
		t.Fatalf("managed project page = %#v", managed.ProjectPageURL)
	}

	pkg.Origin = LocalOrigin()
	local := packageView(pkg, nil, nil)
	if local.ProjectPageURL != nil {
		t.Fatalf("local package exposed a project page: %q", *local.ProjectPageURL)
	}
}

func TestDeveloperNodeAutomaticTrustNeverPersistsAcrossBackendRestart(t *testing.T) {
	root := t.TempDir()
	params := InitializeParams{
		ApplicationSupportDirectory: filepath.Join(root, "support"),
		CacheDirectory:              filepath.Join(root, "cache"),
		CurrentVersion:              "0.1.0",
		BuildNumber:                 "1",
	}
	controller, err := NewController(params, nil, ControllerDependencies{DisableBackground: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.SetDeveloperMode(true); err != nil {
		t.Fatal(err)
	}
	if err := controller.SetDeveloperAllowUnknownNode(true); err != nil {
		t.Fatal(err)
	}
	if snapshot := controller.Snapshot(); !snapshot.DeveloperMode || !snapshot.DeveloperAllowUnknownNode {
		t.Fatalf("developer Node trust was not enabled: %#v", snapshot)
	}
	controller.nodeRuntime.StopAll()
	controller.cancel()

	restarted, err := NewController(params, nil, ControllerDependencies{DisableBackground: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.cancel()
	if snapshot := restarted.Snapshot(); !snapshot.DeveloperMode || snapshot.DeveloperAllowUnknownNode {
		t.Fatalf("automatic Node trust persisted across restart: %#v", snapshot)
	}
}

func TestGlobalMasterSwitchBlocksNodeRuntimeEnvironment(t *testing.T) {
	controller := &Controller{
		config:          AppConfiguration{Enabled: false},
		nodeEnvironment: &NodeEnvironment{NodePath: "/node", NPMPath: "/npm", NPXPath: "/npx"},
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if environment := controller.enabledNodeEnvironmentLocked(); environment != nil {
		t.Fatalf("disabled master switch exposed a Node runtime environment: %#v", environment)
	}
	controller.config.Enabled = true
	if environment := controller.enabledNodeEnvironmentLocked(); environment == nil || environment.NodePath != "/node" {
		t.Fatalf("enabled master switch lost the Node runtime environment: %#v", environment)
	}
}

func TestControllerRevokesExplicitNodeAuthorizationOnDisableAndRevisionChange(t *testing.T) {
	root := t.TempDir()
	params := InitializeParams{
		ApplicationSupportDirectory: filepath.Join(root, "support"),
		CacheDirectory:              filepath.Join(root, "cache"),
		CurrentVersion:              "0.1.0",
		BuildNumber:                 "1",
	}
	controller, err := NewController(
		params,
		nil,
		ControllerDependencies{DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()
	controller.mu.Lock()
	controller.config.Enabled = false
	controller.disabledCleanupCompleted = true
	controller.mu.Unlock()

	directory := makeStorePackage(t, controller.store, "node-lifecycle", "node-lifecycle", "1.0.0", 0, nil, "")
	manifestPath := filepath.Join(directory, "package.json")
	manifest := PackageManifest{}
	if err := readJSON(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	nodeEntry := "src/node.js"
	manifest.CodexTweaks.Entrypoints.Node = &nodeEntry
	manifest.CodexTweaks.Permissions.Node = &PackageNodePermission{Reason: "读取本地测试文件。"}
	if err := os.WriteFile(filepath.Join(directory, "src", "node.js"), []byte("export function activate() {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	pkg, _ := controller.packageByID("node-lifecycle")
	activateTestBuild(t, controller.store, pkg, "renderer", "")
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}

	snapshot := controller.Snapshot()
	node := snapshot.Packages[0].Node
	if node == nil || node.AuthorizationID == "" || node.Authorized {
		t.Fatalf("unexpected initial Node authorization state: %#v", node)
	}
	if !snapshot.Packages[0].AvailableActions.AuthorizeNode {
		t.Fatal("disabled Node package did not expose the authorization flow")
	}
	if err := controller.AuthorizeNodePackage("node-lifecycle", node.AuthorizationID); err == nil {
		t.Fatal("disabled Node package accepted an authorization")
	}
	if err := controller.SetPackageEnabled("node-lifecycle", true); err != nil {
		t.Fatal(err)
	}
	if err := controller.AuthorizeNodePackage("node-lifecycle", node.AuthorizationID); err != nil {
		t.Fatal(err)
	}
	if authorized := controller.Snapshot().Packages[0].Node; authorized == nil || !authorized.ExplicitlyAuthorized {
		t.Fatalf("explicit authorization was not applied: %#v", authorized)
	}
	controller.mu.Lock()
	controller.config.Enabled = true
	controller.disabledCleanupCompleted = true
	controller.mu.Unlock()
	if err := controller.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if records := controller.nodeAuthorizations; len(records) != 1 {
		t.Fatalf("global master switch revoked explicit authorization: %#v", records)
	}
	restarted, err := NewController(params, nil, ControllerDependencies{DisableBackground: true})
	if err != nil {
		t.Fatal(err)
	}
	if restartedNode := restarted.Snapshot().Packages[0].Node; restartedNode == nil || !restartedNode.ExplicitlyAuthorized {
		t.Fatalf("explicit authorization did not survive a backend restart: %#v", restartedNode)
	}
	restarted.cancel()
	if err := controller.SetPackageEnabled("node-lifecycle", false); err != nil {
		t.Fatal(err)
	}
	if records := controller.nodeAuthorizations; len(records) != 0 {
		t.Fatalf("package disable retained explicit authorization: %#v", records)
	}
	staleRecords := map[string]NodeAuthorizationRecord{}
	authorizeNodeRecord(staleRecords, "node-lifecycle", node.AuthorizationID)
	if err := controller.store.SaveNodeAuthorizations(staleRecords); err != nil {
		t.Fatal(err)
	}
	controller.mu.Lock()
	controller.nodeAuthorizations = staleRecords
	controller.mu.Unlock()
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	if records, err := controller.store.LoadNodeAuthorizations(); err != nil || len(records) != 0 {
		t.Fatalf("disabled package revived a stale authorization: %#v, %v", records, err)
	}
	if err := controller.SetPackageEnabled("node-lifecycle", true); err != nil {
		t.Fatal(err)
	}
	if pending := controller.Snapshot().Packages[0].Node; pending == nil || pending.Authorized {
		t.Fatalf("package re-enable did not return to pending authorization: %#v", pending)
	}
	if err := controller.AuthorizeNodePackage("node-lifecycle", node.AuthorizationID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "src", "node.js"), []byte("export function activate() { return () => {}; }"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	if records := controller.nodeAuthorizations; len(records) != 0 {
		t.Fatalf("source revision change retained explicit authorization: %#v", records)
	}
}

func TestControllerKeepsManualBuildAvailableForLocalPackageChanges(t *testing.T) {
	root := t.TempDir()
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "1",
		},
		nil,
		ControllerDependencies{DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()

	directory := makeStorePackage(t, controller.store, "local-package", "local-package", "1.0.0", 0, nil, "")
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	activateTestBuild(t, controller.store, controller.packages[0], "local-js", "")
	controller.mu.Lock()
	controller.nodeEnvironment = &NodeEnvironment{Version: "v24.0.0"}
	controller.mu.Unlock()
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}

	assertBuildState := func(disposition BuildDisposition) {
		t.Helper()
		view := controller.Snapshot().Packages[0]
		if view.BuildDisposition != disposition || !view.AvailableActions.Build {
			t.Fatalf("build state = %s, available = %t; want %s and available", view.BuildDisposition, view.AvailableActions.Build, disposition)
		}
	}
	assertBuildState(BuildCurrent)

	if err := os.WriteFile(filepath.Join(directory, "src", "index.js"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	assertBuildState(BuildSourceChanged)

	var manifest PackageManifest
	manifestPath := filepath.Join(directory, "package.json")
	if err := readJSON(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Version = "1.1.0"
	if err := writeJSONAtomic(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	assertBuildState(BuildVersionUpdate)
}

func TestControllerLocalInstallEmitsSnapshotContainingPackage(t *testing.T) {
	root := t.TempDir()
	events := make(chan AppSnapshot, 8)
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "1",
		},
		func(snapshot AppSnapshot) { events <- snapshot },
		ControllerDependencies{DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()

	source := filepath.Join(root, "local-package")
	writeInstallerPackage(t, source, "local-install-sample", "1.0.0", nil)
	controller.InstallLocalPackage(source)

	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case snapshot := <-events:
			if snapshot.LocalOperationError != nil {
				t.Fatalf("local install failed: %s", *snapshot.LocalOperationError)
			}
			if snapshot.LocalOperationMessage == nil {
				continue
			}
			if len(snapshot.Packages) != 1 || snapshot.Packages[0].ID != "local-install-sample" {
				t.Fatalf("successful install snapshot does not contain package: %#v", snapshot.Packages)
			}
			if !containsString(snapshot.DisabledPackageIDs, "local-install-sample") {
				t.Fatalf("locally installed package must remain disabled: %#v", snapshot.DisabledPackageIDs)
			}
			return
		case <-timeout.C:
			t.Fatal("timed out waiting for successful local install snapshot")
		}
	}
}

func TestControllerExportsPackageAndPublishesOperationState(t *testing.T) {
	root := t.TempDir()
	events := make(chan AppSnapshot, 16)
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "1",
		},
		func(snapshot AppSnapshot) { events <- snapshot },
		ControllerDependencies{DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()

	makeStorePackage(t, controller.store, "export-sample", "export-sample", "1.2.3", 0, nil, "")
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	view := controller.Snapshot().Packages[0]
	if !view.AvailableActions.Export || view.ExportFileName != "export-sample-v1.2.3.zip" {
		t.Fatalf("export presentation is incomplete: %#v", view)
	}
	destination := filepath.Join(root, view.ExportFileName)
	if err := controller.ExportPackage(view.ID, destination); err != nil {
		t.Fatal(err)
	}

	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case snapshot := <-events:
			if snapshot.LocalOperationError != nil {
				t.Fatalf("package export failed: %s", *snapshot.LocalOperationError)
			}
			if snapshot.LocalOperationMessage == nil {
				continue
			}
			if len(snapshot.ExportingPackageIDs) != 0 {
				t.Fatalf("completed export is still marked active: %#v", snapshot.ExportingPackageIDs)
			}
			if _, err := os.Stat(destination); err != nil {
				t.Fatalf("exported ZIP is missing: %v", err)
			}
			return
		case <-timeout.C:
			t.Fatal("timed out waiting for successful package export snapshot")
		}
	}
}

func TestControllerSnapshotSerializesEmptyDependencyStatusesAsArrays(t *testing.T) {
	root := t.TempDir()
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "1",
		},
		nil,
		ControllerDependencies{DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()

	makeStorePackage(t, controller.store, "no-dependencies", "no-dependencies", "1.0.0", 10, nil, "")
	if err := controller.ReloadPackages(); err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	statuses, exists := snapshot.PackageDependencyStatuses["no-dependencies"]
	if !exists || statuses == nil {
		t.Fatalf("empty dependency statuses must be a non-nil array: %#v", snapshot.PackageDependencyStatuses)
	}
	encoded, err := json.Marshal(snapshot.PackageDependencyStatuses)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "null") {
		t.Fatalf("dependency status arrays must not serialize as null: %s", encoded)
	}
}

func TestControllerDoesNotEmitUnchangedSnapshots(t *testing.T) {
	root := t.TempDir()
	eventCount := 0
	controller, err := NewController(
		InitializeParams{
			ApplicationSupportDirectory: filepath.Join(root, "support"),
			CacheDirectory:              filepath.Join(root, "cache"),
			CurrentVersion:              "0.1.0",
			BuildNumber:                 "1",
		},
		func(AppSnapshot) { eventCount++ },
		ControllerDependencies{DisableBackground: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.cancel()

	if eventCount != 1 {
		t.Fatalf("initial event count = %d, want 1", eventCount)
	}
	controller.emit()
	controller.emit()
	if eventCount != 1 {
		t.Fatalf("unchanged event count = %d, want 1", eventCount)
	}

	controller.mu.Lock()
	controller.status = AppStatus{Kind: StatusDisabled}
	controller.mu.Unlock()
	controller.emit()
	if eventCount != 2 {
		t.Fatalf("changed event count = %d, want 2", eventCount)
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
