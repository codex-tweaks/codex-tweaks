import Foundation

enum TweakPackageRemoteSelectorType: String, Codable, CaseIterable, Identifiable, Sendable {
    case branch
    case latestSemverTag
    case tag
    case githubLatestRelease
    case githubRelease
    case commit

    var id: Self { self }

    var title: String {
        switch self {
        case .branch: return "指定分支"
        case .latestSemverTag: return "最新 SemVer Tag"
        case .tag: return "指定 Tag"
        case .githubLatestRelease: return "GitHub 最新 Release"
        case .githubRelease: return "指定 GitHub Release"
        case .commit: return "指定 Commit"
        }
    }

    var valueLabel: String? {
        switch self {
        case .branch: return "分支名称"
        case .tag: return "Tag 名称"
        case .githubRelease: return "Release 的 Tag"
        case .commit: return "Commit SHA"
        case .latestSemverTag, .githubLatestRelease: return nil
        }
    }

    var defaultValue: String {
        switch self {
        case .branch: return "main"
        default: return ""
        }
    }

    var isPinned: Bool {
        switch self {
        case .tag, .githubRelease, .commit: return true
        case .branch, .latestSemverTag, .githubLatestRelease: return false
        }
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

    init(version: String, source: TweakPackageSource? = nil) {
        self.version = version
        self.source = source
    }
}

struct TweakPackageManifest: Codable, Equatable {
    struct Configuration: Codable, Equatable {
        let apiVersion: Int
        let entry: String
        let priority: Int
        let packageDependencies: [String: TweakPackageDependency]

        init(
            apiVersion: Int = 2,
            entry: String,
            priority: Int = 0,
            packageDependencies: [String: TweakPackageDependency] = [:]
        ) {
            self.apiVersion = apiVersion
            self.entry = entry
            self.priority = priority
            self.packageDependencies = packageDependencies
        }

        private enum CodingKeys: String, CodingKey {
            case apiVersion
            case entry
            case priority
            case packageDependencies
        }

        init(from decoder: Decoder) throws {
            let container = try decoder.container(keyedBy: CodingKeys.self)
            apiVersion = try container.decodeIfPresent(Int.self, forKey: .apiVersion) ?? 2
            entry = try container.decode(String.self, forKey: .entry)
            priority = try container.decodeIfPresent(Int.self, forKey: .priority) ?? 0
            packageDependencies = try container.decodeIfPresent(
                [String: TweakPackageDependency].self,
                forKey: .packageDependencies
            ) ?? [:]
        }
    }

    let name: String
    let version: String
    let description: String
    let type: String?
    let dependencies: [String: String]
    let codexTweaks: Configuration

    init(
        name: String,
        version: String,
        description: String,
        type: String? = nil,
        dependencies: [String: String] = [:],
        codexTweaks: Configuration
    ) {
        self.name = name
        self.version = version
        self.description = description
        self.type = type
        self.dependencies = dependencies
        self.codexTweaks = codexTweaks
    }

    private enum CodingKeys: String, CodingKey {
        case name
        case version
        case description
        case type
        case dependencies
        case codexTweaks
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        name = try container.decode(String.self, forKey: .name)
        version = try container.decode(String.self, forKey: .version)
        description = try container.decodeIfPresent(String.self, forKey: .description) ?? ""
        type = try container.decodeIfPresent(String.self, forKey: .type)
        dependencies = try container.decodeIfPresent(
            [String: String].self,
            forKey: .dependencies
        ) ?? [:]
        codexTweaks = try container.decode(Configuration.self, forKey: .codexTweaks)
    }
}

struct TweakPackageBuildRecord: Codable, Equatable {
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

    init(
        packageID: String,
        packageVersion: String,
        packageDependencies: [String: String] = [:],
        sourceFingerprint: String,
        dependencyFingerprint: String,
        compilerVersion: String,
        nodeVersion: String,
        buildDirectoryName: String,
        hasCSS: Bool,
        builtAt: Date
    ) {
        self.packageID = packageID
        self.packageVersion = packageVersion
        self.packageDependencies = packageDependencies
        self.sourceFingerprint = sourceFingerprint
        self.dependencyFingerprint = dependencyFingerprint
        self.compilerVersion = compilerVersion
        self.nodeVersion = nodeVersion
        self.buildDirectoryName = buildDirectoryName
        self.hasCSS = hasCSS
        self.builtAt = builtAt
    }

