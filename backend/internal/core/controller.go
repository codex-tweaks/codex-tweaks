package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ProtocolVersion = 9

type Controller struct {
	mu                  sync.Mutex
	refreshMu           sync.Mutex
	eventMu             sync.Mutex
	ctx                 context.Context
	cancel              context.CancelFunc
	event               func(AppSnapshot)
	lastEmittedSnapshot *AppSnapshot

	store              *Store
	logger             *Logger
	builder            *Builder
	nodeRuntime        *NodeRuntimeSupervisor
	remote             *RemoteManager
	installer          *LocalInstaller
	exporter           *PackageExporter
	cdp                *CDPService
	platform           Platform
	updates            *UpdateService
	configPath         string
	skillPath          string
	currentVersion     string
	buildNumber        string
	preferredLanguages []string
	disableBackground  bool

	config                       AppConfiguration
	status                       AppStatus
	packages                     []Package
	disabledPackageIDs           map[string]bool
	buildingPackageIDs           map[string]bool
	exportingPackageIDs          map[string]bool
	deletingPackageIDs           map[string]bool
	packageBuildErrors           map[string]string
	packageBuildErrorRequestKeys map[string]string
	packageRuntimeErrors         map[string]string
	packagePayloadErrors         map[string]string
	packageDependencyStatuses    map[string][]DependencyStatus
	packageDependencyIssues      map[string][]string
	packagePriorityConstraints   map[string]PriorityConstraint
	nodeEnvironment              *NodeEnvironment
	checkingNode                 bool
	nodeAuthorizations           map[string]NodeAuthorizationRecord
	developerAllowUnknownNode    bool
	gitEnvironment               *GitEnvironment
	checkingGit                  bool
	checkingRemoteUpdates        bool
	remotePackageUpdates         map[string]RemoteUpdate
	remotePackageErrors          map[string]string
	installingPackageIDs         map[string]bool
	installingRemotePackage      bool
	remoteOperationMessage       *string
	remoteOperationError         *string
	installingLocalPackage       bool
	localOperationMessage        *string
	localOperationError          *string
	logText                      string

	forceGeneration            int
	restartingCodexUI          bool
	developerBuildAttemptKeys  map[string]string
	disabledCleanupCompleted   bool
	hasAttemptedInitialLaunch  bool
	lastAutomaticRemoteCheckAt time.Time

	updateChecking  bool
	latestRelease   *GitHubRelease
	updateLastError *string
	pendingUpdate   *GitHubRelease
}

