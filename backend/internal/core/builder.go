package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var DependencyInstallArguments = []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}

type NodeEnvironment struct {
	NodePath string `json:"nodePath"`
	NPMPath  string `json:"npmPath"`
	NPXPath  string `json:"npxPath"`
	Version  string `json:"version"`
}

type Builder struct {
	store  *Store
	runner CommandRunner
	mu     sync.Mutex
}

func NewBuilder(store *Store, runner CommandRunner) *Builder {
	if runner == nil {
		runner = SystemCommandRunner{}
	}
	return &Builder{store: store, runner: runner}
}

func (b *Builder) DetectNodeEnvironment(ctx context.Context) *NodeEnvironment {
	environment := environmentMap()
	home, _ := os.UserHomeDir()
	for _, nodePath := range NodeCandidates(environment, home) {
		binDirectory := filepath.Dir(nodePath)
		npmName, npxName := "npm", "npx"
		if runtime.GOOS == "windows" {
			npmName, npxName = "npm.cmd", "npx.cmd"
		}
		npmPath, npxPath := filepath.Join(binDirectory, npmName), filepath.Join(binDirectory, npxName)
		if !isExecutable(nodePath) || !isExecutable(npmPath) || !isExecutable(npxPath) {
			continue
		}
		result, err := b.runner.Run(ctx, nodePath, []string{"--version"}, binDirectory, processEnvironment(binDirectory, environment))
		if err != nil || result.Status != 0 {
			continue
		}
		return &NodeEnvironment{NodePath: nodePath, NPMPath: npmPath, NPXPath: npxPath, Version: strings.TrimSpace(result.Output)}
	}
	return nil
}

func (b *Builder) Build(ctx context.Context, pkg Package, installDependencies, allowCompilerDownload bool) (PackageBuildRecord, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if pkg.ValidationError != nil || pkg.Manifest == nil || pkg.SourceFingerprint == nil || pkg.DependencyFingerprint == nil {
		return PackageBuildRecord{}, errors.New("包配置无效，无法编译。")
	}
	node := b.DetectNodeEnvironment(ctx)
	if node == nil {
		return PackageBuildRecord{}, errors.New("没有找到可用的 Node.js、npm 和 npx。")
	}
	environment := processEnvironment(filepath.Dir(node.NodePath), environmentMap())
	if installDependencies && len(pkg.Manifest.Dependencies) > 0 {
		if !isRegularNonSymlink(filepath.Join(pkg.Directory, "package-lock.json")) {
			return PackageBuildRecord{}, errors.New("包含 npm 依赖的包必须提供 package-lock.json。")
		}
		result, err := b.runner.Run(ctx, node.NPMPath, DependencyInstallArguments, pkg.Directory, environment)
		if err != nil {
			return PackageBuildRecord{}, err
		}
		if err := requireCommandSuccess(result, "npm ci"); err != nil {
			return PackageBuildRecord{}, err
		}
	}

	buildsDirectory, err := b.store.BuildsDirectory(pkg.ID)
	if err != nil {
		return PackageBuildRecord{}, err
	}
	stagingDirectory, err := os.MkdirTemp(buildsDirectory, ".staging-")
	if err != nil {
		return PackageBuildRecord{}, err
	}
	defer os.RemoveAll(stagingDirectory)
	entryPath := filepath.Join(pkg.Directory, filepath.FromSlash(pkg.Manifest.CodexTweaks.Entry))
	javaScriptPath := filepath.Join(stagingDirectory, "bundle.js")
	result, err := b.runner.Run(ctx, node.NPXPath, EsbuildArguments(entryPath, javaScriptPath, allowCompilerDownload), pkg.Directory, environment)
	if err != nil {
		return PackageBuildRecord{}, err
	}
	if err := requireCommandSuccess(result, "esbuild"); err != nil {
		return PackageBuildRecord{}, err
	}
	if !isRegularNonSymlink(javaScriptPath) {
		return PackageBuildRecord{}, errors.New("编译器没有生成 bundle.js。")
	}
	randomSuffix, err := randomHex(4)
	if err != nil {
		return PackageBuildRecord{}, err
	}
	buildDirectoryName := *pkg.SourceFingerprint + "-esbuild-" + CompilerVersion + "-" + randomSuffix
	finalDirectory := filepath.Join(buildsDirectory, buildDirectoryName)
	if err := os.Rename(stagingDirectory, finalDirectory); err != nil {
		return PackageBuildRecord{}, err
	}
	record := PackageBuildRecord{
		PackageID: pkg.Manifest.Name, PackageVersion: pkg.Manifest.Version,
		PackageDependencies: packageDependencyVersions(pkg.Manifest.CodexTweaks.PackageDependencies),
		SourceFingerprint:   *pkg.SourceFingerprint, DependencyFingerprint: *pkg.DependencyFingerprint,
		CompilerVersion: CompilerVersion, NodeVersion: node.Version, BuildDirectoryName: buildDirectoryName,
		HasCSS: isRegularNonSymlink(filepath.Join(finalDirectory, "bundle.css")), BuiltAt: NewCodableTime(time.Now()),
	}
	if err := b.store.ActivateBuild(record); err != nil {
		return PackageBuildRecord{}, err
	}
	return record, nil
}

