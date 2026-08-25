package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNodeAuthorizationIDBindsEveryExecutableRevisionInput(t *testing.T) {
	reason := "读取本地文件。"
	nodeEntry := "src/node.js"
	pkg := Package{
		ID: "node-test",
		Manifest: &PackageManifest{
			Name: "node-test", Version: "1.0.0",
			CodexTweaks: PackageConfiguration{
				APIVersion:  APIVersion,
				Entrypoints: PackageEntrypoints{Renderer: "src/index.js", Node: &nodeEntry},
				Permissions: PackagePermissions{Node: &PackageNodePermission{Reason: reason}},
			},
		},
	}
	manifestFingerprint := "manifest"
	sourceFingerprint := "source"
	dependencyFingerprint := "dependency"
	pkg.ManifestFingerprint = &manifestFingerprint
	pkg.SourceFingerprint = &sourceFingerprint
	pkg.DependencyFingerprint = &dependencyFingerprint
	pkg.ActiveBuild = &ActivePackageBuild{Record: PackageBuildRecord{
		PackageID: "node-test", PackageVersion: "1.0.0",
		Entrypoints:         PackageEntrypoints{Renderer: "src/index.js", Node: &nodeEntry},
		NodePermission:      &PackageNodePermission{Reason: reason},
		ManifestFingerprint: manifestFingerprint, SourceFingerprint: sourceFingerprint,
		DependencyFingerprint: dependencyFingerprint, RendererFingerprint: "renderer", CSSFingerprint: "css",
		NodeBundleFingerprint: "bundle",
		CompilerVersion:       CompilerVersion, HasNode: true,
	}}

	initial := NodeAuthorizationID(pkg)
	if initial == "" {
		t.Fatal("current Node package did not receive an authorization ID")
	}
	pkg.ActiveBuild.Record.NodePermission.Reason = "用途发生变化。"
	if changed := NodeAuthorizationID(pkg); changed == initial || changed == "" {
		t.Fatalf("reason did not change authorization ID: %q", changed)
	}
	pkg.ActiveBuild.Record.NodePermission.Reason = reason
	pkg.ActiveBuild.Record.RendererFingerprint = "updated-renderer"
	if changed := NodeAuthorizationID(pkg); changed == initial || changed == "" {
		t.Fatalf("Renderer bundle did not change authorization ID: %q", changed)
	}
	pkg.ActiveBuild.Record.RendererFingerprint = "renderer"
	pkg.ActiveBuild.Record.NodeBundleFingerprint = "updated-bundle"
	if changed := NodeAuthorizationID(pkg); changed == initial || changed == "" {
		t.Fatalf("Node bundle did not change authorization ID: %q", changed)
	}
	pkg.Origin = ManagedOrigin(ManagedPackageLock{PackageID: pkg.ID, ResolvedCommit: "commit-a"})
	managedRevision := NodeAuthorizationID(pkg)
	pkg.Origin.Lock.ResolvedCommit = "commit-b"
	if changed := NodeAuthorizationID(pkg); changed == managedRevision || changed == "" {
		t.Fatalf("managed package update did not change authorization ID: %q", changed)
	}
}

func TestSecureFingerprintStringsFramesVariableLengthValues(t *testing.T) {
	left := SecureFingerprintStrings("a", "\x00b")
	right := SecureFingerprintStrings("a\x00", "b")
	if left == right {
		t.Fatalf("length framing did not distinguish ambiguous values: %s", left)
	}
}

func TestNodeAuthorizationStorePersistsOnlyExplicitRecords(t *testing.T) {
	store, _ := newTestStore(t)
	records := map[string]NodeAuthorizationRecord{}
	authorizeNodeRecord(records, "sample", "revision")
	if err := store.SaveNodeAuthorizations(records); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadNodeAuthorizations()
	if err != nil {
		t.Fatal(err)
	}
	if loaded["sample"].AuthorizationID != "revision" || loaded["sample"].AuthorizedAt.Time.IsZero() {
		t.Fatalf("authorization was not persisted: %#v", loaded)
	}
}

func TestNodeRuntimeReconcileClearsFailureWhenTrustIsRemoved(t *testing.T) {
	store, _ := newTestStore(t)
	supervisor := NewNodeRuntimeSupervisor(store, nil, nil)
	supervisor.failures["sample"] = nodeRuntimeFailure{
		AuthorizationID: "revision",
		Message:         "failed",
	}
	supervisor.Reconcile(context.Background(), nil, map[string]bool{}, map[string]string{}, nil)
	if failures := supervisor.RuntimeErrors(); len(failures) != 0 {
		t.Fatalf("runtime failures survived trust removal: %#v", failures)
	}
}