func NewController(params InitializeParams, event func(AppSnapshot), dependencies ControllerDependencies) (*Controller, error) {
	store, err := NewStore(params.ApplicationSupportDirectory, params.CacheDirectory, params.BundledPackagesDirectory)
	if err != nil {
		return nil, err
	}
	applicationSupport := params.ApplicationSupportDirectory
	if applicationSupport == "" {
		applicationSupport = filepath.Dir(filepath.Dir(store.StateDirectory))
	}
	logger := dependencies.Logger
	if logger == nil {
		logger, err = NewLogger(applicationSupport)
		if err != nil {
			return nil, err
		}
	}
	platform := dependencies.Platform
	if platform == nil {
		platform = NewPlatform(dependencies.Runner)
	}
	cdp := dependencies.CDP
	if cdp == nil {
		cdp = NewCDPService(logger)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if repairable, ok := platform.(backgroundRepairPlatform); ok {
		repairable.useBackgroundRepairContext(ctx, logger)
	}
	controller := &Controller{
		ctx: ctx, cancel: cancel, event: event, store: store, logger: logger,
		builder:   NewBuilder(store, dependencies.Runner),
		remote:    NewRemoteManager(store, dependencies.Runner, dependencies.HTTPClient, nil),
		installer: NewLocalInstaller(store), exporter: NewPackageExporter(), cdp: cdp, platform: platform,
		updates:    NewUpdateService(dependencies.HTTPClient),
		configPath: filepath.Join(store.StateDirectory, "app-state.json"), skillPath: params.SkillPath,
		currentVersion: normalizedInstalledVersion(params.CurrentVersion), buildNumber: params.BuildNumber,
		preferredLanguages: append([]string(nil), params.PreferredLanguages...),
		disableBackground:  dependencies.DisableBackground,
		status:             AppStatus{Kind: StatusStarting},
		disabledPackageIDs: map[string]bool{}, buildingPackageIDs: map[string]bool{},
		exportingPackageIDs: map[string]bool{}, deletingPackageIDs: map[string]bool{},
		packageBuildErrors: map[string]string{}, packageBuildErrorRequestKeys: map[string]string{},
		packageRuntimeErrors: map[string]string{}, packagePayloadErrors: map[string]string{},
		packageDependencyStatuses: map[string][]DependencyStatus{}, packageDependencyIssues: map[string][]string{},
		packagePriorityConstraints: map[string]PriorityConstraint{}, remotePackageUpdates: map[string]RemoteUpdate{},
		remotePackageErrors: map[string]string{}, installingPackageIDs: map[string]bool{},
		developerBuildAttemptKeys: map[string]string{},
		nodeAuthorizations:        map[string]NodeAuthorizationRecord{},
	}
	if err := controller.loadConfiguration(); err != nil {
		cancel()
		return nil, err
	}
	if err := store.Prepare(); err != nil {
		cancel()
		return nil, err
	}
	authorizations, err := store.LoadNodeAuthorizations()
	if err != nil {
		cancel()
		return nil, err
	}
	controller.nodeAuthorizations = authorizations
	controller.nodeRuntime = NewNodeRuntimeSupervisor(store, dependencies.Runner, logger)
	controller.nodeRuntime.SetEventHandler(cdp.EmitNodeEvent)
	cdp.SetNodeInvoker(controller.nodeRuntime)
	if err := controller.updatePackages(); err != nil {
		message := "无法准备功能包：" + err.Error()
		controller.status = AppStatus{Kind: StatusError, Message: &message}
		logger.Error("准备功能包失败：" + err.Error())
	}
	controller.refreshLog()
	logger.Info("Codex Tweaks Go 后端已启动")
	controller.emit()
	if !dependencies.DisableBackground {
		go controller.monitor()
		controller.CheckNodeEnvironment()
		controller.CheckGitEnvironment()
		if controller.config.UpdateAutoCheck {
			controller.CheckAppUpdate(true)
		}
	}
	return controller, nil
}

func normalizedInstalledVersion(value string) string {
	if _, ok := ParseSemanticVersion(value); ok {
		return NormalizeVersion(value)
	}
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func (c *Controller) loadConfiguration() error {
	configuration := AppConfiguration{}
	if _, err := os.Stat(c.configPath); err == nil {
		if err := readJSON(c.configPath, &configuration); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		configuration = AppConfiguration{
			SchemaVersion:         1,
			Enabled:               true,
			DisabledPackageIDs:    []string{},
			Language:              LanguageAuto,
			UpdateChannel:         UpdateStable,
			UpdateAutoCheck:       true,
			UpdateSkippedVersions: []string{},
		}
	}
	if configuration.SchemaVersion != 1 {
		return fmt.Errorf("unsupported configuration schema version %d", configuration.SchemaVersion)
	}
	if configuration.UpdateChannel != UpdateBeta {
		configuration.UpdateChannel = UpdateStable
	}
	configuration.Language = NormalizeAppLanguage(configuration.Language)
	configuration.DisabledPackageIDs = uniqueSorted(configuration.DisabledPackageIDs)
	configuration.UpdateSkippedVersions = uniqueSorted(configuration.UpdateSkippedVersions)
	c.config = configuration
	c.disabledPackageIDs = stringSet(configuration.DisabledPackageIDs)
	return c.persistConfigurationLocked()
}

func (c *Controller) persistConfigurationLocked() error {
	c.config.DisabledPackageIDs = sortedTrueKeys(c.disabledPackageIDs)
	return writeJSONAtomic(c.configPath, c.config)
}

func (c *Controller) presentationText() map[string]string {
	c.mu.Lock()
	locale := ResolveAppLanguage(c.config.Language, c.preferredLanguages)
	c.mu.Unlock()
	return PresentationTextForLocale(locale)
}

func (c *Controller) monitor() {
	c.Refresh()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.Refresh()
		}
	}
}

