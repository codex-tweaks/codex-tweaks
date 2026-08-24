package core

import (
	"net/http"
	"sort"
)

type AppStatusKind string

const (
	StatusStarting        AppStatusKind = "starting"
	StatusLaunchingCodex  AppStatusKind = "launchingCodex"
	StatusCodexNotRunning AppStatusKind = "codexNotRunning"
	StatusWaitingForCDP   AppStatusKind = "waitingForCDP"
	StatusRestartRequired AppStatusKind = "restartRequired"
	StatusWaitingForPage  AppStatusKind = "waitingForPage"
	StatusConnected       AppStatusKind = "connected"
	StatusDisabled        AppStatusKind = "disabled"
	StatusError           AppStatusKind = "error"
)

type AppStatus struct {
	Kind        AppStatusKind `json:"kind"`
	TargetCount int           `json:"targetCount,omitempty"`
	Message     *string       `json:"message,omitempty"`
}

type InitializeParams struct {
	ApplicationSupportDirectory string `json:"applicationSupportDirectory,omitempty"`
	CacheDirectory              string `json:"cacheDirectory,omitempty"`
	BundledPackagesDirectory    string `json:"bundledPackagesDirectory,omitempty"`
	SkillPath                   string `json:"skillPath,omitempty"`
	CurrentVersion              string `json:"currentVersion"`
	BuildNumber                 string `json:"buildNumber"`
}

type AppConfiguration struct {
	SchemaVersion         int           `json:"schemaVersion"`
	Enabled               bool          `json:"enabled"`
	DeveloperMode         bool          `json:"developerMode"`
	KnownPackageIDs       *[]string     `json:"knownPackageIDs"`
	DisabledPackageIDs    []string      `json:"disabledPackageIDs"`
	UpdateChannel         UpdateChannel `json:"updateChannel"`
	UpdateAutoCheck       bool          `json:"updateAutoCheck"`
	UpdateLastCheckAt     *CodableTime  `json:"updateLastCheckAt,omitempty"`
	UpdateSkippedVersions []string      `json:"updateSkippedVersions"`
}

type PackageView struct {
	ID                            string                       `json:"id"`
	DirectoryName                 string                       `json:"directoryName"`
	Directory                     string                       `json:"directory"`
	ExportFileName                string                       `json:"exportFileName"`
	Manifest                      *PackageManifest             `json:"manifest,omitempty"`
	SourceFingerprint             *string                      `json:"sourceFingerprint,omitempty"`
	DependencyFingerprint         *string                      `json:"dependencyFingerprint,omitempty"`
	ActiveBuild                   *ActivePackageBuild          `json:"activeBuild,omitempty"`
	ValidationError               *string                      `json:"validationError,omitempty"`
	PriorityOverride              *int                         `json:"priorityOverride,omitempty"`
	Origin                        PackageOrigin                `json:"origin"`
	DisplayName                   string                       `json:"displayName"`
	DisplayVersion                string                       `json:"displayVersion"`
	Detail                        string                       `json:"detail"`
	DeclaredPriority              int                          `json:"declaredPriority"`
	Priority                      int                          `json:"priority"`
	HasDependencies               bool                         `json:"hasDependencies"`
	PackageDependencies           map[string]PackageDependency `json:"packageDependencies"`
	RuntimePackageDependencies    map[string]string            `json:"runtimePackageDependencies"`
	IsManaged                     bool                         `json:"isManaged"`
	ManagedLock                   *ManagedPackageLock          `json:"managedLock,omitempty"`
	BuildDisposition              BuildDisposition             `json:"buildDisposition"`
	BuildRequestKey               *string                      `json:"buildRequestKey,omitempty"`
	CanInstallMissingDependencies bool                         `json:"canInstallMissingDependencies"`
	CanEnableDependencies         bool                         `json:"canEnableDependencies"`
	AvailableActions              PackageAvailableActions      `json:"availableActions"`
	Presentation                  PackagePresentation          `json:"presentation"`
}

type PackagePresentation struct {
	StatusTitle  string `json:"statusTitle"`
	StatusDetail string `json:"statusDetail"`
	StatusTone   string `json:"statusTone"`
	IsError      bool   `json:"isError"`
	IsPending    bool   `json:"isPending"`
}