    private enum CodingKeys: String, CodingKey {
        case packageID
        case packageVersion
        case packageDependencies
        case sourceFingerprint
        case dependencyFingerprint
        case compilerVersion
        case nodeVersion
        case buildDirectoryName
        case hasCSS
        case builtAt
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        packageID = try container.decode(String.self, forKey: .packageID)
        packageVersion = try container.decode(String.self, forKey: .packageVersion)
        packageDependencies = try container.decodeIfPresent(
            [String: String].self,
            forKey: .packageDependencies
        ) ?? [:]
        sourceFingerprint = try container.decode(String.self, forKey: .sourceFingerprint)
        dependencyFingerprint = try container.decode(String.self, forKey: .dependencyFingerprint)
        compilerVersion = try container.decode(String.self, forKey: .compilerVersion)
        nodeVersion = try container.decode(String.self, forKey: .nodeVersion)
        buildDirectoryName = try container.decode(String.self, forKey: .buildDirectoryName)
        hasCSS = try container.decode(Bool.self, forKey: .hasCSS)
        builtAt = try container.decode(Date.self, forKey: .builtAt)
    }
}

struct ActiveTweakPackageBuild: Equatable {
    let record: TweakPackageBuildRecord
    let outputDirectoryURL: URL

    var javaScriptURL: URL {
        outputDirectoryURL.appendingPathComponent("bundle.js")
    }

    var cssURL: URL {
        outputDirectoryURL.appendingPathComponent("bundle.css")
    }
}

struct TweakManagedPackageRegistration: Codable, Equatable, Sendable {
    let packageID: String
    let source: TweakPackageSource
    let addedAt: Date
    var versionRequirements: [String]
    var lastCheckedAt: Date?
    var remoteETag: String?
    var lastResolvedReference: String?
    var lastResolvedCommit: String?

    init(
        packageID: String,
        source: TweakPackageSource,
        addedAt: Date,
        versionRequirements: [String] = [],
        lastCheckedAt: Date? = nil,
        remoteETag: String? = nil,
        lastResolvedReference: String? = nil,
        lastResolvedCommit: String? = nil
    ) {
        self.packageID = packageID
        self.source = source
        self.addedAt = addedAt
        self.versionRequirements = versionRequirements
        self.lastCheckedAt = lastCheckedAt
        self.remoteETag = remoteETag
        self.lastResolvedReference = lastResolvedReference
        self.lastResolvedCommit = lastResolvedCommit
    }

    private enum CodingKeys: String, CodingKey {
        case packageID
        case source
        case addedAt
        case versionRequirements
        case lastCheckedAt
        case remoteETag
        case lastResolvedReference
        case lastResolvedCommit
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        packageID = try container.decode(String.self, forKey: .packageID)
        source = try container.decode(TweakPackageSource.self, forKey: .source)
        addedAt = try container.decode(Date.self, forKey: .addedAt)
        versionRequirements = try container.decodeIfPresent(
            [String].self,
            forKey: .versionRequirements
        ) ?? []
        lastCheckedAt = try container.decodeIfPresent(Date.self, forKey: .lastCheckedAt)
        remoteETag = try container.decodeIfPresent(String.self, forKey: .remoteETag)
        lastResolvedReference = try container.decodeIfPresent(
            String.self,
            forKey: .lastResolvedReference
        )
        lastResolvedCommit = try container.decodeIfPresent(
            String.self,
            forKey: .lastResolvedCommit
        )
    }
}

struct TweakManagedPackageRegistry: Codable, Equatable, Sendable {
    var schemaVersion = 1
    var packages: [String: TweakManagedPackageRegistration] = [:]
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

struct TweakManagedPackageLockfile: Codable, Equatable, Sendable {
    var schemaVersion = 1
    var packages: [String: TweakManagedPackageLock] = [:]
}

enum TweakPackageOrigin: Equatable {
    case local
    case managed(TweakManagedPackageLock)
}

struct TweakPackageUserSetting: Codable, Equatable {
    var priorityOverride: Int?
}

struct TweakPackageUserSettings: Codable, Equatable {
    var schemaVersion = 1
    var packages: [String: TweakPackageUserSetting] = [:]
}

struct TweakPackage: Identifiable, Equatable {
    enum BuildDisposition: Equatable {
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
    let directoryURL: URL
    let manifest: TweakPackageManifest?
    let sourceFingerprint: String?
    let dependencyFingerprint: String?
    let activeBuild: ActiveTweakPackageBuild?
    let validationError: String?
    let priorityOverride: Int?
    let origin: TweakPackageOrigin