func (c *Controller) Snapshot() AppSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	installedIDs := map[string]bool{}
	for _, pkg := range c.packages {
		if pkg.Manifest != nil {
			installedIDs[pkg.Manifest.Name] = true
		}
	}
	packageViews := make([]PackageView, 0, len(c.packages))
	locale := ResolveAppLanguage(c.config.Language, c.preferredLanguages)
	presentationText := PresentationTextForLocale(locale)
	enabledCount := 0
	for _, pkg := range c.packages {
		view := packageView(pkg, c.disabledPackageIDs, installedIDs)
		view.Node = c.packageNodeViewLocked(pkg)
		remoteUpdate, hasRemoteUpdate := c.remotePackageUpdates[pkg.ID]
		hasPackageExport := len(c.exportingPackageIDs) > 0
		deletingAnyPackage := len(c.deletingPackageIDs) > 0
		packageMutationBusy := c.installingLocalPackage || c.installingRemotePackage ||
			hasPackageExport || deletingAnyPackage || len(c.installingPackageIDs) > 0 ||
			len(c.buildingPackageIDs) > 0 || c.checkingRemoteUpdates
		view.AvailableActions = PackageAvailableActions{
			SetEnabled:                 !deletingAnyPackage,
			SetPriority:                !deletingAnyPackage,
			OpenDirectory:              !c.deletingPackageIDs[pkg.ID],
			Export:                     !hasPackageExport && !c.installingLocalPackage && !c.installingRemotePackage && !deletingAnyPackage,
			Delete:                     !packageMutationBusy,
			InstallMissingDependencies: view.CanInstallMissingDependencies && c.gitEnvironment != nil && !c.installingPackageIDs[pkg.ID] && !deletingAnyPackage,
			EnableDependencies:         view.CanEnableDependencies && !deletingAnyPackage,
			UpdateManagedPackage:       hasRemoteUpdate && remoteUpdate.Installable() && c.gitEnvironment != nil && !c.installingPackageIDs[pkg.ID] && !deletingAnyPackage,
			Build:                      view.ValidationError == nil && view.BuildRequestKey != nil && c.nodeEnvironment != nil && !c.buildingPackageIDs[pkg.ID] && !deletingAnyPackage,
			AuthorizeNode:              view.Node != nil && view.Node.AuthorizationID != "" && !view.Node.Authorized && !deletingAnyPackage,
		}
		view.Presentation = c.packagePresentation(view, presentationText)
		packageViews = append(packageViews, view)
		if !c.disabledPackageIDs[pkg.ID] {
			enabledCount++
		}
	}
	activeCount := 0
	runningNodePackages := map[string]bool{}
	if c.nodeRuntime != nil {
		runningNodePackages = c.nodeRuntime.RunningPackageIDs()
	}
	effectiveActiveDisabled := cloneSet(c.disabledPackageIDs)
	for _, pkg := range c.packages {
		if pkg.Manifest != nil && pkg.Manifest.CodexTweaks.Permissions.Node != nil && !runningNodePackages[pkg.ID] {
			effectiveActiveDisabled[pkg.ID] = true
		}
	}
	for range ResolveDependencies(c.packages, effectiveActiveDisabled).LoadablePackages {
		activeCount++
	}
	updateSnapshot := c.updateSnapshotLocked()
	presentation := NewPresentationContract(PresentationState{
		LanguagePreference: c.config.Language, PreferredLanguages: c.preferredLanguages,
		Status: c.status, Enabled: c.config.Enabled,
		CheckingNode: c.checkingNode, CheckingGit: c.checkingGit,
		RestartingCodexUI:      c.restartingCodexUI,
		CheckingRemoteUpdates:  c.checkingRemoteUpdates,
		InstallingLocalPackage: c.installingLocalPackage, InstallingRemotePackage: c.installingRemotePackage,
		ExportingPackage: len(c.exportingPackageIDs) > 0,
		DeletingPackage:  len(c.deletingPackageIDs) > 0,
		GitAvailable:     c.gitEnvironment != nil, LogAvailable: strings.TrimSpace(c.logText) != "",
		AuthoringPromptAvailable: c.skillPath != "",
		UpdateChecking:           c.updateChecking, UpdateAvailable: updateSnapshot.UpdateAvailable,
	})
	return AppSnapshot{
		ProtocolVersion: ProtocolVersion, Presentation: presentation, Status: c.status,
		Enabled: c.config.Enabled, DeveloperMode: c.config.DeveloperMode,
		DeveloperAllowUnknownNode: c.developerAllowUnknownNode, Packages: packageViews,
		DisabledPackageIDs: sortedTrueKeys(c.disabledPackageIDs), BuildingPackageIDs: sortedTrueKeys(c.buildingPackageIDs),
		ExportingPackageIDs: sortedTrueKeys(c.exportingPackageIDs),
		PackageBuildErrors:  cloneStringMap(c.packageBuildErrors), PackageRuntimeErrors: cloneStringMap(c.packageRuntimeErrors),
		PackagePayloadErrors:       cloneStringMap(c.packagePayloadErrors),
		PackageDependencyStatuses:  cloneDependencyStatuses(c.packageDependencyStatuses),
		PackageDependencyIssues:    cloneStringSlices(c.packageDependencyIssues),
		PackagePriorityConstraints: clonePriorityConstraints(c.packagePriorityConstraints),
		NodeEnvironment:            cloneNodeEnvironment(c.nodeEnvironment), CheckingNode: c.checkingNode,
		GitEnvironment: cloneGitEnvironment(c.gitEnvironment), CheckingGit: c.checkingGit,
		CheckingRemoteUpdates: c.checkingRemoteUpdates, RemotePackageUpdates: cloneRemoteUpdates(c.remotePackageUpdates),
		RemotePackageErrors: cloneStringMap(c.remotePackageErrors), InstallingPackageIDs: sortedTrueKeys(c.installingPackageIDs),
		InstallingRemotePackage: c.installingRemotePackage, RemoteOperationMessage: cloneStringPointer(c.remoteOperationMessage),
		RemoteOperationError: cloneStringPointer(c.remoteOperationError), InstallingLocalPackage: c.installingLocalPackage,
		LocalOperationMessage: cloneStringPointer(c.localOperationMessage), LocalOperationError: cloneStringPointer(c.localOperationError),
		TweaksDirectory: c.store.TweaksDirectory, PackagesDirectory: c.store.PackagesDirectory,
		LogPath: c.logger.Path, LogText: c.logText, EnabledPackageCount: enabledCount, ActivePackageCount: activeCount,
		Update: updateSnapshot,
	}
}

