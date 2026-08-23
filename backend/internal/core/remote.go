package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type GitEnvironment struct {
	GitPath string `json:"gitPath"`
	Version string `json:"version"`
}

type RemoteResolution struct {
	Reference      string
	Commit         string
	FetchReference string
	ETag           *string
}

type RemoteUpdateStatus string

const (
	RemoteUpdateCurrent                RemoteUpdateStatus = "current"
	RemoteUpdateAvailable              RemoteUpdateStatus = "available"
	RemoteUpdatePinnedReferenceChanged RemoteUpdateStatus = "pinnedReferenceChanged"
)

type RemoteUpdate struct {
	PackageID          string             `json:"packageID"`
	CurrentCommit      string             `json:"currentCommit"`
	CandidateReference string             `json:"candidateReference"`
	CandidateCommit    string             `json:"candidateCommit"`
	CheckedAt          CodableTime        `json:"checkedAt"`
	Status             RemoteUpdateStatus `json:"status"`
}

func (u RemoteUpdate) Installable() bool { return u.Status == RemoteUpdateAvailable }

type ManagedPackageInstallResult struct {
	PackageID string             `json:"packageID"`
	Manifest  PackageManifest    `json:"manifest"`
	Lock      ManagedPackageLock `json:"lock"`
}

type RemoteInstallOptions struct {
	ExpectedPackageID string
	Requirement       string
}

type pendingRemoteDependency struct {
	DependencyID string
	Dependency   PackageDependency
}

type RemoteManager struct {
	store       *Store
	runner      CommandRunner
	httpClient  *http.Client
	environment map[string]string
	mu          sync.Mutex
}

func NewRemoteManager(store *Store, runner CommandRunner, httpClient *http.Client, environment map[string]string) *RemoteManager {
	if runner == nil {
		runner = SystemCommandRunner{}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if environment == nil {
		environment = environmentMap()
	}
	return &RemoteManager{store: store, runner: runner, httpClient: httpClient, environment: environment}
}

func (m *RemoteManager) DetectGitEnvironment(ctx context.Context) *GitEnvironment {
	for _, gitPath := range GitCandidates(m.environment) {
		if !isExecutable(gitPath) {
			continue
		}
		result, err := m.runner.Run(ctx, gitPath, []string{"--version"}, os.TempDir(), m.gitEnvironment())
		if err == nil && result.Status == 0 {
			return &GitEnvironment{GitPath: gitPath, Version: strings.TrimSpace(result.Output)}
		}
	}
	return nil
}

func GitCandidates(environment map[string]string) []string {
	executable := "git"
	if runtime.GOOS == "windows" {
		executable = "git.exe"
	}
	candidates := []string{}
	for _, directory := range filepath.SplitList(environmentValue(environment, "PATH")) {
		if directory != "" {
			candidates = append(candidates, filepath.Join(directory, executable))
		}
	}
	if runtime.GOOS == "windows" {
		for _, root := range []string{environmentValue(environment, "ProgramFiles"), environmentValue(environment, "ProgramFiles(x86)")} {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, "Git", "cmd", executable))
			}
		}
	} else {
		candidates = append(candidates, "/usr/bin/git", "/opt/homebrew/bin/git", "/usr/local/bin/git")
	}
	seen, result := map[string]bool{}, []string{}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if !seen[candidate] {
			seen[candidate] = true
			result = append(result, candidate)
		}
	}
	return result
}

func (m *RemoteManager) ManagedPackageIDs() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	registry, err := m.loadRegistry()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(registry.Packages))
	for packageID := range registry.Packages {
		ids = append(ids, packageID)
	}
	sort.Strings(ids)
	return ids, nil
}

func (m *RemoteManager) Registration(packageID string) (*ManagedPackageRegistration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	registry, err := m.loadRegistry()
	if err != nil {
		return nil, err
	}
	registration, exists := registry.Packages[packageID]
	if !exists {
		return nil, nil
	}
	return &registration, nil
}

func (m *RemoteManager) Install(ctx context.Context, source PackageSource, options RemoteInstallOptions) (ManagedPackageInstallResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.install(ctx, source, options)
}

