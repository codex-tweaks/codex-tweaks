using System.Text.Json.Serialization;
using CodexTweaks.Windows.Generated;

namespace CodexTweaks.Windows.Models;

internal sealed class BackendPing
{
    [JsonPropertyName("protocolVersion")]
    public int ProtocolVersion { get; init; }

    [JsonPropertyName("backend")]
    public string Backend { get; init; } = string.Empty;
}

internal sealed class BackendAppStatus
{
    [JsonPropertyName("kind")]
    public string Kind { get; init; } = "starting";

    [JsonPropertyName("targetCount")]
    public int TargetCount { get; init; }

    [JsonPropertyName("message")]
    public string? Message { get; init; }
}

internal sealed class NodeEnvironment
{
    [JsonPropertyName("nodePath")]
    public string NodePath { get; init; } = string.Empty;

    [JsonPropertyName("version")]
    public string Version { get; init; } = string.Empty;
}

internal sealed class GitEnvironment
{
    [JsonPropertyName("gitPath")]
    public string GitPath { get; init; } = string.Empty;

    [JsonPropertyName("version")]
    public string Version { get; init; } = string.Empty;
}

internal sealed class PackageView
{
    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("directory")]
    public string Directory { get; init; } = string.Empty;

    [JsonPropertyName("exportFileName")]
    public string ExportFileName { get; init; } = string.Empty;

    [JsonPropertyName("displayName")]
    public string DisplayName { get; init; } = string.Empty;

    [JsonPropertyName("displayVersion")]
    public string DisplayVersion { get; init; } = string.Empty;

    [JsonPropertyName("detail")]
    public string Detail { get; init; } = string.Empty;

    [JsonPropertyName("validationError")]
    public string? ValidationError { get; init; }

    [JsonPropertyName("priorityOverride")]
    public int? PriorityOverride { get; init; }

    [JsonPropertyName("declaredPriority")]
    public int DeclaredPriority { get; init; }

    [JsonPropertyName("priority")]
    public int Priority { get; init; }

    [JsonPropertyName("isManaged")]
    public bool IsManaged { get; init; }

    [JsonPropertyName("projectPageURL")]
    public string? ProjectPageUrl { get; init; }

    [JsonPropertyName("buildDisposition")]
    public string BuildDisposition { get; init; } = string.Empty;

    [JsonPropertyName("hasDependencies")]
    public bool HasDependencies { get; init; }

    [JsonPropertyName("canInstallMissingDependencies")]
    public bool CanInstallMissingDependencies { get; init; }

    [JsonPropertyName("canEnableDependencies")]
    public bool CanEnableDependencies { get; init; }

    [JsonPropertyName("availableActions")]
    public PackageAvailableActions AvailableActions { get; init; } = new();

    [JsonPropertyName("presentation")]
    public PackagePresentation Presentation { get; init; } = new();

    [JsonPropertyName("node")]
    public PackageNodeView? Node { get; init; }
}

internal sealed class PackageNodeView
{
    [JsonPropertyName("authorizationID")]
    public string AuthorizationId { get; init; } = string.Empty;

    [JsonPropertyName("reason")]
    public string Reason { get; init; } = string.Empty;

    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;

    [JsonPropertyName("authorized")]
    public bool Authorized { get; init; }

    [JsonPropertyName("explicitlyAuthorized")]
    public bool ExplicitlyAuthorized { get; init; }

    [JsonPropertyName("automaticallyAllowed")]
    public bool AutomaticallyAllowed { get; init; }

    [JsonPropertyName("running")]
    public bool Running { get; init; }
}

internal sealed class PackagePresentation
{
    [JsonPropertyName("statusTitle")]
    public string StatusTitle { get; init; } = string.Empty;

    [JsonPropertyName("statusDetail")]
    public string StatusDetail { get; init; } = string.Empty;

    [JsonPropertyName("statusTone")]
    public string StatusTone { get; init; } = string.Empty;

    [JsonPropertyName("isError")]
    public bool IsError { get; init; }

    [JsonPropertyName("isPending")]
    public bool IsPending { get; init; }
}

internal sealed class PackageAvailableActions
{
    [JsonPropertyName("setEnabled")]
    public bool SetEnabled { get; init; }

    [JsonPropertyName("setPriority")]
    public bool SetPriority { get; init; }

    [JsonPropertyName("openDirectory")]
    public bool OpenDirectory { get; init; }

    [JsonPropertyName("export")]
    public bool Export { get; init; }

    [JsonPropertyName("delete")]
    public bool Delete { get; init; }

    [JsonPropertyName("installMissingDependencies")]
    public bool InstallMissingDependencies { get; init; }

    [JsonPropertyName("enableDependencies")]
    public bool EnableDependencies { get; init; }

    [JsonPropertyName("updateManagedPackage")]
    public bool UpdateManagedPackage { get; init; }

    [JsonPropertyName("build")]
    public bool Build { get; init; }

    [JsonPropertyName("authorizeNode")]
    public bool AuthorizeNode { get; init; }
}

internal sealed class RemoteUpdate
{
    [JsonPropertyName("status")]
    public string Status { get; init; } = string.Empty;
}

internal sealed class GitHubRelease
{
    [JsonPropertyName("tag_name")]
    public string TagName { get; init; } = string.Empty;

