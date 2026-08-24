package core

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPackageExporterCreatesReinstallableZIP(t *testing.T) {
	store, root := newTestStore(t)
	directory := makeStorePackage(t, store, "export-sample", "export-sample", "1.2.3", 10, nil, "")
	writeTestFile(t, filepath.Join(directory, ".git", "config"), "ignored")
	writeTestFile(t, filepath.Join(directory, "node_modules", "demo", "index.js"), "ignored")
	writeTestFile(t, filepath.Join(directory, "src", ".DS_Store"), "ignored")
	writeTestFile(t, filepath.Join(directory, ".package-note"), "included")

	packages, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages[0]
	if got := PackageArchiveFileName(pkg); got != "export-sample-v1.2.3.zip" {
		t.Fatalf("archive filename = %q", got)
	}
	destination := filepath.Join(root, PackageArchiveFileName(pkg))
	if err := NewPackageExporter().Export(context.Background(), pkg, destination); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
		if !strings.HasPrefix(file.Name, "export-sample/") {
			t.Fatalf("archive entry escaped wrapper directory: %q", file.Name)
		}
	}
	joined := strings.Join(names, "\n")
	for _, required := range []string{"export-sample/package.json", "export-sample/src/index.js", "export-sample/.package-note"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("archive is missing %q: %s", required, joined)
		}
	}
	for _, excluded := range []string{".git", "node_modules", ".DS_Store"} {
		if strings.Contains(joined, excluded) {
			t.Fatalf("archive contains excluded tree %q: %s", excluded, joined)
		}
	}

	importStore, _ := newTestStore(t)
	result, err := NewLocalInstaller(importStore).Install(destination)
	if err != nil {
		t.Fatalf("exported archive cannot be reinstalled: %v", err)
	}
	if result.PackageID != "export-sample" {
		t.Fatalf("reinstalled package ID = %q", result.PackageID)
	}
}

func TestPackageExporterRejectsUnsafeSourceAndDestination(t *testing.T) {
	store, root := newTestStore(t)
	directory := makeStorePackage(t, store, "unsafe-export", "unsafe-export", "1.0.0", 0, nil, "")
	packages, err := store.LoadPackages()
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages[0]
	exporter := NewPackageExporter()

	insideSource := filepath.Join(directory, "unsafe-export.zip")
	if err := exporter.Export(context.Background(), pkg, insideSource); err == nil || !strings.Contains(err.Error(), "功能包目录") {
		t.Fatalf("expected destination-inside-source rejection, got %v", err)
	}

	symlink := filepath.Join(directory, "linked.js")
	if err := os.Symlink(filepath.Join(directory, "src", "index.js"), symlink); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("current Windows user cannot create the symlink fixture: %v", err)
		}
		t.Fatal(err)
	}
	destination := filepath.Join(root, "unsafe-export.zip")
	if err := exporter.Export(context.Background(), pkg, destination); err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("failed export left a destination file: %v", err)
	}
}
