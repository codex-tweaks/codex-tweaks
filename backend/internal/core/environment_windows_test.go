//go:build windows

package core

import (
	"context"
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

func TestWindowsNodeCandidatesIncludeMiseLocations(t *testing.T) {
	localAppData := t.TempDir()
	customMiseData := t.TempDir()
	defaultMiseData := filepath.Join(localAppData, "mise")
	for _, versionDirectory := range []string{
		filepath.Join(customMiseData, "installs", "node", "22.22.2"),
		filepath.Join(defaultMiseData, "installs", "node", "20.19.0"),
	} {
		if err := os.MkdirAll(versionDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	candidates := NodeCandidates(map[string]string{
		"LOCALAPPDATA":  localAppData,
		"MISE_DATA_DIR": customMiseData,
	}, "")
	for _, expected := range []string{
		filepath.Join(customMiseData, "shims", "node.exe"),
		filepath.Join(customMiseData, "installs", "node", "22.22.2", "node.exe"),
		filepath.Join(defaultMiseData, "shims", "node.exe"),
		filepath.Join(defaultMiseData, "installs", "node", "20.19.0", "node.exe"),
	} {
		if !containsString(candidates, expected) {
			t.Fatalf("Node candidates did not include %q: %#v", expected, candidates)
		}
	}
}

func TestWindowsNodeDetectionAcceptsCommandAndExecutableCompanions(t *testing.T) {
	for _, extension := range []string{".cmd", ".exe"} {
		t.Run(extension, func(t *testing.T) {
			binDirectory := t.TempDir()
			nodePath := filepath.Join(binDirectory, "node.exe")
			for _, path := range []string{
				nodePath,
				filepath.Join(binDirectory, "npm"+extension),
				filepath.Join(binDirectory, "npx"+extension),
			} {
				if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", binDirectory)

			runner := nodeDetectionRunnerFunc(func(_ context.Context, executable string, arguments []string, directory string, _ []string) (CommandResult, error) {
				if executable != nodePath || len(arguments) != 1 || arguments[0] != "--version" || directory != binDirectory {
					t.Fatalf("unexpected Node detection command: %q %#v in %q", executable, arguments, directory)
				}
				return CommandResult{Output: "v22.22.2\n"}, nil
			})
			environment := NewBuilder(nil, runner).DetectNodeEnvironment(context.Background())
			if environment == nil {
				t.Fatal("Node environment was not detected")
			}
			if environment.NodePath != nodePath || environment.NPMPath != filepath.Join(binDirectory, "npm"+extension) || environment.NPXPath != filepath.Join(binDirectory, "npx"+extension) || environment.Version != "v22.22.2" {
				t.Fatalf("unexpected Node environment: %#v", environment)
			}
		})
	}
}

type nodeDetectionRunnerFunc func(
	ctx context.Context,
	executable string,
	arguments []string,
	directory string,
	environment []string,
) (CommandResult, error)

func (function nodeDetectionRunnerFunc) Run(
	ctx context.Context,
	executable string,
	arguments []string,
	directory string,
	environment []string,
) (CommandResult, error) {
	return function(ctx, executable, arguments, directory, environment)
}