func (c *Controller) packagePresentation(view PackageView, text map[string]string) PackagePresentation {
	packageID := view.ID
	remoteUpdate, hasRemoteUpdate := c.remotePackageUpdates[packageID]
	dependencyIssues := c.packageDependencyIssues[packageID]
	criticalDependency := false
	for _, dependency := range c.packageDependencyStatuses[packageID] {
		switch dependency.State.Kind {
		case DependencySourceConflict, DependencyCycle, DependencyInvalidRequirement, DependencySelfReference:
			criticalDependency = true
		}
	}
	hasError := view.ValidationError != nil || c.packageBuildErrors[packageID] != "" ||
		c.packagePayloadErrors[packageID] != "" || c.packageRuntimeErrors[packageID] != "" ||
		c.remotePackageErrors[packageID] != "" || criticalDependency
	isPending := hasRemoteUpdate && remoteUpdate.Status != RemoteUpdateCurrent || len(dependencyIssues) > 0
	if view.Node != nil && !view.Node.Authorized {
		isPending = true
	}
	switch view.BuildDisposition {
	case BuildNotBuilt, BuildVersionUpdate, BuildDependencyUpdate, BuildSourceChanged, BuildCompilerUpdate:
		isPending = true
	}

	result := PackagePresentation{IsError: hasError, IsPending: isPending, StatusTone: "warning"}
	switch {
	case c.deletingPackageIDs[packageID]:
		result.StatusTitle = text["packages.status.deleting"]
	case c.installingPackageIDs[packageID]:
		result.StatusTitle = text["packages.status.installingRemote"]
	case c.buildingPackageIDs[packageID]:
		result.StatusTitle = text["packages.status.building"]
	case c.exportingPackageIDs[packageID]:
		result.StatusTitle = text["packages.status.exporting"]
	case view.ValidationError != nil:
		result.StatusTitle = text["packages.status.invalid"]
	case c.remotePackageErrors[packageID] != "":
		result.StatusTitle = text["packages.status.remoteFailed"]
	case hasRemoteUpdate && remoteUpdate.Status == RemoteUpdatePinnedReferenceChanged:
		result.StatusTitle = text["packages.status.pinnedChanged"]
	case hasRemoteUpdate && remoteUpdate.Status == RemoteUpdateAvailable:
		result.StatusTitle = text["packages.status.remoteUpdate"]
	case len(dependencyIssues) > 0:
		result.StatusTitle = text["packages.status.dependencyBlocked"]
	case c.packageBuildErrors[packageID] != "":
		result.StatusTitle = text["packages.status.buildFailed"]
	case c.packagePayloadErrors[packageID] != "":
		result.StatusTitle = text["packages.status.payloadFailed"]
	case c.packageRuntimeErrors[packageID] != "":
		result.StatusTitle = text["packages.status.runtimeFailed"]
	case c.disabledPackageIDs[packageID]:
		result.StatusTitle = text["packages.status.disabled"]
	case view.Node != nil && view.Node.Status == "pendingAuthorization":
		result.StatusTitle = text["packages.status.nodeAuthorizationRequired"]
	default:
		result.StatusTitle = map[BuildDisposition]string{
			BuildInvalid:          text["packages.status.unavailable"],
			BuildNotBuilt:         text["packages.status.notBuilt"],
			BuildCurrent:          text["packages.status.active"],
			BuildVersionUpdate:    text["packages.status.newVersion"],
			BuildDependencyUpdate: text["packages.status.dependencyChanged"],
			BuildSourceChanged:    text["packages.status.sourceChanged"],
			BuildCompilerUpdate:   text["packages.status.compilerChanged"],
		}[view.BuildDisposition]
	}

	switch {
	case c.deletingPackageIDs[packageID]:
		result.StatusDetail = text["packages.detail.deleting"]
	case c.exportingPackageIDs[packageID]:
		result.StatusDetail = text["packages.detail.exporting"]
	case view.ValidationError != nil:
		result.StatusDetail = *view.ValidationError
	case c.remotePackageErrors[packageID] != "":
		result.StatusDetail = c.remotePackageErrors[packageID]
	case hasRemoteUpdate && remoteUpdate.Status == RemoteUpdateAvailable:
		commit := remoteUpdate.CandidateCommit
		if len(commit) > 12 {
			commit = commit[:12]
		}
		result.StatusDetail = resolvePresentationText(text, "packages.detail.remoteAvailable", map[string]string{
			"reference": remoteUpdate.CandidateReference,
			"commit":    commit,
		})
	case hasRemoteUpdate && remoteUpdate.Status == RemoteUpdatePinnedReferenceChanged:
		result.StatusDetail = text["packages.detail.pinnedChanged"]
	case len(dependencyIssues) > 0:
		result.StatusDetail = resolvePresentationText(text, "packages.detail.dependencyIssues", map[string]string{
			"count": strconv.Itoa(len(dependencyIssues)),
		})
	case c.packageBuildErrors[packageID] != "":
		result.StatusDetail = c.packageBuildErrors[packageID]
		if view.ActiveBuild != nil {
			result.StatusDetail = resolvePresentationText(text, "packages.detail.activeBuildError", map[string]string{
				"version": view.ActiveBuild.Record.PackageVersion,
				"message": result.StatusDetail,
			})
		}
	case c.packagePayloadErrors[packageID] != "":
		result.StatusDetail = c.packagePayloadErrors[packageID]
	case c.packageRuntimeErrors[packageID] != "":
		result.StatusDetail = c.packageRuntimeErrors[packageID]
	case view.Node != nil && view.Node.Status == "pendingAuthorization":
		result.StatusDetail = text["packages.detail.nodeAuthorizationRequired"]
	case view.ActiveBuild == nil:
		result.StatusDetail = text["packages.notBuiltDetail"]
	case view.BuildDisposition == BuildVersionUpdate:
		result.StatusDetail = resolvePresentationText(text, "packages.detail.versionUpdate", map[string]string{
			"current": view.ActiveBuild.Record.PackageVersion,
			"next":    view.DisplayVersion,
		})
	case view.BuildDisposition == BuildDependencyUpdate:
		result.StatusDetail = text["packages.detail.dependencyUpdate"]
	case view.BuildDisposition == BuildSourceChanged:
		result.StatusDetail = text["packages.detail.sourceChanged"]
	case view.BuildDisposition == BuildCompilerUpdate:
		result.StatusDetail = resolvePresentationText(text, "packages.detail.compilerUpdate", map[string]string{
			"version": view.ActiveBuild.Record.CompilerVersion,
		})
	default:
		result.StatusDetail = resolvePresentationText(text, "packages.detail.lastBuilt", map[string]string{
			"date": view.ActiveBuild.Record.BuiltAt.Local().Format("2006-01-02 15:04"),
		})
	}

	switch {
	case hasError:
		result.StatusTone = "danger"
	case hasRemoteUpdate && remoteUpdate.Status == RemoteUpdatePinnedReferenceChanged,
		hasRemoteUpdate && remoteUpdate.Status == RemoteUpdateAvailable,
		len(dependencyIssues) > 0:
		result.StatusTone = "warning"
	case c.installingPackageIDs[packageID] || c.buildingPackageIDs[packageID] ||
		c.exportingPackageIDs[packageID] || c.deletingPackageIDs[packageID]:
		result.StatusTone = "accent"
	case c.disabledPackageIDs[packageID]:
		result.StatusTone = "neutral"
	case view.BuildDisposition == BuildCurrent:
		result.StatusTone = "success"
	}
	return result
}