func NodeCandidates(environment map[string]string, home string) []string {
	executable := "node"
	if runtime.GOOS == "windows" {
		executable = "node.exe"
	}
	candidates := []string{}
	for _, path := range filepath.SplitList(environmentValue(environment, "PATH")) {
		if path != "" {
			candidates = append(candidates, filepath.Join(path, executable))
		}
	}
	if runtime.GOOS == "windows" {
		for _, root := range []string{environmentValue(environment, "ProgramFiles"), environmentValue(environment, "LOCALAPPDATA")} {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, "nodejs", executable))
			}
		}
	} else {
		for _, binDirectory := range []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin"), filepath.Join(home, ".local", "share", "mise", "shims"), filepath.Join(home, ".volta", "bin")} {
			candidates = append(candidates, filepath.Join(binDirectory, executable))
		}
		for _, root := range []string{filepath.Join(home, ".local", "share", "mise", "installs", "node"), filepath.Join(home, ".nvm", "versions", "node")} {
			entries, err := os.ReadDir(root)
			if err != nil {
				continue
			}
			sort.Slice(entries, func(left, right int) bool { return entries[left].Name() > entries[right].Name() })
			for _, entry := range entries {
				if entry.IsDir() {
					candidates = append(candidates, filepath.Join(root, entry.Name(), "bin", executable))
				}
			}
		}
	}
	seen := map[string]bool{}
	result := []string{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if !seen[candidate] {
			seen[candidate] = true
			result = append(result, candidate)
		}
	}
	return result
}

func EsbuildArguments(entryPath, outputPath string, allowCompilerDownload bool) []string {
	arguments := []string{"--yes"}
	if !allowCompilerDownload {
		arguments = append(arguments, "--offline")
	}
	return append(arguments,
		"esbuild@"+CompilerVersion, entryPath, "--bundle", "--platform=browser", "--format=cjs",
		"--target=chrome120", "--sourcemap=inline", "--log-level=warning",
		`--define:process.env.NODE_ENV="production"`, "--outfile="+outputPath,
	)
}

func processEnvironment(nodeBinDirectory string, values map[string]string) []string {
	clone := map[string]string{}
	for key, value := range values {
		clone[key] = value
	}
	existingPath := environmentValue(clone, "PATH")
	if existingPath == "" {
		existingPath = "/usr/bin:/bin:/usr/sbin:/sbin"
	}
	if nodeBinDirectory != "" {
		setEnvironmentValue(clone, "PATH", nodeBinDirectory+string(os.PathListSeparator)+existingPath)
	} else {
		setEnvironmentValue(clone, "PATH", existingPath)
	}
	clone["NO_UPDATE_NOTIFIER"] = "1"
	return environmentSlice(clone)
}

func environmentSlice(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func environmentMap() map[string]string {
	result := map[string]string{}
	for _, value := range os.Environ() {
		if key, item, ok := strings.Cut(value, "="); ok {
			result[key] = item
		}
	}
	return result
}

func environmentValue(values map[string]string, key string) string {
	if value, exists := values[key]; exists {
		return value
	}
	if runtime.GOOS == "windows" {
		for candidate, value := range values {
			if strings.EqualFold(candidate, key) {
				return value
			}
		}
	}
	return ""
}

func setEnvironmentValue(values map[string]string, key, value string) {
	if runtime.GOOS == "windows" {
		for candidate := range values {
			if candidate != key && strings.EqualFold(candidate, key) {
				delete(values, candidate)
			}
		}
	}
	values[key] = value
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0
}

func requireCommandSuccess(result CommandResult, command string) error {
	if result.Status == 0 {
		return nil
	}
	detail := strings.TrimSpace(tailBytes(result.Output, 8000))
	if detail == "" {
		return fmt.Errorf("%s 执行失败（退出码 %d）。", command, result.Status)
	}
	return fmt.Errorf("%s 执行失败（退出码 %d）：\n%s", command, result.Status, detail)
}

func tailBytes(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[len(value)-maximum:]
}

func randomHex(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func packageDependencyVersions(dependencies map[string]PackageDependency) map[string]string {
	result := make(map[string]string, len(dependencies))
	for packageID, dependency := range dependencies {
		result[packageID] = dependency.Version
	}
	return result
}
