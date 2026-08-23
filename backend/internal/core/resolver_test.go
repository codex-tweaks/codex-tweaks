package core

import (
	"strings"
	"testing"
	"time"
)

func TestDependencyTopologyOverridesPriority(t *testing.T) {
	dependency := testPackage("foundation", "1.0.0", 100, intPointer(500), nil, true, LocalOrigin())
	consumer := testPackage("consumer", "1.0.0", 0, intPointer(-500), map[string]PackageDependency{
		"foundation": {Version: "^1.0.0"},
	}, true, LocalOrigin())

	result := ResolveDependencies([]Package{consumer, dependency}, map[string]bool{})
	assertPackageIDs(t, result.OrderedPackages, []string{"foundation", "consumer"})
	assertPackageIDs(t, result.LoadablePackages, []string{"foundation", "consumer"})
	if len(result.IssuesByPackageID) != 0 {
		t.Fatalf("unexpected issues: %#v", result.IssuesByPackageID)
	}
	if got := result.PriorityConstraintsByPackageID["consumer"].MustLoadAfterPackageIDs; len(got) != 1 || got[0] != "foundation" {
		t.Fatalf("unexpected consumer constraint: %#v", got)
	}
}

func TestMissingDependencyAndCycleAreIsolated(t *testing.T) {
	affected := testPackage("affected", "1.0.0", 0, nil, map[string]PackageDependency{"missing": {Version: "^1.0.0"}}, true, LocalOrigin())
	first := testPackage("first", "1.0.0", 0, nil, map[string]PackageDependency{"second": {Version: "1.x"}}, true, LocalOrigin())
	second := testPackage("second", "1.0.0", 0, nil, map[string]PackageDependency{"first": {Version: "1.x"}}, true, LocalOrigin())
	healthy := testPackage("healthy", "1.0.0", 10, nil, nil, true, LocalOrigin())

	result := ResolveDependencies([]Package{affected, first, second, healthy}, map[string]bool{})
	assertPackageIDs(t, result.LoadablePackages, []string{"healthy"})
	if !result.CyclePackageIDs["first"] || !result.CyclePackageIDs["second"] {
		t.Fatalf("cycle not detected: %#v", result.CyclePackageIDs)
	}
	if !strings.Contains(result.IssuesByPackageID["affected"][0], "缺少依赖 missing") {
		t.Fatalf("missing dependency issue not preserved: %#v", result.IssuesByPackageID)
	}
}

func TestDisabledUnbuiltVersionAndSourceConflicts(t *testing.T) {
	source := PackageSource{URL: "https://github.com/example/declared.git", Selector: NewRemoteSelector(SelectorBranch, "main")}
	installedSource := PackageSource{URL: "https://github.com/example/installed.git", Selector: NewRemoteSelector(SelectorBranch, "main")}
	lock := ManagedPackageLock{PackageID: "dependency", PackageVersion: "1.0.0", Source: installedSource, InstalledAt: NewCodableTime(time.Unix(0, 0))}
	dependency := testPackage("dependency", "1.0.0", 0, nil, nil, true, ManagedOrigin(lock))
	consumer := testPackage("consumer", "1.0.0", 0, nil, map[string]PackageDependency{
		"dependency": {Version: "^2.0.0", Source: &source},
	}, true, LocalOrigin())

	conflict := ResolveDependencies([]Package{consumer, dependency}, map[string]bool{})
	if got := conflict.DependenciesByPackageID["consumer"][0].State.Kind; got != DependencySourceConflict {
		t.Fatalf("got %s, want source conflict", got)
	}

	consumer.Manifest.CodexTweaks.PackageDependencies["dependency"] = PackageDependency{Version: "^2.0.0"}
	disabled := ResolveDependencies([]Package{consumer, dependency}, map[string]bool{"dependency": true})
	if got := disabled.DependenciesByPackageID["consumer"][0].State.Kind; got != DependencyDisabled {
		t.Fatalf("got %s, want disabled", got)
	}

	unbuiltDependency := testPackage("dependency", "1.0.0", 0, nil, nil, false, LocalOrigin())
	unbuilt := ResolveDependencies([]Package{consumer, unbuiltDependency}, map[string]bool{})
	if got := unbuilt.DependenciesByPackageID["consumer"][0].State.Kind; got != DependencyNotBuilt {
		t.Fatalf("got %s, want not built", got)
	}

	mismatch := ResolveDependencies([]Package{consumer, dependency}, map[string]bool{})
	if got := mismatch.DependenciesByPackageID["consumer"][0].State.Kind; got != DependencyVersionMismatch {
		t.Fatalf("got %s, want version mismatch", got)
	}
}

func TestEnablementAndFingerprintContracts(t *testing.T) {
	first := ReconcileEnablement(map[string]bool{"enabled": true, "disabled": true}, nil, map[string]bool{"disabled": true}, false)
	if first.DisabledPackageIDs["enabled"] || !first.DisabledPackageIDs["disabled"] || len(first.NewlyDiscoveredPackageIDs) != 0 {
		t.Fatalf("unexpected first reconciliation: %#v", first)
	}
	second := ReconcileEnablement(map[string]bool{"existing": true, "new": true}, map[string]bool{"existing": true}, map[string]bool{}, true)
	if !second.DisabledPackageIDs["new"] || !second.NewlyDiscoveredPackageIDs["new"] {
		t.Fatalf("new package should default disabled: %#v", second)
	}
	if FingerprintString("same") != "097b5e18bf93ef5b" {
		t.Fatalf("FNV-1a output changed: %s", FingerprintString("same"))
	}
}

func testPackage(id, version string, priority int, override *int, dependencies map[string]PackageDependency, built bool, origin PackageOrigin) Package {
	if dependencies == nil {
		dependencies = map[string]PackageDependency{}
	}
	manifest := &PackageManifest{
		Name: id, Version: version, Description: id, Dependencies: map[string]string{},
		CodexTweaks: PackageConfiguration{APIVersion: 2, Entry: "src/index.js", Priority: priority, PackageDependencies: dependencies},
	}
	source, dependencyFingerprint := "source-"+id, "dependencies-"+id
	pkg := Package{ID: id, DirectoryName: id, Directory: "/tmp/" + id, Manifest: manifest, SourceFingerprint: &source, DependencyFingerprint: &dependencyFingerprint, PriorityOverride: override, Origin: origin}
	if built {
		runtimeDependencies := map[string]string{}
		for dependencyID, dependency := range dependencies {
			runtimeDependencies[dependencyID] = dependency.Version
		}
		pkg.ActiveBuild = &ActivePackageBuild{Record: PackageBuildRecord{
			PackageID: id, PackageVersion: version, PackageDependencies: runtimeDependencies,
			SourceFingerprint: source, DependencyFingerprint: dependencyFingerprint,
			CompilerVersion: CompilerVersion, NodeVersion: "v24.0.0", BuildDirectoryName: "build", BuiltAt: NewCodableTime(time.Unix(0, 0)),
		}, OutputDirectory: "/tmp/" + id}
	}
	return pkg
}

func intPointer(value int) *int { return &value }

func assertPackageIDs(t *testing.T, packages []Package, want []string) {
	t.Helper()
	if len(packages) != len(want) {
		t.Fatalf("package IDs length = %d, want %d: %#v", len(packages), len(want), packages)
	}
	for index := range want {
		if packages[index].ID != want[index] {
			t.Fatalf("package ID %d = %q, want %q", index, packages[index].ID, want[index])
		}
	}
}