type PackageAvailableActions struct {
	SetEnabled                 bool `json:"setEnabled"`
	SetPriority                bool `json:"setPriority"`
	OpenDirectory              bool `json:"openDirectory"`
	Export                     bool `json:"export"`
	InstallMissingDependencies bool `json:"installMissingDependencies"`
	EnableDependencies         bool `json:"enableDependencies"`
	UpdateManagedPackage       bool `json:"updateManagedPackage"`
	Build                      bool `json:"build"`
}

type UpdateSnapshot struct {
	Channel                UpdateChannel  `json:"channel"`
	AutoCheck              bool           `json:"autoCheck"`
	Checking               bool           `json:"checking"`
	LatestRelease          *GitHubRelease `json:"latestRelease,omitempty"`
	LastError              *string        `json:"lastError,omitempty"`
	LastCheckAt            *CodableTime   `json:"lastCheckAt,omitempty"`
	PendingUpdate          *GitHubRelease `json:"pendingUpdate,omitempty"`
	CurrentVersion         string         `json:"currentVersion"`
	BuildNumber            string         `json:"buildNumber"`
	HasNewerVersion        bool           `json:"hasNewerVersion"`
	UpdateAvailable        bool           `json:"updateAvailable"`
	LatestVersionString    string         `json:"latestVersionString"`
	LatestVersionIsSkipped bool           `json:"latestVersionIsSkipped"`
	DownloadURL            *string        `json:"downloadURL,omitempty"`
}

type AppSnapshot struct {
	ProtocolVersion            int                           `json:"protocolVersion"`
	Presentation               PresentationContract          `json:"presentation"`
	Status                     AppStatus                     `json:"status"`
	Enabled                    bool                          `json:"enabled"`
	DeveloperMode              bool                          `json:"developerMode"`
	Packages                   []PackageView                 `json:"packages"`
	DisabledPackageIDs         []string                      `json:"disabledPackageIDs"`
	BuildingPackageIDs         []string                      `json:"buildingPackageIDs"`
	ExportingPackageIDs        []string                      `json:"exportingPackageIDs"`
	PackageBuildErrors         map[string]string             `json:"packageBuildErrors"`
	PackageRuntimeErrors       map[string]string             `json:"packageRuntimeErrors"`
	PackagePayloadErrors       map[string]string             `json:"packagePayloadErrors"`
	PackageDependencyStatuses  map[string][]DependencyStatus `json:"packageDependencyStatuses"`
	PackageDependencyIssues    map[string][]string           `json:"packageDependencyIssues"`
	PackagePriorityConstraints map[string]PriorityConstraint `json:"packagePriorityConstraints"`
	NodeEnvironment            *NodeEnvironment              `json:"nodeEnvironment,omitempty"`
	CheckingNode               bool                          `json:"checkingNode"`
	GitEnvironment             *GitEnvironment               `json:"gitEnvironment,omitempty"`
	CheckingGit                bool                          `json:"checkingGit"`
	CheckingRemoteUpdates      bool                          `json:"checkingRemoteUpdates"`
	RemotePackageUpdates       map[string]RemoteUpdate       `json:"remotePackageUpdates"`
	RemotePackageErrors        map[string]string             `json:"remotePackageErrors"`
	InstallingPackageIDs       []string                      `json:"installingPackageIDs"`
	InstallingRemotePackage    bool                          `json:"installingRemotePackage"`
	RemoteOperationMessage     *string                       `json:"remoteOperationMessage,omitempty"`
	RemoteOperationError       *string                       `json:"remoteOperationError,omitempty"`
	InstallingLocalPackage     bool                          `json:"installingLocalPackage"`
	LocalOperationMessage      *string                       `json:"localOperationMessage,omitempty"`
	LocalOperationError        *string                       `json:"localOperationError,omitempty"`
	TweaksDirectory            string                        `json:"tweaksDirectory"`
	PackagesDirectory          string                        `json:"packagesDirectory"`
	LogPath                    string                        `json:"logPath"`
	LogText                    string                        `json:"logText"`
	EnabledPackageCount        int                           `json:"enabledPackageCount"`
	ActivePackageCount         int                           `json:"activePackageCount"`
	Update                     UpdateSnapshot                `json:"update"`
}

type ControllerDependencies struct {
	Runner            CommandRunner
	HTTPClient        *http.Client
	Platform          Platform
	Logger            *Logger
	CDP               *CDPService
	DisableBackground bool
}

func sortedTrueKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key, included := range values {
		if included {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