func packageView(pkg Package, disabled, installed map[string]bool) PackageView {
	detail := "没有提供包说明。"
	if pkg.Manifest != nil && pkg.Manifest.Description != "" {
		detail = pkg.Manifest.Description
	}
	requestKey, hasRequestKey := pkg.BuildRequestKey(CompilerVersion)
	var requestKeyPointer *string
	if hasRequestKey {
		requestKeyPointer = &requestKey
	}
	canInstall := false
	for packageID, dependency := range pkg.PackageDependencies() {
		if !installed[packageID] && dependency.Source != nil {
			canInstall = true
			break
		}
	}
	canEnable := false
	for packageID := range pkg.RuntimePackageDependencies() {
		if disabled[packageID] {
			canEnable = true
			break
		}
	}
	var projectPageURL *string
	if lock := pkg.ManagedLock(); lock != nil {
		projectPageURL = repositoryProjectPageURL(lock.Source.URL)
	}
	return PackageView{
		ID: pkg.ID, DirectoryName: pkg.DirectoryName, Directory: pkg.Directory,
		ExportFileName: PackageArchiveFileName(pkg), Manifest: pkg.Manifest,
		SourceFingerprint: pkg.SourceFingerprint, DependencyFingerprint: pkg.DependencyFingerprint,
		ActiveBuild: pkg.ActiveBuild, ValidationError: pkg.ValidationError, PriorityOverride: pkg.PriorityOverride,
		Origin: pkg.Origin, DisplayName: pkg.DisplayName(), DisplayVersion: pkg.Version(), Detail: detail,
		DeclaredPriority: pkg.DeclaredPriority(), Priority: pkg.Priority(),
		HasDependencies:     pkg.Manifest != nil && len(pkg.Manifest.Dependencies) > 0,
		PackageDependencies: pkg.PackageDependencies(), RuntimePackageDependencies: pkg.RuntimePackageDependencies(),
		IsManaged: pkg.Origin.Kind == OriginManaged, ManagedLock: pkg.ManagedLock(), ProjectPageURL: projectPageURL,
		BuildDisposition: pkg.BuildDisposition(CompilerVersion), BuildRequestKey: requestKeyPointer,
		CanInstallMissingDependencies: canInstall, CanEnableDependencies: canEnable,
	}
}

