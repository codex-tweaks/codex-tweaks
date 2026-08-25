package core

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	payload := store.LoadPayload(packages, map[string]bool{})
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

func TestStoreCarriesCapabilitiesWithoutChangingPackageAPIVersion(t *testing.T) {
	store, _ := newTestStore(t)
	directory := makeStorePackage(t, store, "capable", "capable", "1.0.0", 0, nil, "")
	manifest := PackageManifest{}
	manifestPath := filepath.Join(directory, "package.json")
	if err := readJSON(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.CodexTweaks.Capabilities = map[string]CapabilityRequirement{
		NetworkCapabilityID: {
			Version:     "^1.0.0",
			Permissions: json.RawMessage(`{"origins":["https://example.com"]}`),
		},
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
		t.Fatalf("capability manifest failed: %v %#v", err, packages)
	}
	if packages[0].Manifest.CodexTweaks.APIVersion != APIVersion {
		t.Fatalf("package API version changed: %d", packages[0].Manifest.CodexTweaks.APIVersion)
	}
	activateTestBuild(t, store, packages[0], "capable-js", "")
	packages, _ = store.LoadPackages()
	payload := store.LoadPayload(packages, map[string]bool{})
	grant, ok := payload.Payload.Packages[0].Capabilities[NetworkCapabilityID]
	if !ok || grant.Version != NetworkCapabilityVersion {
		t.Fatalf("capability grant missing from payload: %#v", payload)
	}

	initialFingerprint := *packages[0].SourceFingerprint
	initialDependencyFingerprint := *packages[0].DependencyFingerprint
	manifest.CodexTweaks.Capabilities[NetworkCapabilityID] = CapabilityRequirement{
		Version:     "^1.0.0",
		Permissions: json.RawMessage(`{"origins":["https://changed.example"]}`),
	}
	data, _ = json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	packages, _ = store.LoadPackages()
	if *packages[0].SourceFingerprint == initialFingerprint ||
		packages[0].BuildDisposition(CompilerVersion) != BuildSourceChanged {
		t.Fatalf("capability change did not invalidate build: %#v", packages[0])
	}
	if *packages[0].DependencyFingerprint != initialDependencyFingerprint {
		t.Fatalf("capability change was classified as a dependency update: %#v", packages[0])
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
	manifest := PackageManifest{Name: name, Version: version, Description: name, Type: &typeValue, Dependencies: dependencies, CodexTweaks: PackageConfiguration{APIVersion: 2, Entry: "src/index.js", Priority: priority, PackageDependencies: map[string]PackageDependency{}}}
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
	record := PackageBuildRecord{PackageID: pkg.ID, PackageVersion: pkg.Manifest.Version, PackageDependencies: map[string]string{}, Capabilities: cloneCapabilityRequirements(pkg.Manifest.CodexTweaks.Capabilities), SourceFingerprint: *pkg.SourceFingerprint, DependencyFingerprint: *pkg.DependencyFingerprint, CompilerVersion: CompilerVersion, NodeVersion: "v24", BuildDirectoryName: "test-build", HasCSS: css != "", BuiltAt: NewCodableTime(time.Now())}
	if err := store.ActivateBuild(record); err != nil {
		t.Fatal(err)
	}
}