func (m *RemoteManager) install(ctx context.Context, source PackageSource, options RemoteInstallOptions) (ManagedPackageInstallResult, error) {
	if err := m.store.Prepare(); err != nil {
		return ManagedPackageInstallResult{}, err
	}
	git := m.DetectGitEnvironment(ctx)
	if git == nil {
		return ManagedPackageInstallResult{}, errors.New("没有找到可用的 Git。")
	}
	if err := ValidateSource(source); err != nil {
		return ManagedPackageInstallResult{}, err
	}
	registry, err := m.loadRegistry()
	if err != nil {
		return ManagedPackageInstallResult{}, err
	}
	lockfile, err := m.store.LoadManagedLockfile()
	if err != nil {
		return ManagedPackageInstallResult{}, err
	}
	if options.ExpectedPackageID != "" {
		if existing, found := registry.Packages[options.ExpectedPackageID]; found && !SourcesMatch(existing.Source, source) {
			return ManagedPackageInstallResult{}, fmt.Errorf("依赖 %s 声明的仓库与本机已登记来源不一致。", options.ExpectedPackageID)
		}
	}
	var cached *ManagedPackageRegistration
	if options.ExpectedPackageID != "" {
		if value, found := registry.Packages[options.ExpectedPackageID]; found {
			copy := value
			cached = &copy
		}
	}
	if cached == nil {
		for _, value := range registry.Packages {
			if SourcesMatch(value.Source, source) {
				copy := value
				cached = &copy
				break
			}
		}
	}
	requirements := []string{}
	if cached != nil {
		requirements = append(requirements, cached.VersionRequirements...)
	}
	if options.Requirement != "" && !containsString(requirements, options.Requirement) {
		requirements = append(requirements, options.Requirement)
	}
	resolution, err := m.resolve(ctx, source, requirements, cached)
	if err != nil {
		return ManagedPackageInstallResult{}, err
	}
	staging, err := os.MkdirTemp(m.store.ManagedPackagesDirectory, ".staging-")
	if err != nil {
		return ManagedPackageInstallResult{}, err
	}
	defer os.RemoveAll(staging)
	if err := m.checkout(ctx, source, resolution, *git, staging); err != nil {
		return ManagedPackageInstallResult{}, err
	}
	_ = os.RemoveAll(filepath.Join(staging, ".git"))
	inspected := m.store.InspectPackage(staging, LocalOrigin(), nil, "")
	if inspected.ValidationError != nil || inspected.Manifest == nil {
		message := "无法读取 package.json。"
		if inspected.ValidationError != nil {
			message = *inspected.ValidationError
		}
		return ManagedPackageInstallResult{}, errors.New("远程仓库不是有效的 Codex Tweaks 功能包：" + message)
	}
	manifest := *inspected.Manifest
	if options.ExpectedPackageID != "" && manifest.Name != options.ExpectedPackageID {
		return ManagedPackageInstallResult{}, fmt.Errorf("功能包标识不匹配：期望 %s，实际为 %s。", options.ExpectedPackageID, manifest.Name)
	}
	for _, requirementText := range requirements {
		if requirement, ok := ParseVersionRequirement(requirementText); ok && !requirement.Contains(manifest.Version) {
			return ManagedPackageInstallResult{}, fmt.Errorf("功能包 %s 的 v%s 不满足版本要求 %s。", manifest.Name, manifest.Version, requirementText)
		}
	}
	if existing, found := registry.Packages[manifest.Name]; found && !SourcesMatch(existing.Source, source) {
		return ManagedPackageInstallResult{}, fmt.Errorf("依赖 %s 声明的仓库与本机已登记来源不一致。", manifest.Name)
	}
	packageSourceRoot := filepath.Join(m.store.ManagedSourcesDirectory, FingerprintString(manifest.Name))
	if err := os.MkdirAll(packageSourceRoot, 0o700); err != nil {
		return ManagedPackageInstallResult{}, err
	}
	commit := strings.ToLower(resolution.Commit)
	finalSource := filepath.Join(packageSourceRoot, commit)
	if !directoryExists(finalSource) {
		if err := os.Rename(staging, finalSource); err != nil {
			return ManagedPackageInstallResult{}, err
		}
	}
	relativePath, err := filepath.Rel(m.store.ManagedPackagesDirectory, finalSource)
	if err != nil {
		return ManagedPackageInstallResult{}, err
	}
	now := NewCodableTime(time.Now())
	lock := ManagedPackageLock{
		PackageID: manifest.Name, PackageVersion: manifest.Version, Source: source,
		ResolvedReference: resolution.Reference, ResolvedCommit: commit,
		SourceRelativePath: filepath.ToSlash(relativePath), InstalledAt: now,
	}
	addedAt := now
	if existing, found := registry.Packages[manifest.Name]; found {
		addedAt = existing.AddedAt
	}
	resolvedReference, resolvedCommit := resolution.Reference, commit
	registry.Packages[manifest.Name] = ManagedPackageRegistration{
		PackageID: manifest.Name, Source: source, AddedAt: addedAt, VersionRequirements: requirements,
		LastCheckedAt: &now, RemoteETag: resolution.ETag,
		LastResolvedReference: &resolvedReference, LastResolvedCommit: &resolvedCommit,
	}
	lockfile.Packages[manifest.Name] = lock
	if err := m.writeRegistry(registry); err != nil {
		return ManagedPackageInstallResult{}, err
	}
	if err := m.writeLockfile(lockfile); err != nil {
		return ManagedPackageInstallResult{}, err
	}
	return ManagedPackageInstallResult{PackageID: manifest.Name, Manifest: manifest, Lock: lock}, nil
}