func (c *Controller) updateSnapshotLocked() UpdateSnapshot {
	latestVersion := "-"
	if c.latestRelease != nil {
		latestVersion = NormalizeVersion(c.latestRelease.TagName)
	}
	hasNewer := HasNewerVersion(c.latestRelease, c.currentVersion)
	skipped := false
	if c.latestRelease != nil {
		skipped = containsString(c.config.UpdateSkippedVersions, NormalizeVersion(c.latestRelease.TagName))
	}
	return UpdateSnapshot{
		Channel: c.config.UpdateChannel, PackageChannel: packageChannelForRelease(c.latestRelease),
		AutoCheck: c.config.UpdateAutoCheck, Checking: c.updateChecking,
		LatestRelease: cloneRelease(c.latestRelease), LastError: cloneStringPointer(c.updateLastError),
		LastCheckAt: cloneTimePointer(c.config.UpdateLastCheckAt), PendingUpdate: cloneRelease(c.pendingUpdate),
		CurrentVersion: c.currentVersion, BuildNumber: c.buildNumber, HasNewerVersion: hasNewer,
		UpdateAvailable: hasNewer && !skipped, LatestVersionString: latestVersion,
		LatestVersionIsSkipped: skipped, DownloadURL: preferredDownloadForRelease(c.latestRelease, c.platform.Architecture()),
	}
}

