package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuilderBuildsBundledSampleOnDesktop(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_INTEGRATION") != "1" {
		t.Skip("set CODEX_TWEAKS_INTEGRATION=1 to run the real Node/esbuild smoke test")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("desktop runtime smoke test")
	}
	bundled, err := filepath.Abs(filepath.Join("..", "..", "..", "app", "Resources", "Tweaks", "packages"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := NewStore(filepath.Join(root, "support"), filepath.Join(root, "cache"), bundled)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || packages[0].ID != "ct-sample" {
		t.Fatalf("unexpected bundled packages: %#v", packages)
	}
	record, err := NewBuilder(store, nil).Build(context.Background(), packages[0], true, true)
	if err != nil {
		t.Fatal(err)
	}
	if record.PackageID != "ct-sample" || !record.HasCSS {
		t.Fatalf("unexpected build record: %#v", record)
	}
	loaded, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].BuildDisposition(CompilerVersion) != BuildCurrent || loaded[0].ActiveBuild == nil {
		t.Fatalf("build was not atomically activated: %#v", loaded[0])
	}
}