func (m *RemoteManager) InstallMissingDependencies(ctx context.Context, pkg Package, installedPackages []Package) ([]ManagedPackageInstallResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pkg.Manifest == nil {
		return nil, errors.New("远程仓库不是有效的 Codex Tweaks 功能包：根功能包配置无效。")
	}
	type availablePackage struct {
		Package  Package
		Manifest PackageManifest
	}
	available := map[string]availablePackage{}
	for _, installed := range installedPackages {
		if installed.ValidationError == nil && installed.Manifest != nil {
			if _, exists := available[installed.Manifest.Name]; !exists {
				available[installed.Manifest.Name] = availablePackage{Package: installed, Manifest: *installed.Manifest}
			}
		}
	}
	pending := []pendingRemoteDependency{}
	for dependencyID, dependency := range pkg.Manifest.CodexTweaks.PackageDependencies {
		pending = append(pending, pendingRemoteDependency{DependencyID: dependencyID, Dependency: dependency})
	}
	sort.Slice(pending, func(left, right int) bool { return pending[left].DependencyID < pending[right].DependencyID })
	expanded := map[string]bool{}
	results := []ManagedPackageInstallResult{}
	for len(pending) > 0 {
		item := pending[0]
		pending = pending[1:]
		requirement, ok := ParseVersionRequirement(item.Dependency.Version)
		if !ok {
			return nil, fmt.Errorf("功能包 %s 的 v— 不满足版本要求 %s。", item.DependencyID, item.Dependency.Version)
		}
		if installed, found := available[item.DependencyID]; found {
			if !requirement.Contains(installed.Manifest.Version) {
				return nil, fmt.Errorf("功能包 %s 的 v%s 不满足版本要求 %s。", item.DependencyID, installed.Manifest.Version, item.Dependency.Version)
			}
			if item.Dependency.Source != nil && installed.Package.ManagedLock() != nil && !SourcesMatch(*item.Dependency.Source, installed.Package.ManagedLock().Source) {
				return nil, fmt.Errorf("依赖 %s 声明的仓库与本机已登记来源不一致。", item.DependencyID)
			}
			if installed.Package.Origin.Kind == OriginManaged {
				if err := m.addVersionRequirement(item.Dependency.Version, item.DependencyID); err != nil {
					return nil, err
				}
			}
			if !expanded[item.DependencyID] {
				expanded[item.DependencyID] = true
				pending = appendDependencies(pending, installed.Manifest.CodexTweaks.PackageDependencies)
			}
			continue
		}
		if item.Dependency.Source == nil {
			return nil, fmt.Errorf("缺少依赖 %s，但清单没有提供可安装的 Git 来源。", item.DependencyID)
		}
		result, err := m.install(ctx, *item.Dependency.Source, RemoteInstallOptions{ExpectedPackageID: item.DependencyID, Requirement: item.Dependency.Version})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		directory := filepath.Join(m.store.ManagedPackagesDirectory, filepath.FromSlash(result.Lock.SourceRelativePath))
		installed := Package{ID: result.PackageID, DirectoryName: result.PackageID, Directory: directory, Manifest: &result.Manifest, Origin: ManagedOrigin(result.Lock)}
		available[result.PackageID] = availablePackage{Package: installed, Manifest: result.Manifest}
		if !expanded[result.PackageID] {
			expanded[result.PackageID] = true
			pending = appendDependencies(pending, result.Manifest.CodexTweaks.PackageDependencies)
		}
	}
	return results, nil
}

