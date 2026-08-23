//go:build windows

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsEnvironmentPathLookupIsCaseInsensitive(t *testing.T) {
	nodeDirectory := filepath.Join(t.TempDir(), "node")
	gitDirectory := filepath.Join(t.TempDir(), "git")
	environment := map[string]string{
		"Path": strings.Join([]string{nodeDirectory, gitDirectory}, string(os.PathListSeparator)),
	}
	if !containsString(NodeCandidates(environment, ""), filepath.Join(nodeDirectory, "node.exe")) {
		t.Fatalf("Node candidates did not read mixed-case Path: %#v", NodeCandidates(environment, ""))
	}
	if !containsString(GitCandidates(environment), filepath.Join(gitDirectory, "git.exe")) {
		t.Fatalf("Git candidates did not read mixed-case Path: %#v", GitCandidates(environment))
	}

	processed := processEnvironment(nodeDirectory, environment)
	pathEntries := 0
	for _, entry := range processed {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") {
			pathEntries++
		}
	}
	if pathEntries != 1 {
		t.Fatalf("process environment contains %d case-insensitive PATH entries: %#v", pathEntries, processed)
	}
}
