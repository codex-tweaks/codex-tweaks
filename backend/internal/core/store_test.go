package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreLoadsByPriorityAndIsolatesInvalidAndDuplicatePackages(t *testing.T) {
	store, root := newTestStore(t)
	makeStorePackage(t, store, "zeta", "zeta", "1.0.0", 20, nil, "")
	makeStorePackage(t, store, "beta", "duplicate", "1.0.0", 10, nil, "")
	makeStorePackage(t, store, "alpha", "duplicate", "1.0.0", 10, nil, "")
	broken := filepath.Join(store.PackagesDirectory, "broken")
	if err := os.MkdirAll(broken, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "package.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	packages, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 4 {
		t.Fatalf("got %d packages", len(packages))
	}
	valid := []string{}
	duplicates := 0
	for _, pkg := range packages {
		if pkg.ValidationError == nil {
			valid = append(valid, pkg.ID)
		}
		if pkg.Manifest != nil && pkg.Manifest.Name == "duplicate" && pkg.ValidationError != nil {
			duplicates++
		}
	}
	if len(valid) != 1 || valid[0] != "zeta" || duplicates != 2 {
		t.Fatalf("unexpected packages: %#v", packages)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
}

func TestStoreBuildDispositionAndPayload(t *testing.T) {
	store, _ := newTestStore(t)
	directory := makeStorePackage(t, store, "demo", "demo", "1.0.0", 0, map[string]string{"example": "1.0.0"}, "lock-v1")
	packages, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages[0]
	if got := pkg.BuildDisposition(CompilerVersion); got != BuildNotBuilt {
		t.Fatalf("got %s", got)
	}
	activateTestBuild(t, store, pkg, "demo-js", "demo-css")
	packages, _ = store.LoadPackages()
	pkg = packages[0]
	if got := pkg.BuildDisposition(CompilerVersion); got != BuildCurrent {
		t.Fatalf("got %s", got)
	}
	payload := store.LoadPayload(packages, map[string]bool{}, map[string]bool{})
	if len(payload.Payload.Packages) != 1 || payload.Payload.Packages[0].JavaScript != "demo-js" || payload.Payload.Packages[0].CSS != "demo-css" {
		t.Fatalf("unexpected payload: %#v", payload)
	}

	if err := os.WriteFile(filepath.Join(directory, "src", "index.js"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	packages, _ = store.LoadPackages()
	if got := packages[0].BuildDisposition(CompilerVersion); got != BuildSourceChanged {
		t.Fatalf("got %s", got)
	}
	if err := os.WriteFile(filepath.Join(directory, "package-lock.json"), []byte("lock-v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	packages, _ = store.LoadPackages()
	if got := packages[0].BuildDisposition(CompilerVersion); got != BuildDependencyUpdate {
		t.Fatalf("got %s", got)
	}
}

func TestStorePriorityOverrideIsSeparateAndNormalized(t *testing.T) {
	store, _ := newTestStore(t)
	directory := makeStorePackage(t, store, "priority", "priority", "1.0.0", 42, nil, "")
	priority := -7
	if err := store.SetPriorityOverride("priority", &priority); err != nil {
		t.Fatal(err)
	}
	packages, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	if packages[0].Priority() != -7 || packages[0].DeclaredPriority() != 42 {
		t.Fatalf("override not applied: %#v", packages[0])
	}
	var manifest PackageManifest
	if err := readJSON(filepath.Join(directory, "package.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.CodexTweaks.Priority != 42 {
		t.Fatal("manifest was mutated")
	}
	matching := 42
	if err := store.SetPriorityOverride("priority", &matching); err != nil {
		t.Fatal(err)
	}
	packages, _ = store.LoadPackages()
	if packages[0].PriorityOverride != nil {
		t.Fatal("matching override should be removed")
	}
}

func TestStoreDeletedBundledPackageDoesNotReturnOnReload(t *testing.T) {
	root := t.TempDir()
	bundledDirectory := filepath.Join(root, "Bundled", "packages")
	bundledStore := &Store{PackagesDirectory: bundledDirectory}
	makeStorePackage(t, bundledStore, "bundled-sample", "bundled-sample", "1.0.0", 0, nil, "")
	store, err := NewStore(
		filepath.Join(root, "Application Support"),
		filepath.Join(root, "Caches"),
		bundledDirectory,
	)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := store.LoadPackages()
	if err != nil || len(packages) != 1 {
		t.Fatalf("bundled package was not seeded: %v %#v", err, packages)
	}
	if err := store.DeleteLocalPackage(packages[0]); err != nil {
		t.Fatal(err)
	}
	packages, err = store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 0 {
		t.Fatalf("deleted bundled package returned: %#v", packages)
	}
	if _, err := os.Stat(filepath.Join(store.PackagesDirectory, "bundled-sample")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted bundled package directory still exists: %v", err)
	}
}

func TestStoreRejectsDeletingOutsidePackagesDirectory(t *testing.T) {
	store, root := newTestStore(t)
	directory := filepath.Join(root, "outside")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	err := store.DeleteLocalPackage(Package{
		ID: "outside", DirectoryName: "outside", Directory: directory, Origin: LocalOrigin(),
	})
	if err == nil {
		t.Fatal("outside directory was accepted for deletion")
	}
	if _, statErr := os.Stat(directory); statErr != nil {
		t.Fatalf("outside directory was changed: %v", statErr)
	}
}

func TestStoreCarriesNodeAndUIDeclarationsAndInvalidatesManifestRevision(t *testing.T) {
	store, _ := newTestStore(t)
	directory := makeStorePackage(t, store, "node-package", "node-package", "1.0.0", 0, nil, "")
	manifest := PackageManifest{}
	manifestPath := filepath.Join(directory, "package.json")
	if err := readJSON(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	nodeEntry := "src/node.js"
	manifest.CodexTweaks.Entrypoints.Node = &nodeEntry
	manifest.CodexTweaks.Permissions.Node = &PackageNodePermission{Reason: "读取用户选择的文件并生成缩略图。"}
	manifest.CodexTweaks.UI.SettingsSections = &SettingsSectionsExtension{
		APIVersion: 1,
		Items:      []UISettingsSectionDeclaration{{ID: "appearance-extra", Title: "扩展外观"}},
	}
	if err := os.WriteFile(filepath.Join(directory, "src", "node.js"), []byte("export function activate() {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	packages, err := store.LoadPackages()
	if err != nil || len(packages) != 1 || packages[0].ValidationError != nil {
		t.Fatalf("Node manifest failed: %v %#v", err, packages)
	}
	if packages[0].Manifest.CodexTweaks.APIVersion != APIVersion {
		t.Fatalf("package API version changed: %d", packages[0].Manifest.CodexTweaks.APIVersion)
	}
	activateTestBuild(t, store, packages[0], "node-js", "")
	packages, _ = store.LoadPackages()
	if payload := store.LoadPayload(packages, map[string]bool{}, map[string]bool{}); len(payload.Payload.Packages) != 0 {
		t.Fatalf("unauthorized Node package entered payload: %#v", payload)
	}
	payload := store.LoadPayload(packages, map[string]bool{}, map[string]bool{"node-package": true})
	if len(payload.Payload.Packages) != 1 || payload.Payload.Packages[0].Node == nil ||
		payload.Payload.Packages[0].UI.SettingsSections == nil {
		t.Fatalf("Node/UI declarations missing from payload: %#v", payload)
	}

	initialFingerprint := *packages[0].SourceFingerprint
	initialDependencyFingerprint := *packages[0].DependencyFingerprint
	manifest.CodexTweaks.Permissions.Node.Reason = "用途说明已经变化，必须重新授权。"
	data, _ = json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	packages, _ = store.LoadPackages()
	if *packages[0].SourceFingerprint != initialFingerprint ||
		packages[0].BuildDisposition(CompilerVersion) != BuildSourceChanged {
		t.Fatalf("manifest change did not invalidate build independently: %#v", packages[0])
	}
	if *packages[0].DependencyFingerprint != initialDependencyFingerprint {
		t.Fatalf("manifest change was classified as a dependency update: %#v", packages[0])
	}
	if err := os.WriteFile(packages[0].ActiveBuild.JavaScriptPath(), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	tamperedPackages, _ := store.LoadPackages()
	if tamperedPackages[0].ActiveBuild != nil {
		t.Fatal("changed Renderer bundle remained an active executable build")
	}
	if err := os.WriteFile(packages[0].ActiveBuild.JavaScriptPath(), []byte("node-js"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packages[0].ActiveBuild.NodeJavaScriptPath(), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	packages, _ = store.LoadPackages()
	if packages[0].ActiveBuild != nil {
		t.Fatal("changed Node bundle remained an active executable build")
	}
}

func TestStoreRejectsLegacyAndIncompleteNodeDeclarations(t *testing.T) {
	store, _ := newTestStore(t)
	tests := []struct {
		name      string
		mutate    func(map[string]any)
		wantError string
		needsNode bool
	}{
		{
			name: "missing API version",
			mutate: func(configuration map[string]any) {
				delete(configuration, "apiVersion")
			},
			wantError: "API v0",
		},
		{
			name: "legacy entry",
			mutate: func(configuration map[string]any) {
				delete(configuration, "entrypoints")
				configuration["entry"] = "src/index.js"
			},
			wantError: "不能使用 entry",
		},
		{
			name: "legacy capabilities",
			mutate: func(configuration map[string]any) {
				configuration["capabilities"] = map[string]any{"network": map[string]any{}}
			},
			wantError: "已移除 codexTweaks.capabilities",
		},
		{
			name: "unknown UI extension",
			mutate: func(configuration map[string]any) {
				configuration["ui"] = map[string]any{"generic": map[string]any{}}
			},
			wantError: "codexTweaks.ui 包含不支持的字段",
		},
		{
			name: "Node entry without permission",
			mutate: func(configuration map[string]any) {
				configuration["entrypoints"].(map[string]any)["node"] = "src/node.js"
			},
			wantError: "必须同时声明",
			needsNode: true,
		},
		{
			name: "blank Node reason",
			mutate: func(configuration map[string]any) {
				configuration["entrypoints"].(map[string]any)["node"] = "src/node.js"
				configuration["permissions"] = map[string]any{
					"node": map[string]any{"reason": "  "},
				}
			},
			wantError: "reason 必须为 1 到 1000",
			needsNode: true,
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directoryName := fmt.Sprintf("invalid-%d", index)
			directory := makeStorePackage(t, store, directoryName, directoryName, "1.0.0", 0, nil, "")
			if test.needsNode {
				if err := os.WriteFile(filepath.Join(directory, "src", "node.js"), []byte("export function activate() {}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			manifest := map[string]any{}
			manifestPath := filepath.Join(directory, "package.json")
			if err := readJSON(manifestPath, &manifest); err != nil {
				t.Fatal(err)
			}
			test.mutate(manifest["codexTweaks"].(map[string]any))
			contents, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, contents, 0o600); err != nil {
				t.Fatal(err)
			}
			pkg := store.InspectPackage(directory, LocalOrigin(), nil, "")
			if pkg.ValidationError == nil || !strings.Contains(*pkg.ValidationError, test.wantError) {
				t.Fatalf("validation error = %v, want %q", pkg.ValidationError, test.wantError)
			}
		})
	}
}

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "Application Support"), filepath.Join(root, "Caches"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	return store, root
}

func makeStorePackage(t *testing.T, store *Store, directory, name, version string, priority int, dependencies map[string]string, lockfile string) string {
	t.Helper()
	packageDirectory := filepath.Join(store.PackagesDirectory, directory)
	if err := os.MkdirAll(filepath.Join(packageDirectory, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDirectory, "src", "index.js"), []byte("export function activate() {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	typeValue := "module"
	manifest := PackageManifest{Name: name, Version: version, Description: name, Type: &typeValue, Dependencies: dependencies, CodexTweaks: PackageConfiguration{APIVersion: APIVersion, Entrypoints: PackageEntrypoints{Renderer: "src/index.js"}, Priority: priority, PackageDependencies: map[string]PackageDependency{}}}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDirectory, "package.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if lockfile != "" {
		if err := os.WriteFile(filepath.Join(packageDirectory, "package-lock.json"), []byte(lockfile), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return packageDirectory
}

func activateTestBuild(t *testing.T, store *Store, pkg Package, javascript, css string) {
	t.Helper()
	builds, err := store.BuildsDirectory(pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	buildDirectory := filepath.Join(builds, "test-build")
	if err := os.MkdirAll(buildDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildDirectory, "bundle.js"), []byte(javascript), 0o600); err != nil {
		t.Fatal(err)
	}
	if css != "" {
		if err := os.WriteFile(filepath.Join(buildDirectory, "bundle.css"), []byte(css), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	hasNode := pkg.Manifest.CodexTweaks.Entrypoints.Node != nil
	cssFingerprint := ""
	if css != "" {
		cssFingerprint = SecureFingerprintString(css)
	}
	nodeBundleFingerprint := ""
	if hasNode {
		nodeBundle := []byte("module.exports.activate = () => {}")
		if err := os.WriteFile(filepath.Join(buildDirectory, "node-bundle.cjs"), nodeBundle, 0o600); err != nil {
			t.Fatal(err)
		}
		nodeBundleFingerprint = SecureFingerprintBytes(nodeBundle)
	}
	record := PackageBuildRecord{
		PackageID: pkg.ID, PackageVersion: pkg.Manifest.Version,
		PackageDependencies: map[string]string{}, Entrypoints: pkg.Manifest.CodexTweaks.Entrypoints,
		NodePermission: pkg.Manifest.CodexTweaks.Permissions.Node, UI: pkg.Manifest.CodexTweaks.UI,
		ManifestFingerprint: *pkg.ManifestFingerprint, SourceFingerprint: *pkg.SourceFingerprint,
		DependencyFingerprint: *pkg.DependencyFingerprint,
		RendererFingerprint:   SecureFingerprintString(javascript),
		CSSFingerprint:        cssFingerprint, NodeBundleFingerprint: nodeBundleFingerprint,
		CompilerVersion: CompilerVersion, NodeVersion: "v24", BuildDirectoryName: "test-build",
		HasCSS: css != "", HasNode: hasNode, BuiltAt: NewCodableTime(time.Now()),
	}
	if err := store.ActivateBuild(record); err != nil {
		t.Fatal(err)
	}
}
