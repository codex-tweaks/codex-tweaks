package core

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuilderArgumentsPreserveLockedAndPinnedContract(t *testing.T) {
	wantInstall := []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}
	if len(DependencyInstallArguments) != len(wantInstall) {
		t.Fatalf("unexpected npm args: %#v", DependencyInstallArguments)
	}
	for index := range wantInstall {
		if DependencyInstallArguments[index] != wantInstall[index] {
			t.Fatalf("unexpected npm args: %#v", DependencyInstallArguments)
		}
	}
	arguments := EsbuildArguments("/tmp/package/src/index.ts", "/tmp/build/bundle.js", true)
	for _, expected := range []string{"esbuild@" + CompilerVersion, "--platform=browser", "--format=cjs", "--outfile=/tmp/build/bundle.js"} {
		if !containsString(arguments, expected) {
			t.Fatalf("missing %q in %#v", expected, arguments)
		}
	}
	if containsString(arguments, "--offline") {
		t.Fatal("normal build must permit pinned compiler download")
	}
	if !containsString(EsbuildArguments("/tmp/index.js", "/tmp/bundle.js", false), "--offline") {
		t.Fatal("developer build must use only cached compiler")
	}
}

func TestNodeCandidatesUsePathAndDeduplicate(t *testing.T) {
	candidates := NodeCandidates(map[string]string{"PATH": filepath.Join("/custom", "bin") + string(filepath.ListSeparator) + filepath.Join("/custom", "bin") + string(filepath.ListSeparator) + filepath.Join("/second", "bin")}, "/tmp/home")
	executable := "node"
	if runtime.GOOS == "windows" {
		executable = "node.exe"
	}
	if candidates[0] != filepath.Join("/custom", "bin", executable) || candidates[1] != filepath.Join("/second", "bin", executable) {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}