    var displayName: String {
        manifest?.name ?? directoryName
    }

    var version: String {
        manifest?.version ?? "—"
    }

    var detail: String {
        manifest?.description.isEmpty == false
            ? manifest?.description ?? ""
            : "没有提供包说明。"
    }

    var declaredPriority: Int {
        manifest?.codexTweaks.priority ?? 0
    }

    var priority: Int { priorityOverride ?? declaredPriority }

    var hasDependencies: Bool {
        !(manifest?.dependencies.isEmpty ?? true)
    }

    var packageDependencies: [String: TweakPackageDependency] {
        manifest?.codexTweaks.packageDependencies ?? [:]
    }

    var runtimePackageDependencies: [String: String] {
        if let activeBuild {
            return activeBuild.record.packageDependencies
        }
        return packageDependencies.mapValues(\.version)
    }

    var isManaged: Bool {
        if case .managed = origin { return true }
        return false
    }

    var managedLock: TweakManagedPackageLock? {
        if case let .managed(lock) = origin { return lock }
        return nil
    }

    func withPriorityOverride(_ value: Int?) -> TweakPackage {
        TweakPackage(
            id: id,
            directoryName: directoryName,
            directoryURL: directoryURL,
            manifest: manifest,
            sourceFingerprint: sourceFingerprint,
            dependencyFingerprint: dependencyFingerprint,
            activeBuild: activeBuild,
            validationError: validationError,
            priorityOverride: value,
            origin: origin
        )
    }

    func buildDisposition(compilerVersion: String) -> BuildDisposition {
        guard validationError == nil,
              let manifest,
              let sourceFingerprint,
              let dependencyFingerprint else {
            return .invalid
        }
        guard let activeBuild else { return .notBuilt }
        if activeBuild.record.packageVersion != manifest.version {
            return .versionUpdate
        }
        if activeBuild.record.dependencyFingerprint != dependencyFingerprint {
            return .dependencyUpdate
        }
        if activeBuild.record.sourceFingerprint != sourceFingerprint {
            return .sourceChanged
        }
        if activeBuild.record.compilerVersion != compilerVersion {
            return .compilerUpdate
        }
        return .current
    }

    func buildRequestKey(compilerVersion: String) -> String? {
        guard validationError == nil,
              let manifest,
              let sourceFingerprint,
              let dependencyFingerprint else { return nil }
        return manifest.version
            + "\u{0}" + sourceFingerprint
            + "\u{0}" + dependencyFingerprint
            + "\u{0}" + compilerVersion
    }
}

struct TweakPackageEnablementReconciliation: Equatable {
    let knownPackageIDs: Set<String>
    let disabledPackageIDs: Set<String>
    let newlyDiscoveredPackageIDs: Set<String>

    static func reconcile(
        discoveredPackageIDs: Set<String>,
        knownPackageIDs: Set<String>?,
        disabledPackageIDs: Set<String>
    ) -> Self {
        guard let knownPackageIDs else {
            return Self(
                knownPackageIDs: discoveredPackageIDs,
                disabledPackageIDs: disabledPackageIDs,
                newlyDiscoveredPackageIDs: []
            )
        }

        let newlyDiscoveredPackageIDs = discoveredPackageIDs.subtracting(knownPackageIDs)
        return Self(
            knownPackageIDs: knownPackageIDs.union(discoveredPackageIDs),
            disabledPackageIDs: disabledPackageIDs.union(newlyDiscoveredPackageIDs),
            newlyDiscoveredPackageIDs: newlyDiscoveredPackageIDs
        )
    }
}

struct CompiledTweakPackage: Equatable, Identifiable {
    let id: String
    let name: String
    let version: String
    let buildFingerprint: String
    let dependencyIDs: [String]
    let css: String
    let javascript: String
}

struct TweakPayload: Equatable {
    let packages: [CompiledTweakPackage]
    let version: String
}

struct TweakPayloadLoadResult: Equatable {
    let payload: TweakPayload
    let packageErrors: [String: String]
}