func TestDesiredNodeRuntimesBlockDependentsOfUnauthorizedOrFailedNodePackages(t *testing.T) {
	dependency := nodeRevisionTestPackage("dependency", nil)
	dependent := nodeRevisionTestPackage("dependent", map[string]string{"dependency": "^1.0.0"})
	packages := []Package{dependency, dependent}

	desired := desiredNodeRuntimePackages(
		packages,
		map[string]bool{},
		map[string]string{"dependent": NodeTrustExplicit},
		map[string]bool{},
	)
	if len(desired) != 0 {
		t.Fatalf("dependent runtime survived an unauthorized Node dependency: %#v", desired)
	}

	desired = desiredNodeRuntimePackages(
		packages,
		map[string]bool{},
		map[string]string{"dependency": NodeTrustExplicit, "dependent": NodeTrustExplicit},
		map[string]bool{},
	)
	if len(desired) != 2 {
		t.Fatalf("authorized Node dependency graph did not become runnable: %#v", desired)
	}

	desired = desiredNodeRuntimePackages(
		packages,
		map[string]bool{},
		map[string]string{"dependency": NodeTrustExplicit, "dependent": NodeTrustExplicit},
		map[string]bool{"dependency": true},
	)
	if len(desired) != 0 {
		t.Fatalf("dependent runtime survived a failed Node dependency: %#v", desired)
	}
}

func nodeRevisionTestPackage(id string, dependencies map[string]string) Package {
	nodeEntry := "src/node.js"
	manifestFingerprint := "manifest-" + id
	sourceFingerprint := "source-" + id
	dependencyFingerprint := "dependencies-" + id
	packageDependencies := map[string]PackageDependency{}
	for dependencyID, version := range dependencies {
		packageDependencies[dependencyID] = PackageDependency{Version: version}
	}
	permission := &PackageNodePermission{Reason: "测试 Node runtime 依赖隔离。"}
	return Package{
		ID: id,
		Manifest: &PackageManifest{
			Name: id, Version: "1.0.0",
			CodexTweaks: PackageConfiguration{
				APIVersion:          APIVersion,
				Entrypoints:         PackageEntrypoints{Renderer: "src/index.js", Node: &nodeEntry},
				PackageDependencies: packageDependencies,
				Permissions:         PackagePermissions{Node: permission},
			},
		},
		ManifestFingerprint:   &manifestFingerprint,
		SourceFingerprint:     &sourceFingerprint,
		DependencyFingerprint: &dependencyFingerprint,
		ActiveBuild: &ActivePackageBuild{Record: PackageBuildRecord{
			PackageID: id, PackageVersion: "1.0.0", PackageDependencies: dependencies,
			Entrypoints:    PackageEntrypoints{Renderer: "src/index.js", Node: &nodeEntry},
			NodePermission: permission, ManifestFingerprint: manifestFingerprint,
			SourceFingerprint: sourceFingerprint, DependencyFingerprint: dependencyFingerprint,
			RendererFingerprint:   "renderer-" + id,
			NodeBundleFingerprint: "bundle-" + id, CompilerVersion: CompilerVersion, HasNode: true,
		}},
	}
}

func TestNodeRuntimeProcessInvokesHandlersEmitsEventsAndCleansUp(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not available")
	}
	root := t.TempDir()
	buildDirectory := filepath.Join(root, "build")
	packageDirectory := filepath.Join(root, "package")
	dataDirectory := filepath.Join(root, "data")
	for _, directory := range []string{buildDirectory, packageDirectory, dataDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	bundle := `
exports.activate = ({ rpc, dataDirectory }) => {
  const fs = require("node:fs");
  const path = require("node:path");
  rpc.handle("echo", ({ value }) => ({ value, from: "node" }));
  rpc.emit("activated", { ready: true });
  return () => fs.writeFileSync(path.join(dataDirectory, "cleaned"), "yes");
};
`
	if err := os.WriteFile(filepath.Join(buildDirectory, "node-bundle.cjs"), []byte(bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg := Package{ID: "node-runtime-test", ActiveBuild: &ActivePackageBuild{OutputDirectory: buildDirectory}}
	events := make(chan NodeRuntimeEvent, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	process, err := startNodeRuntimeProcess(
		ctx,
		pkg,
		NodeEnvironment{NodePath: nodePath},
		dataDirectory,
		"authorization",
		nil,
		func(event NodeRuntimeEvent) { events <- event },
	)
	if err != nil {
		t.Fatal(err)
	}
	result, invocationError := process.invoke(ctx, "echo", json.RawMessage(`{"value":42}`))
	if invocationError != nil || string(result) != `{"value":42,"from":"node"}` {
		t.Fatalf("Node invocation = %s, %v", result, invocationError)
	}
	select {
	case event := <-events:
		if event.Name != "activated" || string(event.Payload) != `{"ready":true}` {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-ctx.Done():
		t.Fatal("Node event was not delivered")
	}
	process.stop()
	if contents, err := os.ReadFile(filepath.Join(dataDirectory, "cleaned")); err != nil || string(contents) != "yes" {
		t.Fatalf("Node cleanup did not run: %q, %v", contents, err)
	}
}