func appendDependencies(pending []pendingRemoteDependency, dependencies map[string]PackageDependency) []pendingRemoteDependency {
	ids := make([]string, 0, len(dependencies))
	for dependencyID := range dependencies {
		ids = append(ids, dependencyID)
	}
	sort.Strings(ids)
	for _, dependencyID := range ids {
		pending = append(pending, pendingRemoteDependency{DependencyID: dependencyID, Dependency: dependencies[dependencyID]})
	}
	return pending
}

func (m *RemoteManager) CheckForUpdate(ctx context.Context, packageID string) (RemoteUpdate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	registry, err := m.loadRegistry()
	if err != nil {
		return RemoteUpdate{}, err
	}
	lockfile, err := m.store.LoadManagedLockfile()
	if err != nil {
		return RemoteUpdate{}, err
	}
	registration, registered := registry.Packages[packageID]
	lock, locked := lockfile.Packages[packageID]
	if !registered || !locked {
		return RemoteUpdate{}, fmt.Errorf("功能包 %s 不是由 Git 管理的包。", packageID)
	}
	checkedAt := NewCodableTime(time.Now())
	if registration.Source.Selector.Type == SelectorCommit {
		registration.LastCheckedAt = &checkedAt
		registry.Packages[packageID] = registration
		if err := m.writeRegistry(registry); err != nil {
			return RemoteUpdate{}, err
		}
		return RemoteUpdate{PackageID: packageID, CurrentCommit: lock.ResolvedCommit, CandidateReference: lock.ResolvedReference, CandidateCommit: lock.ResolvedCommit, CheckedAt: checkedAt, Status: RemoteUpdateCurrent}, nil
	}
	resolution, err := m.resolve(ctx, registration.Source, registration.VersionRequirements, &registration)
	if err != nil {
		return RemoteUpdate{}, err
	}
	registration.LastCheckedAt = &checkedAt
	if resolution.ETag != nil {
		registration.RemoteETag = resolution.ETag
	}
	reference, commit := resolution.Reference, strings.ToLower(resolution.Commit)
	registration.LastResolvedReference, registration.LastResolvedCommit = &reference, &commit
	registry.Packages[packageID] = registration
	if err := m.writeRegistry(registry); err != nil {
		return RemoteUpdate{}, err
	}
	status := RemoteUpdateCurrent
	if commit != strings.ToLower(lock.ResolvedCommit) {
		if registration.Source.Selector.IsPinned() {
			status = RemoteUpdatePinnedReferenceChanged
		} else {
			status = RemoteUpdateAvailable
		}
	}
	return RemoteUpdate{PackageID: packageID, CurrentCommit: lock.ResolvedCommit, CandidateReference: reference, CandidateCommit: commit, CheckedAt: checkedAt, Status: status}, nil
}

func (m *RemoteManager) resolve(ctx context.Context, source PackageSource, requirements []string, cached *ManagedPackageRegistration) (RemoteResolution, error) {
	git := m.DetectGitEnvironment(ctx)
	if git == nil {
		return RemoteResolution{}, errors.New("没有找到可用的 Git。")
	}
	switch source.Selector.Type {
	case SelectorBranch:
		branch, err := requiredSelectorValue(source.Selector)
		if err != nil {
			return RemoteResolution{}, err
		}
		reference := "refs/heads/" + branch
		commit, err := m.resolveSingleRef(ctx, reference, source, *git)
		return RemoteResolution{Reference: branch, Commit: commit, FetchReference: reference}, err
	case SelectorTag:
		tag, err := requiredSelectorValue(source.Selector)
		if err != nil {
			return RemoteResolution{}, err
		}
		return m.resolveTag(ctx, tag, source, *git, nil)
	case SelectorLatestSemverTag:
		result, err := m.gitCommand(ctx, *git, []string{"ls-remote", "--tags", "--refs", source.URL})
		if err != nil {
			return RemoteResolution{}, err
		}
		type candidate struct {
			Name    string
			Version SemanticVersion
		}
		candidates := []candidate{}
		parsedRequirements := []SemanticVersionRequirement{}
		for _, raw := range requirements {
			if parsed, ok := ParseVersionRequirement(raw); ok {
				parsedRequirements = append(parsedRequirements, parsed)
			}
		}
		for _, line := range strings.Split(result.Output, "\n") {
			columns := strings.Fields(line)
			if len(columns) < 2 || !strings.HasPrefix(columns[1], "refs/tags/") {
				continue
			}
			name := strings.TrimPrefix(columns[1], "refs/tags/")
			version, ok := ParseSemanticVersion(name)
			if !ok || !allRequirementsContain(parsedRequirements, name) {
				continue
			}
			candidates = append(candidates, candidate{Name: name, Version: version})
		}
		if len(candidates) == 0 {
			requirement := strings.Join(requirements, " ∩ ")
			if requirement == "" {
				return RemoteResolution{}, errors.New("远程仓库没有有效的 SemVer Tag。")
			}
			return RemoteResolution{}, fmt.Errorf("没有找到满足 %s 的 SemVer Tag。", requirement)
		}
		sort.Slice(candidates, func(left, right int) bool { return candidates[left].Version.Compare(candidates[right].Version) < 0 })
		return m.resolveTag(ctx, candidates[len(candidates)-1].Name, source, *git, nil)
	case SelectorGitHubLatestRelease, SelectorGitHubRelease:
		tag, etag, err := m.resolveGitHubRelease(ctx, source, cached)
		if err != nil {
			return RemoteResolution{}, err
		}
		return m.resolveTag(ctx, tag, source, *git, etag)
	case SelectorCommit:
		commit, err := requiredSelectorValue(source.Selector)
		commit = strings.ToLower(commit)
		if err != nil || !isFullCommitSHA(commit) {
			return RemoteResolution{}, fmt.Errorf("远程仓库中没有找到引用：%s。", commit)
		}
		return RemoteResolution{Reference: commit, Commit: commit, FetchReference: commit}, nil
	default:
		return RemoteResolution{}, errors.New("不支持的远程来源选择器。")
	}
}

