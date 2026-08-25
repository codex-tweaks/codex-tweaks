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

    [JsonPropertyName("installMissingDependencies")]
    public bool InstallMissingDependencies { get; init; }

    [JsonPropertyName("enableDependencies")]
    public bool EnableDependencies { get; init; }

    [JsonPropertyName("updateManagedPackage")]
    public bool UpdateManagedPackage { get; init; }

    [JsonPropertyName("build")]
    public bool Build { get; init; }
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

internal sealed class CapabilityUsageDescriptor
{
    [JsonPropertyName("useWhen")]
    public List<string> UseWhen { get; init; } = [];

    [JsonPropertyName("constraints")]
    public List<string> Constraints { get; init; } = [];

    [JsonPropertyName("manifestExample")]
    public string ManifestExample { get; init; } = string.Empty;

    [JsonPropertyName("runtimeExample")]
    public string RuntimeExample { get; init; } = string.Empty;
}

internal sealed class CapabilityManifestDescriptor
{
    [JsonPropertyName("requirementJSONPointer")]
    public string RequirementJsonPointer { get; init; } = string.Empty;

    [JsonPropertyName("fields")]
    public List<CapabilityFieldDescriptor> Fields { get; init; } = [];
}

internal sealed class CapabilityRuntimeDescriptor
{
    [JsonPropertyName("scope")]
    public string Scope { get; init; } = string.Empty;

    [JsonPropertyName("requiredAccess")]
    public string RequiredAccess { get; init; } = string.Empty;

    [JsonPropertyName("optionalAccess")]
    public string OptionalAccess { get; init; } = string.Empty;

    [JsonPropertyName("properties")]
    public List<CapabilityFieldDescriptor> Properties { get; init; } = [];

    [JsonPropertyName("methods")]
    public List<CapabilityMethodDescriptor> Methods { get; init; } = [];
}

internal sealed class CapabilityMethodDescriptor
{
    [JsonPropertyName("name")]
    public string Name { get; init; } = string.Empty;

    [JsonPropertyName("async")]
    public bool Async { get; init; }

    [JsonPropertyName("signature")]
    public string Signature { get; init; } = string.Empty;

    [JsonPropertyName("description")]
    public string Description { get; init; } = string.Empty;

    [JsonPropertyName("inputs")]
    public List<CapabilityFieldDescriptor> Inputs { get; init; } = [];

    [JsonPropertyName("outputs")]
    public List<CapabilityFieldDescriptor> Outputs { get; init; } = [];

    [JsonPropertyName("errors")]
    public List<CapabilityErrorDescriptor> Errors { get; init; } = [];
}

internal sealed class CapabilityFieldDescriptor
{
    [JsonPropertyName("path")]
    public string Path { get; init; } = string.Empty;

    [JsonPropertyName("type")]
    public string Type { get; init; } = string.Empty;

    [JsonPropertyName("itemType")]
    public string? ItemType { get; init; }

    [JsonPropertyName("format")]
    public string? Format { get; init; }

    [JsonPropertyName("required")]
    public bool Required { get; init; }

    [JsonPropertyName("description")]
    public string Description { get; init; } = string.Empty;

    [JsonPropertyName("values")]
    public List<string>? Values { get; init; }

    [JsonPropertyName("defaultJSON")]
    public string? DefaultJson { get; init; }

    [JsonPropertyName("minimum")]
    public int? Minimum { get; init; }

    [JsonPropertyName("maximum")]
    public int? Maximum { get; init; }

    [JsonPropertyName("minimumItems")]
    public int? MinimumItems { get; init; }

    [JsonPropertyName("maximumItems")]
    public int? MaximumItems { get; init; }

    [JsonPropertyName("minimumLength")]
    public int? MinimumLength { get; init; }

    [JsonPropertyName("maximumLength")]
    public int? MaximumLength { get; init; }
}

internal sealed class CapabilityErrorDescriptor
{
    [JsonPropertyName("code")]
    public string Code { get; init; } = string.Empty;

    [JsonPropertyName("description")]
    public string Description { get; init; } = string.Empty;
}

internal sealed class CapabilityDescriptor
{
    [JsonPropertyName("descriptorVersion")]
    public int DescriptorVersion { get; init; }

    [JsonPropertyName("id")]
    public string Id { get; init; } = string.Empty;

    [JsonPropertyName("version")]
    public string Version { get; init; } = string.Empty;

    [JsonPropertyName("summary")]
    public string Summary { get; init; } = string.Empty;

    [JsonPropertyName("usage")]
    public CapabilityUsageDescriptor Usage { get; init; } = new();

    [JsonPropertyName("manifest")]
    public CapabilityManifestDescriptor Manifest { get; init; } = new();

    [JsonPropertyName("runtime")]
    public CapabilityRuntimeDescriptor Runtime { get; init; } = new();
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

    [JsonPropertyName("developerMode")]
    public bool DeveloperMode { get; init; }

    [JsonPropertyName("availableCapabilities")]
    public List<CapabilityDescriptor> AvailableCapabilities { get; init; } = [];

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