func preferredDownloadForRelease(release *GitHubRelease, architecture string) *string {
	if release == nil {
		return nil
	}
	return PreferredDownloadURL(*release, "", architecture)
}

func (c *Controller) emit() {
	if c.event == nil {
		return
	}
	snapshot := c.Snapshot()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	if c.lastEmittedSnapshot != nil && reflect.DeepEqual(*c.lastEmittedSnapshot, snapshot) {
		return
	}
	c.lastEmittedSnapshot = &snapshot
	c.event(snapshot)
}

func (c *Controller) refreshLog() {
	contents, err := c.logger.ReadPreviewNewestFirst()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.logText = "日志读取失败：" + err.Error()
	} else {
		c.logText = contents
	}
}

func (c *Controller) RefreshLog() {
	c.refreshLog()
	c.emit()
}

func (c *Controller) ClearLog() error {
	if err := c.logger.Clear(); err != nil {
		return err
	}
	c.mu.Lock()
	c.logText = ""
	c.mu.Unlock()
	c.emit()
	return nil
}

func (c *Controller) ReadAuthoringPrompt() (string, error) {
	if c.skillPath == "" {
		return "", errors.New("无法读取 Codex Tweaks 功能包开发 Skill。")
	}
	contents, err := os.ReadFile(c.skillPath)
	if err != nil {
		return "", err
	}
	return string(contents), nil
}

func (c *Controller) Shutdown() {
	if c.nodeRuntime != nil {
		c.nodeRuntime.StopAll()
	}
	c.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	_ = c.cdp.CleanupAllTargets(ctx)
	c.logger.Info("Codex Tweaks Go 后端已退出")
}

func uniqueSorted(values []string) []string {
	set := stringSet(values)
	return sortedTrueKeys(set)
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneNodeEnvironment(value *NodeEnvironment) *NodeEnvironment {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneGitEnvironment(value *GitEnvironment) *GitEnvironment {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneRelease(value *GitHubRelease) *GitHubRelease {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Assets = append([]GitHubAsset{}, value.Assets...)
	return &copy
}

func cloneTimePointer(value *CodableTime) *CodableTime {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDependencyStatuses(source map[string][]DependencyStatus) map[string][]DependencyStatus {
	result := make(map[string][]DependencyStatus, len(source))
	for key, value := range source {
		result[key] = append([]DependencyStatus{}, value...)
	}
	return result
}

func cloneStringSlices(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for key, value := range source {
		result[key] = append([]string{}, value...)
	}
	return result
}

func clonePriorityConstraints(source map[string]PriorityConstraint) map[string]PriorityConstraint {
	result := make(map[string]PriorityConstraint, len(source))
	for key, value := range source {
		value.MustLoadAfterPackageIDs = append([]string{}, value.MustLoadAfterPackageIDs...)
		value.MustLoadBeforePackageIDs = append([]string{}, value.MustLoadBeforePackageIDs...)
		result[key] = value
	}
	return result
}

func cloneRemoteUpdates(source map[string]RemoteUpdate) map[string]RemoteUpdate {
	result := make(map[string]RemoteUpdate, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func mapsEqual(left, right map[string]bool) bool { return reflect.DeepEqual(left, right) }

func sortedPackageIDs(packages []Package) []string {
	result := make([]string, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, pkg.ID)
	}
	sort.Strings(result)
	return result
}
