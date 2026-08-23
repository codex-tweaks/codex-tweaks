package core

import (
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Store struct {
	TweaksDirectory          string
	PackagesDirectory        string
	StateDirectory           string
	PackageSettingsPath      string
	ManagedPackagesDirectory string
	ManagedSourcesDirectory  string
	ManagedRegistryPath      string
	ManagedLockfilePath      string
	BuildCacheDirectory      string
	BundledPackagesDirectory string
}

func NewStore(applicationSupport, caches, bundledPackages string) (*Store, error) {
	var err error
	if applicationSupport == "" {
		applicationSupport, err = os.UserConfigDir()
		if err != nil {
			return nil, err
		}
	}
	if caches == "" {
		caches, err = os.UserCacheDir()
		if err != nil {
			return nil, err
		}
	}
	applicationRoot := filepath.Join(applicationSupport, "Codex Tweaks")
	tweaks := filepath.Join(applicationRoot, "Tweaks")
	managed := filepath.Join(applicationRoot, "ManagedPackages")
	state := filepath.Join(applicationRoot, "State")
	return &Store{
		TweaksDirectory:          tweaks,
		PackagesDirectory:        filepath.Join(tweaks, "packages"),
		StateDirectory:           state,
		PackageSettingsPath:      filepath.Join(state, "package-settings.json"),
		ManagedPackagesDirectory: managed,
		ManagedSourcesDirectory:  filepath.Join(managed, "sources"),
		ManagedRegistryPath:      filepath.Join(managed, "registry.json"),
		ManagedLockfilePath:      filepath.Join(managed, "packages.lock.json"),
		BuildCacheDirectory:      filepath.Join(caches, "Codex Tweaks", "PackageBuilds"),
		BundledPackagesDirectory: bundledPackages,
	}, nil
}

func (s *Store) Prepare() error {
	for _, directory := range []string{s.PackagesDirectory, s.BuildCacheDirectory, s.StateDirectory, s.ManagedPackagesDirectory, s.ManagedSourcesDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return err
		}
	}
	if s.BundledPackagesDirectory == "" {
		return nil
	}
	if _, err := os.Stat(s.BundledPackagesDirectory); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	packages, err := directSubdirectories(s.BundledPackagesDirectory)
	if err != nil {
		return err
	}
	for _, source := range packages {
		destination := filepath.Join(s.PackagesDirectory, filepath.Base(source))
		if _, err := os.Stat(destination); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := copyTree(source, destination, map[string]bool{}, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadPackages() ([]Package, error) {
	if err := s.Prepare(); err != nil {
		return nil, err
	}
	settings, err := s.LoadUserSettings()
	if err != nil {
		return nil, err
	}
	settingsChanged := false
	normalizedPriority := func(pkg Package) *int {
		setting, exists := settings.Packages[pkg.ID]
		if !exists || setting.PriorityOverride == nil || pkg.Manifest == nil {
			return setting.PriorityOverride
		}
		if *setting.PriorityOverride != pkg.DeclaredPriority() {
			return setting.PriorityOverride
		}
		delete(settings.Packages, pkg.ID)
		settingsChanged = true
		return nil
	}

	localDirectories, err := directSubdirectories(s.PackagesDirectory)
	if err != nil {
		return nil, err
	}
	packages := make([]Package, 0, len(localDirectories))
	for _, directory := range localDirectories {
		pkg := s.InspectPackage(directory, LocalOrigin(), nil, "")
		pkg.PriorityOverride = normalizedPriority(pkg)
		packages = append(packages, pkg)
	}
	lockfile, err := s.LoadManagedLockfile()
	if err != nil {
		return nil, err
	}
	lockIDs := make([]string, 0, len(lockfile.Packages))
	for packageID := range lockfile.Packages {
		lockIDs = append(lockIDs, packageID)
	}
	sort.Strings(lockIDs)
	managedRoot, _ := filepath.Abs(s.ManagedPackagesDirectory)
	for _, packageID := range lockIDs {
		lock := lockfile.Packages[packageID]
		sourcePath := filepath.Clean(filepath.Join(s.ManagedPackagesDirectory, filepath.FromSlash(lock.SourceRelativePath)))
		absoluteSource, _ := filepath.Abs(sourcePath)
		origin := ManagedOrigin(lock)
		if !pathIsWithin(absoluteSource, managedRoot) || !directoryExists(absoluteSource) {
			message := "远程功能包的锁定源码不存在。"
			packages = append(packages, Package{ID: lock.PackageID, DirectoryName: lock.PackageID, Directory: absoluteSource, ValidationError: &message, PriorityOverride: settings.Packages[lock.PackageID].PriorityOverride, Origin: origin})
			continue
		}
		pkg := s.InspectPackage(absoluteSource, origin, nil, lock.PackageID)
		pkg.PriorityOverride = normalizedPriority(pkg)
		packages = append(packages, pkg)
	}
	if settingsChanged {
		if err := s.writeUserSettings(settings); err != nil {
			return nil, err
		}
	}

	counts := map[string]int{}
	for _, pkg := range packages {
		if pkg.Manifest != nil {
			counts[pkg.Manifest.Name]++
		}
	}
	for index := range packages {
		if packages[index].Manifest != nil && counts[packages[index].Manifest.Name] > 1 {
			message := "包标识重复：" + packages[index].Manifest.Name
			packages[index].ValidationError = &message
		}
	}
	sortPackages(packages)
	return packages, nil
}

func (s *Store) InspectPackage(directory string, origin PackageOrigin, priorityOverride *int, expectedPackageID string) Package {
	directory = filepath.Clean(directory)
	directoryName := filepath.Base(directory)
	invalid := func(err error) Package {
		message := err.Error()
		return Package{ID: "invalid:" + directoryName, DirectoryName: directoryName, Directory: directory, ValidationError: &message, PriorityOverride: priorityOverride, Origin: origin}
	}
	manifestPath := filepath.Join(directory, "package.json")
	if !isRegularNonSymlink(manifestPath) {
		return invalid(errors.New("package.json 必须是包目录中的普通文件。"))
	}
	var manifest PackageManifest
	if err := readJSON(manifestPath, &manifest); err != nil {
		return invalid(err)
	}
	if err := validateManifest(manifest, directory); err != nil {
		return invalid(err)
	}
	if expectedPackageID != "" && manifest.Name != expectedPackageID {
		return invalid(fmt.Errorf("远程功能包标识不匹配：期望 %s，实际为 %s。", expectedPackageID, manifest.Name))
	}
	sourceFingerprint, err := fingerprintPackage(directory)
	if err != nil {
		return invalid(err)
	}
	dependencyFingerprint, err := fingerprintDependencies(manifest, directory)
	if err != nil {
		return invalid(err)
	}
	activeBuild, _ := s.loadActiveBuild(manifest.Name)
	return Package{
		ID: manifest.Name, DirectoryName: directoryName, Directory: directory, Manifest: &manifest,
		SourceFingerprint: &sourceFingerprint, DependencyFingerprint: &dependencyFingerprint,
		ActiveBuild: activeBuild, PriorityOverride: priorityOverride, Origin: origin,
	}
}

func validateManifest(manifest PackageManifest, packageDirectory string) error {
	if strings.TrimSpace(manifest.Name) == "" || strings.ContainsRune(manifest.Name, 0) || strings.Contains(manifest.Name, "..") {
		return errors.New("package.json 中的 name 无效。")
	}
	if _, ok := ParseSemanticVersion(manifest.Version); !ok {
		return fmt.Errorf("包版本不是有效的 SemVer：%s", manifest.Version)
	}
	if manifest.CodexTweaks.APIVersion != APIVersion {
		return fmt.Errorf("不支持 Codex Tweaks API v%d。", manifest.CodexTweaks.APIVersion)
	}
	for dependencyName, requirement := range manifest.Dependencies {
		if strings.TrimSpace(dependencyName) == "" || strings.TrimSpace(requirement) == "" {
			return fmt.Errorf("npm 依赖名称或版本要求无效：%s。", dependencyName)
		}
	}
	if len(manifest.Dependencies) > 0 && !isRegularNonSymlink(filepath.Join(packageDirectory, "package-lock.json")) {
		return errors.New("声明 npm dependencies 时必须提供 package-lock.json。")
	}
	for dependencyID, dependency := range manifest.CodexTweaks.PackageDependencies {
		if strings.TrimSpace(dependencyID) == "" || dependencyID == manifest.Name {
			return fmt.Errorf("功能包依赖无效：%s。", dependencyID)
		}
		if _, ok := ParseVersionRequirement(dependency.Version); !ok {
			return fmt.Errorf("功能包依赖无效：%s。", dependencyID)
		}
		if dependency.Source != nil {
			if err := ValidateSource(*dependency.Source); err != nil {
				return fmt.Errorf("功能包依赖 %s 的远程来源无效。", dependencyID)
			}
		}
	}
	root, err := filepath.EvalSymlinks(packageDirectory)
	if err != nil {
		return fmt.Errorf("入口文件不存在或超出包目录：%s", manifest.CodexTweaks.Entry)
	}
	entryPath := filepath.Join(packageDirectory, filepath.FromSlash(manifest.CodexTweaks.Entry))
	if !isRegularNonSymlink(entryPath) {
		return fmt.Errorf("入口文件不存在或超出包目录：%s", manifest.CodexTweaks.Entry)
	}
	resolvedEntry, err := filepath.EvalSymlinks(entryPath)
	if err != nil || !pathIsWithin(resolvedEntry, root) || resolvedEntry == root {
		return fmt.Errorf("入口文件不存在或超出包目录：%s", manifest.CodexTweaks.Entry)
	}
	return nil
}

func fingerprintPackage(packageDirectory string) (string, error) {
	type sourceFile struct{ relative, path string }
	files := []sourceFile{}
	err := filepath.WalkDir(packageDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(packageDirectory, path)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		first := strings.Split(relative, "/")[0]
		if (first == ".git" || first == "node_modules") && entry.IsDir() {
			return filepath.SkipDir
		}
		if relative == "package.json" || relative == "package-lock.json" || entry.Name() == ".DS_Store" {
			return nil
		}
		if entry.Type().IsRegular() {
			files = append(files, sourceFile{relative: relative, path: path})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(left, right int) bool { return files[left].relative < files[right].relative })
	state := newFingerprintState()
	for _, file := range files {
		contents, err := os.ReadFile(file.path)
		if err != nil {
			return "", err
		}
		state.Update([]byte(file.relative))
		state.Separator()
		state.Update(contents)
		state.Separator()
	}
	return state.Value(), nil
}

func fingerprintDependencies(manifest PackageManifest, packageDirectory string) (string, error) {
	state := newFingerprintState()
	typeValue := ""
	if manifest.Type != nil {
		typeValue = *manifest.Type
	}
	for _, value := range []string{typeValue, strconv.Itoa(manifest.CodexTweaks.APIVersion), manifest.CodexTweaks.Entry} {
		state.Update([]byte(value))
		state.Separator()
	}
	for _, dependencyID := range sortedStringKeys(manifest.Dependencies) {
		state.Update([]byte(dependencyID))
		state.Separator()
		state.Update([]byte(manifest.Dependencies[dependencyID]))
		state.Separator()
	}
	packageDependencyIDs := make([]string, 0, len(manifest.CodexTweaks.PackageDependencies))
	for dependencyID := range manifest.CodexTweaks.PackageDependencies {
		packageDependencyIDs = append(packageDependencyIDs, dependencyID)
	}
	sort.Strings(packageDependencyIDs)
	for _, dependencyID := range packageDependencyIDs {
		dependency := manifest.CodexTweaks.PackageDependencies[dependencyID]
		state.Update([]byte(dependencyID))
		state.Separator()
		state.Update([]byte(dependency.Version))
		state.Separator()
		if dependency.Source != nil {
			state.Update([]byte(dependency.Source.URL))
			state.Separator()
			state.Update([]byte(dependency.Source.Selector.Type))
			state.Separator()
			if dependency.Source.Selector.Value != nil {
				state.Update([]byte(*dependency.Source.Selector.Value))
			}
			state.Separator()
		}
	}
	lockfile := filepath.Join(packageDirectory, "package-lock.json")
	if _, err := os.Stat(lockfile); err == nil {
		contents, readErr := os.ReadFile(lockfile)
		if readErr != nil {
			return "", readErr
		}
		state.Update(contents)
	}
	return state.Value(), nil
}

func (s *Store) loadActiveBuild(packageID string) (*ActivePackageBuild, error) {
	cacheDirectory := s.packageCacheDirectory(packageID)
	activePath := filepath.Join(cacheDirectory, "active.json")
	if _, err := os.Stat(activePath); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var record PackageBuildRecord
	if err := readJSON(activePath, &record); err != nil {
		return nil, err
	}
	if record.PackageID != packageID {
		return nil, errors.New("当前构建记录或编译产物无效。")
	}
	output := filepath.Join(cacheDirectory, "builds", record.BuildDirectoryName)
	if !isRegularNonSymlink(filepath.Join(output, "bundle.js")) || record.HasCSS && !isRegularNonSymlink(filepath.Join(output, "bundle.css")) {
		return nil, errors.New("当前构建记录或编译产物无效。")
	}
	return &ActivePackageBuild{Record: record, OutputDirectory: output}, nil
}

func (s *Store) BuildsDirectory(packageID string) (string, error) {
	directory := filepath.Join(s.packageCacheDirectory(packageID), "builds")
	return directory, os.MkdirAll(directory, 0o700)
}

func (s *Store) ActivateBuild(record PackageBuildRecord) error {
	return writeJSONAtomic(filepath.Join(s.packageCacheDirectory(record.PackageID), "active.json"), record)
}

func (s *Store) packageCacheDirectory(packageID string) string {
	return filepath.Join(s.BuildCacheDirectory, FingerprintString(packageID))
}

func (s *Store) LoadUserSettings() (PackageUserSettings, error) {
	settings := NewPackageUserSettings()
	if _, err := os.Stat(s.PackageSettingsPath); errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err := readJSON(s.PackageSettingsPath, &settings); err != nil {
		return PackageUserSettings{}, err
	}
	if settings.SchemaVersion == 0 {
		settings.SchemaVersion = 1
	}
	if settings.Packages == nil {
		settings.Packages = map[string]PackageUserSetting{}
	}
	return settings, nil
}

func (s *Store) writeUserSettings(settings PackageUserSettings) error {
	return writeJSONAtomic(s.PackageSettingsPath, settings)
}

func (s *Store) SetPriorityOverride(packageID string, priority *int) error {
	if err := s.Prepare(); err != nil {
		return err
	}
	settings, err := s.LoadUserSettings()
	if err != nil {
		return err
	}
	if priority == nil {
		delete(settings.Packages, packageID)
	} else {
		settings.Packages[packageID] = PackageUserSetting{PriorityOverride: priority}
	}
	return s.writeUserSettings(settings)
}

func (s *Store) LoadManagedLockfile() (ManagedPackageLockfile, error) {
	lockfile := NewManagedPackageLockfile()
	if _, err := os.Stat(s.ManagedLockfilePath); errors.Is(err, os.ErrNotExist) {
		return lockfile, nil
	}
	if err := readJSON(s.ManagedLockfilePath, &lockfile); err != nil {
		return ManagedPackageLockfile{}, err
	}
	if lockfile.SchemaVersion == 0 {
		lockfile.SchemaVersion = 1
	}
	if lockfile.Packages == nil {
		lockfile.Packages = map[string]ManagedPackageLock{}
	}
	return lockfile, nil
}

func (s *Store) LoadPayload(packages []Package, disabledPackageIDs map[string]bool) PayloadLoadResult {
	compiled := []CompiledPackage{}
	errorsByPackage := map[string]string{}
	resolution := ResolveDependencies(packages, disabledPackageIDs)
	for packageID, issues := range resolution.IssuesByPackageID {
		if !disabledPackageIDs[packageID] && len(issues) > 0 {
			errorsByPackage[packageID] = strings.Join(issues, " ")
		}
	}
	for _, pkg := range resolution.LoadablePackages {
		if pkg.Manifest == nil || pkg.ActiveBuild == nil || pkg.ValidationError != nil {
			continue
		}
		javascript, err := os.ReadFile(pkg.ActiveBuild.JavaScriptPath())
		if err != nil {
			errorsByPackage[pkg.ID] = "无法读取已编译产物：" + err.Error()
			continue
		}
		css := []byte{}
		if pkg.ActiveBuild.Record.HasCSS {
			css, err = os.ReadFile(pkg.ActiveBuild.CSSPath())
			if err != nil {
				errorsByPackage[pkg.ID] = "无法读取已编译产物：" + err.Error()
				continue
			}
		}
		compiled = append(compiled, CompiledPackage{
			ID: pkg.ID, Name: pkg.Manifest.Name, Version: pkg.ActiveBuild.Record.PackageVersion,
			BuildFingerprint: pkg.ActiveBuild.Record.SourceFingerprint + "-" + pkg.ActiveBuild.Record.DependencyFingerprint + "-" + pkg.ActiveBuild.Record.CompilerVersion,
			DependencyIDs:    sortedStringKeys(pkg.ActiveBuild.Record.PackageDependencies), CSS: string(css), JavaScript: string(javascript),
		})
	}
	materials := make([]string, 0, len(compiled))
	for _, pkg := range compiled {
		materials = append(materials, strings.Join([]string{pkg.ID, pkg.Version, pkg.BuildFingerprint, pkg.CSS, pkg.JavaScript}, "\x00"))
	}
	return PayloadLoadResult{Payload: Payload{Packages: compiled, Version: FingerprintString(strings.Join(materials, "\x00"))}, PackageErrors: errorsByPackage}
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

type fingerprintState struct{ hash hash.Hash64 }

func newFingerprintState() *fingerprintState    { return &fingerprintState{hash: fnv.New64a()} }
func (s *fingerprintState) Update(value []byte) { _, _ = s.hash.Write(value) }
func (s *fingerprintState) Separator()          { _, _ = s.hash.Write([]byte{0}) }
func (s *fingerprintState) Value() string       { return fmt.Sprintf("%016x", s.hash.Sum64()) }
