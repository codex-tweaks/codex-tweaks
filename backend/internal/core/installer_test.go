package core

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalInstallerCopiesDirectoryAndDropsGeneratedTrees(t *testing.T) {
	store, root := newTestStore(t)
	source := filepath.Join(root, "source")
	writeInstallerPackage(t, source, "local-directory-sample", "1.0.0", nil)
	writeTestFile(t, filepath.Join(source, ".git", "config"), "ignored")
	writeTestFile(t, filepath.Join(source, "node_modules", "demo", "index.js"), "ignored")
	result, err := NewLocalInstaller(store).Install(source)
	if err != nil {
		t.Fatal(err)
	}
	if result.PackageID != "local-directory-sample" || !isRegularNonSymlink(filepath.Join(result.Directory, "package.json")) {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(result.Directory, ".git")); !os.IsNotExist(err) {
		t.Fatal(".git should be excluded")
	}
	if _, err := os.Stat(filepath.Join(result.Directory, "node_modules")); !os.IsNotExist(err) {
		t.Fatal("node_modules should be excluded")
	}
}

func TestLocalInstallerHandlesWrapperZIPAndRejectsTraversal(t *testing.T) {
	store, root := newTestStore(t)
	archivePath := filepath.Join(root, "package.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archiveFile)
	addZIPFile(t, writer, "repository-main/package.json", installerManifestJSON("zip-sample", "1.0.0", nil))
	addZIPFile(t, writer, "repository-main/src/index.js", "export function activate() {}")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := NewLocalInstaller(store).Install(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.PackageID != "zip-sample" || !isRegularNonSymlink(filepath.Join(result.Directory, "src", "index.js")) {
		t.Fatalf("unexpected ZIP result: %#v", result)
	}

	unsafePath := filepath.Join(root, "unsafe.zip")
	unsafeFile, _ := os.Create(unsafePath)
	unsafeWriter := zip.NewWriter(unsafeFile)
	addZIPFile(t, unsafeWriter, "../escape.js", "escape")
	_ = unsafeWriter.Close()
	_ = unsafeFile.Close()
	_, err = NewLocalInstaller(store).Install(unsafePath)
	if err == nil || !strings.Contains(err.Error(), "不安全的路径") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestLocalInstallerRejectsInvalidDuplicateAndSymlink(t *testing.T) {
	store, root := newTestStore(t)
	invalid := filepath.Join(root, "invalid")
	writeInstallerPackage(t, invalid, "missing-lock", "1.0.0", map[string]string{"demo": "1.0.0"})
	_, err := NewLocalInstaller(store).Install(invalid)
	if err == nil || !strings.Contains(err.Error(), "package-lock.json") {
		t.Fatalf("expected lockfile error, got %v", err)
	}

	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	writeInstallerPackage(t, first, "duplicate", "1.0.0", nil)
	writeInstallerPackage(t, second, "duplicate", "2.0.0", nil)
	installer := NewLocalInstaller(store)
	if _, err := installer.Install(first); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(second); err == nil || !strings.Contains(err.Error(), "已经安装") {
		t.Fatalf("expected duplicate error, got %v", err)
	}

	t.Run("symlink", func(t *testing.T) {
		linked := filepath.Join(root, "linked")
		writeInstallerPackage(t, linked, "linked", "1.0.0", nil)
		if err := os.Symlink(filepath.Join(linked, "src", "index.js"), filepath.Join(linked, "linked.js")); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("current Windows user cannot create the symlink fixture: %v", err)
			}
			t.Fatal(err)
		}
		if _, err := installer.Install(linked); err == nil || !strings.Contains(err.Error(), "符号链接") {
			t.Fatalf("expected symlink rejection, got %v", err)
		}
	})
}

func writeInstallerPackage(t *testing.T, directory, name, version string, dependencies map[string]string) {
	t.Helper()
	writeTestFile(t, filepath.Join(directory, "package.json"), installerManifestJSON(name, version, dependencies))
	writeTestFile(t, filepath.Join(directory, "src", "index.js"), "export function activate() {}")
}

func installerManifestJSON(name, version string, dependencies map[string]string) string {
	if dependencies == nil {
		dependencies = map[string]string{}
	}
	manifest := PackageManifest{Name: name, Version: version, Description: "test", Dependencies: dependencies, CodexTweaks: PackageConfiguration{APIVersion: 2, Entry: "src/index.js", Priority: 100, PackageDependencies: map[string]PackageDependency{}}}
	return JSONLiteral(manifest)
}

func writeTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func addZIPFile(t *testing.T, writer *zip.Writer, name, contents string) {
	t.Helper()
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
}