func (m *RemoteManager) resolveSingleRef(ctx context.Context, reference string, source PackageSource, git GitEnvironment) (string, error) {
	result, err := m.gitCommand(ctx, git, []string{"ls-remote", source.URL, reference})
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(result.Output), "\n")
	if len(lines) == 0 || len(strings.Fields(lines[0])) == 0 {
		return "", fmt.Errorf("远程仓库中没有找到引用：%s。", reference)
	}
	return strings.ToLower(strings.Fields(lines[0])[0]), nil
}

func (m *RemoteManager) resolveTag(ctx context.Context, tag string, source PackageSource, git GitEnvironment, etag *string) (RemoteResolution, error) {
	reference := "refs/tags/" + tag
	result, err := m.gitCommand(ctx, git, []string{"ls-remote", "--tags", source.URL, reference, reference + "^{}"})
	if err != nil {
		return RemoteResolution{}, err
	}
	lines := strings.Split(strings.TrimSpace(result.Output), "\n")
	selected := ""
	for _, line := range lines {
		if strings.Contains(line, reference+"^{}") {
			selected = line
			break
		}
	}
	if selected == "" && len(lines) > 0 {
		selected = lines[0]
	}
	columns := strings.Fields(selected)
	if len(columns) == 0 {
		return RemoteResolution{}, fmt.Errorf("远程仓库中没有找到引用：%s。", tag)
	}
	return RemoteResolution{Reference: tag, Commit: strings.ToLower(columns[0]), FetchReference: reference, ETag: etag}, nil
}