    [JsonPropertyName("html_url")]
    public string? HtmlUrl { get; init; }
}

internal sealed class BackendUpdateSnapshot
{
    [JsonPropertyName("channel")]
    public string Channel { get; init; } = "stable";

    [JsonPropertyName("packageChannel")]
    public string PackageChannel { get; init; } = "stable";

    [JsonPropertyName("autoCheck")]
    public bool AutoCheck { get; init; }

    [JsonPropertyName("checking")]
    public bool Checking { get; init; }

    [JsonPropertyName("latestRelease")]
    public GitHubRelease? LatestRelease { get; init; }

    [JsonPropertyName("lastError")]
    public string? LastError { get; init; }

    [JsonPropertyName("lastCheckAt")]
    public DateTimeOffset? LastCheckAt { get; init; }

    [JsonPropertyName("currentVersion")]
    public string CurrentVersion { get; init; } = "-";

    [JsonPropertyName("buildNumber")]
    public string BuildNumber { get; init; } = "-";

    [JsonPropertyName("hasNewerVersion")]
    public bool HasNewerVersion { get; init; }

    [JsonPropertyName("updateAvailable")]
    public bool UpdateAvailable { get; init; }

    [JsonPropertyName("latestVersionString")]
    public string LatestVersionString { get; init; } = "-";

    [JsonPropertyName("latestVersionIsSkipped")]
    public bool LatestVersionIsSkipped { get; init; }
}

internal sealed class BackendAppSnapshot
{
    [JsonPropertyName("protocolVersion")]
    public int ProtocolVersion { get; init; }

    [JsonPropertyName("presentation")]
    public PresentationContract Presentation { get; init; } = new();

    [JsonPropertyName("status")]
    public BackendAppStatus Status { get; init; } = new();

    [JsonPropertyName("enabled")]
    public bool Enabled { get; init; }

    [JsonPropertyName("disableGPUAcceleration")]
    public bool DisableGPUAcceleration { get; init; }

    [JsonPropertyName("developerMode")]
    public bool DeveloperMode { get; init; }

    [JsonPropertyName("developerAllowUnknownNode")]
    public bool DeveloperAllowUnknownNode { get; init; }

    [JsonPropertyName("packages")]
    public List<PackageView> Packages { get; init; } = [];

    [JsonPropertyName("disabledPackageIDs")]
    public List<string> DisabledPackageIds { get; init; } = [];

    [JsonPropertyName("buildingPackageIDs")]
    public List<string> BuildingPackageIds { get; init; } = [];

    [JsonPropertyName("exportingPackageIDs")]
    public List<string> ExportingPackageIds { get; init; } = [];

    [JsonPropertyName("packageBuildErrors")]
    public Dictionary<string, string> PackageBuildErrors { get; init; } = [];

    [JsonPropertyName("packageRuntimeErrors")]
    public Dictionary<string, string> PackageRuntimeErrors { get; init; } = [];

    [JsonPropertyName("packagePayloadErrors")]
    public Dictionary<string, string> PackagePayloadErrors { get; init; } = [];

    [JsonPropertyName("packageDependencyIssues")]
    public Dictionary<string, List<string>> PackageDependencyIssues { get; init; } = [];

    [JsonPropertyName("nodeEnvironment")]
    public NodeEnvironment? NodeEnvironment { get; init; }

    [JsonPropertyName("checkingNode")]
    public bool CheckingNode { get; init; }

    [JsonPropertyName("gitEnvironment")]
    public GitEnvironment? GitEnvironment { get; init; }

    [JsonPropertyName("checkingGit")]
    public bool CheckingGit { get; init; }

    [JsonPropertyName("checkingRemoteUpdates")]
    public bool CheckingRemoteUpdates { get; init; }

    [JsonPropertyName("remotePackageUpdates")]
    public Dictionary<string, RemoteUpdate> RemotePackageUpdates { get; init; } = [];

    [JsonPropertyName("remotePackageErrors")]
    public Dictionary<string, string> RemotePackageErrors { get; init; } = [];

    [JsonPropertyName("installingPackageIDs")]
    public List<string> InstallingPackageIds { get; init; } = [];

    [JsonPropertyName("installingRemotePackage")]
    public bool InstallingRemotePackage { get; init; }

    [JsonPropertyName("remoteOperationMessage")]
    public string? RemoteOperationMessage { get; init; }

    [JsonPropertyName("remoteOperationError")]
    public string? RemoteOperationError { get; init; }

    [JsonPropertyName("installingLocalPackage")]
    public bool InstallingLocalPackage { get; init; }

    [JsonPropertyName("localOperationMessage")]
    public string? LocalOperationMessage { get; init; }

    [JsonPropertyName("localOperationError")]
    public string? LocalOperationError { get; init; }

    [JsonPropertyName("packagesDirectory")]
    public string PackagesDirectory { get; init; } = string.Empty;

    [JsonPropertyName("logPath")]
    public string LogPath { get; init; } = string.Empty;

    [JsonPropertyName("logText")]
    public string LogText { get; init; } = string.Empty;

    [JsonPropertyName("enabledPackageCount")]
    public int EnabledPackageCount { get; init; }

    [JsonPropertyName("activePackageCount")]
    public int ActivePackageCount { get; init; }

    [JsonPropertyName("update")]
    public BackendUpdateSnapshot Update { get; init; } = new();
}
