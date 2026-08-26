import Foundation

enum TweakPackageRemoteSelectorType: String, Codable, CaseIterable, Identifiable, Sendable {
    case defaultBranch
    case branch
    case latestSemverTag
    case tag
    case githubLatestRelease
    case githubRelease
    case commit

    var id: Self { self }

    var titleKey: PresentationTextKey {
        switch self {
        case .defaultBranch: return .selectorDefaultBranch
        case .branch: return .selectorBranch
        case .latestSemverTag: return .selectorLatestSemverTag
        case .tag: return .selectorTag
        case .githubLatestRelease: return .selectorGithubLatestRelease
        case .githubRelease: return .selectorGithubRelease
        case .commit: return .selectorCommit
        }
    }

    var valueLabelKey: PresentationTextKey? {
        switch self {
        case .branch: return .selectorBranchValue
        case .tag: return .selectorTagValue
        case .githubRelease: return .selectorGithubReleaseValue
        case .commit: return .selectorCommitValue
        case .defaultBranch, .latestSemverTag, .githubLatestRelease: return nil
        }
    }

    func title(contract: BackendPresentationContract?) -> String {
        PresentationText.resolve(titleKey, contract: contract)
    }

    func valueLabel(contract: BackendPresentationContract?) -> String? {
        valueLabelKey.map { PresentationText.resolve($0, contract: contract) }
    }

}

struct TweakPackageRemoteSelector: Codable, Equatable, Sendable {
    let type: TweakPackageRemoteSelectorType
    let value: String?

    init(type: TweakPackageRemoteSelectorType, value: String? = nil) {
        self.type = type
        let normalized = value?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.value = normalized?.isEmpty == false ? normalized : nil
    }
}

struct TweakPackageSource: Codable, Equatable, Sendable {
    let url: String
    let selector: TweakPackageRemoteSelector
}

struct TweakPackageDependency: Codable, Equatable, Sendable {
    let version: String
    let source: TweakPackageSource?
}

struct TweakPackageManifest: Codable, Equatable, Sendable {
    struct Entrypoints: Codable, Equatable, Sendable {
        let renderer: String
        let node: String?
    }

    struct Configuration: Codable, Equatable, Sendable {
        let apiVersion: Int
        let entrypoints: Entrypoints
        let priority: Int
        let packageDependencies: [String: TweakPackageDependency]
    }

    let name: String
    let version: String
    let description: String
    let type: String?
    let dependencies: [String: String]
    let codexTweaks: Configuration
}

struct TweakPackageBuildRecord: Codable, Equatable, Sendable {
    let packageID: String
    let packageVersion: String
    let packageDependencies: [String: String]
    let sourceFingerprint: String
    let dependencyFingerprint: String
    let compilerVersion: String
    let nodeVersion: String
    let buildDirectoryName: String
    let hasCSS: Bool
    let builtAt: Date
}

struct ActiveTweakPackageBuild: Codable, Equatable, Sendable {
    let record: TweakPackageBuildRecord
    let outputDirectory: String

    var outputDirectoryURL: URL { URL(fileURLWithPath: outputDirectory, isDirectory: true) }
    var javaScriptURL: URL { outputDirectoryURL.appendingPathComponent("bundle.js") }
    var cssURL: URL { outputDirectoryURL.appendingPathComponent("bundle.css") }
}

struct TweakManagedPackageLock: Codable, Equatable, Sendable {
    let packageID: String
    let packageVersion: String
    let source: TweakPackageSource
    let resolvedReference: String
    let resolvedCommit: String
    let sourceRelativePath: String
    let installedAt: Date
}

enum TweakPackageOrigin: Codable, Equatable, Sendable {
    case local
    case managed(TweakManagedPackageLock)

    private enum CodingKeys: String, CodingKey { case kind, lock }
    private enum Kind: String, Codable { case local, managed }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        switch try container.decode(Kind.self, forKey: .kind) {
        case .local:
            self = .local
        case .managed:
            self = .managed(try container.decode(TweakManagedPackageLock.self, forKey: .lock))
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        switch self {
        case .local:
            try container.encode(Kind.local, forKey: .kind)
        case let .managed(lock):
            try container.encode(Kind.managed, forKey: .kind)
            try container.encode(lock, forKey: .lock)
        }
    }
}

struct TweakPackage: Codable, Identifiable, Equatable, Sendable {
    enum BuildDisposition: String, Codable, Equatable, Sendable {
        case invalid
        case notBuilt
        case current
        case versionUpdate
        case dependencyUpdate
        case sourceChanged
        case compilerUpdate
    }

    let id: String
    let directoryName: String
    let directory: String
    let exportFileName: String
    let manifest: TweakPackageManifest?
    let sourceFingerprint: String?
    let dependencyFingerprint: String?
    let activeBuild: ActiveTweakPackageBuild?
    let validationError: String?
    let priorityOverride: Int?
    let origin: TweakPackageOrigin
    let displayName: String
    let displayVersion: String
    let detail: String
    let declaredPriority: Int
    let priority: Int
    let hasDependencies: Bool
    let packageDependencies: [String: TweakPackageDependency]
    let runtimePackageDependencies: [String: String]
    let isManaged: Bool
    let managedLock: TweakManagedPackageLock?
    let projectPageURL: URL?
    private let backendBuildDisposition: BuildDisposition
    let buildRequestKey: String?
    let canInstallMissingDependencies: Bool
    let canEnableDependencies: Bool
    let availableActions: TweakPackageAvailableActions
    let presentation: TweakPackagePresentation
    let node: TweakPackageNode?

    var directoryURL: URL { URL(fileURLWithPath: directory, isDirectory: true) }
    var version: String { displayVersion }

    var buildDisposition: BuildDisposition { backendBuildDisposition }

    private enum CodingKeys: String, CodingKey {
        case id, directoryName, directory, exportFileName, manifest, sourceFingerprint, dependencyFingerprint
        case activeBuild, validationError, priorityOverride, origin, displayName, displayVersion
        case detail, declaredPriority, priority, hasDependencies, packageDependencies
        case runtimePackageDependencies, isManaged, managedLock, projectPageURL
        case backendBuildDisposition = "buildDisposition"
        case buildRequestKey, canInstallMissingDependencies, canEnableDependencies
        case availableActions, presentation, node
    }
}

struct TweakPackageNode: Codable, Equatable, Sendable {
    let authorizationID: String
    let reason: String
    let status: String
    let authorized: Bool
    let explicitlyAuthorized: Bool
    let automaticallyAllowed: Bool
    let running: Bool
}

struct TweakPackagePresentation: Codable, Equatable, Sendable {
    let statusTitle: String
    let statusDetail: String
    let statusTone: String
    let isError: Bool
    let isPending: Bool
}

struct TweakPackageAvailableActions: Codable, Equatable, Sendable {
    let setEnabled: Bool
    let setPriority: Bool
    let openDirectory: Bool
    let export: Bool
    let delete: Bool
    let installMissingDependencies: Bool
    let enableDependencies: Bool
    let updateManagedPackage: Bool
    let build: Bool
    let authorizeNode: Bool
}