func (m *RemoteManager) resolveGitHubRelease(ctx context.Context, source PackageSource, cached *ManagedPackageRegistration) (string, *string, error) {
	owner, repository, ok := githubRepository(source.URL)
	if !ok {
		return "", nil, errors.New("Release 选择方式目前只支持 github.com 仓库。")
	}
	path := fmt.Sprintf("repos/%s/%s/releases/latest", owner, repository)
	if source.Selector.Type == SelectorGitHubRelease {
		tag, err := requiredSelectorValue(source.Selector)
		if err != nil {
			return "", nil, err
		}
		path = fmt.Sprintf("repos/%s/%s/releases/tags/%s", owner, repository, url.PathEscape(tag))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/"+path, nil)
	if err != nil {
		return "", nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Codex-Tweaks")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if cached != nil && cached.RemoteETag != nil {
		request.Header.Set("If-None-Match", *cached.RemoteETag)
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && cached != nil && cached.LastResolvedReference != nil {
		return *cached.LastResolvedReference, cached.RemoteETag, nil
	}
	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("GitHub Release 查询失败（HTTP %d）。", response.StatusCode)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil || release.TagName == "" {
		return "", nil, errors.New("GitHub Release 返回内容无效。")
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		return release.TagName, nil, nil
	}
	return release.TagName, &etag, nil
}

func (m *RemoteManager) checkout(ctx context.Context, source PackageSource, resolution RemoteResolution, git GitEnvironment, destination string) error {
	commands := []struct {
		Arguments []string
		Name      string
	}{
		{[]string{"init", "--quiet", destination}, "git init"},
		{[]string{"-C", destination, "remote", "add", "origin", source.URL}, "git remote add"},
		{[]string{"-C", destination, "fetch", "--quiet", "--depth", "1", "--no-tags", "origin", resolution.FetchReference}, "git fetch"},
		{[]string{"-C", destination, "checkout", "--quiet", "--detach", "FETCH_HEAD"}, "git checkout"},
	}
	for _, command := range commands {
		result, err := m.runner.Run(ctx, git.GitPath, command.Arguments, m.store.ManagedPackagesDirectory, m.gitEnvironment())
		if err != nil {
			return err
		}
		if err := requireCommandSuccess(result, command.Name); err != nil {
			return err
		}
	}
	actualResult, err := m.gitCommand(ctx, git, []string{"-C", destination, "rev-parse", "HEAD"})
	if err != nil {
		return err
	}
	actual := strings.ToLower(strings.TrimSpace(actualResult.Output))
	if actual != strings.ToLower(resolution.Commit) {
		return fmt.Errorf("检出的 commit 与锁定结果不一致：期望 %s，实际为 %s。", resolution.Commit, actual)
	}
	return nil
}

func (m *RemoteManager) gitCommand(ctx context.Context, git GitEnvironment, arguments []string) (CommandResult, error) {
	result, err := m.runner.Run(ctx, git.GitPath, arguments, m.store.ManagedPackagesDirectory, m.gitEnvironment())
	if err != nil {
		return CommandResult{}, err
	}
	if result.Status != 0 {
		return CommandResult{}, requireCommandSuccess(result, "git "+firstString(arguments))
	}
	return result, nil
}

func (m *RemoteManager) gitEnvironment() []string {
	values := map[string]string{}
	for key, value := range m.environment {
		values[key] = value
	}
	values["GIT_TERMINAL_PROMPT"] = "0"
	if runtime.GOOS == "windows" {
		values["GIT_ASKPASS"] = ""
		values["GIT_SSH_COMMAND"] = "ssh -o BatchMode=yes"
	} else {
		values["GIT_ASKPASS"] = "/usr/bin/false"
		values["GIT_SSH_COMMAND"] = "ssh -o BatchMode=yes"
	}
	return environmentSlice(values)
}

func (m *RemoteManager) loadRegistry() (ManagedPackageRegistry, error) {
	registry := NewManagedPackageRegistry()
	if _, err := os.Stat(m.store.ManagedRegistryPath); errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err := readJSON(m.store.ManagedRegistryPath, &registry); err != nil {
		return ManagedPackageRegistry{}, err
	}
	if registry.SchemaVersion == 0 {
		registry.SchemaVersion = 1
	}
	if registry.Packages == nil {
		registry.Packages = map[string]ManagedPackageRegistration{}
	}
	return registry, nil
}

func (m *RemoteManager) writeRegistry(registry ManagedPackageRegistry) error {
	return writeJSONAtomic(m.store.ManagedRegistryPath, registry)
}

func (m *RemoteManager) writeLockfile(lockfile ManagedPackageLockfile) error {
	return writeJSONAtomic(m.store.ManagedLockfilePath, lockfile)
}

func (m *RemoteManager) addVersionRequirement(requirement, packageID string) error {
	registry, err := m.loadRegistry()
	if err != nil {
		return err
	}
	registration, exists := registry.Packages[packageID]
	if !exists || containsString(registration.VersionRequirements, requirement) {
		return nil
	}
	registration.VersionRequirements = append(registration.VersionRequirements, requirement)
	registry.Packages[packageID] = registration
	return m.writeRegistry(registry)
}

func requiredSelectorValue(selector RemoteSelector) (string, error) {
	if selector.Value == nil {
		return "", errors.New("远程来源缺少选择值。")
	}
	return *selector.Value, nil
}

func githubRepository(raw string) (string, string, bool) {
	value := strings.TrimSpace(raw)
	var path string
	if strings.HasPrefix(value, "git@github.com:") {
		path = strings.TrimPrefix(value, "git@github.com:")
	} else {
		parsed, err := url.Parse(value)
		if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
			return "", "", false
		}
		path = strings.Trim(parsed.Path, "/")
	}
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func isFullCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, item := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", item) {
			return false
		}
	}
	return true
}

func allRequirementsContain(requirements []SemanticVersionRequirement, version string) bool {
	for _, requirement := range requirements {
		if !requirement.Contains(version) {
			return false
		}
	}
	return true
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
