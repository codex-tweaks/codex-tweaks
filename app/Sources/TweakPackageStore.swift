import Foundation

final class TweakPackageStore: @unchecked Sendable {
    static let compilerVersion = "0.25.9"

    let tweaksDirectoryURL: URL
    let packagesDirectoryURL: URL
    let stateDirectoryURL: URL
    let packageSettingsURL: URL
    let managedPackagesDirectoryURL: URL
    let managedSourcesDirectoryURL: URL
    let managedRegistryURL: URL
    let managedLockfileURL: URL
    let buildCacheDirectoryURL: URL

    init(
        fileManager: FileManager = .default,
        applicationSupportURL: URL? = nil,
        cachesURL: URL? = nil
    ) {
        let applicationSupport = applicationSupportURL ?? fileManager.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first!
        let caches = cachesURL ?? fileManager.urls(
            for: .cachesDirectory,
            in: .userDomainMask
        ).first!

        let applicationRoot = applicationSupport
            .appendingPathComponent("Codex Tweaks", isDirectory: true)
        tweaksDirectoryURL = applicationRoot
            .appendingPathComponent("Tweaks", isDirectory: true)
        packagesDirectoryURL = tweaksDirectoryURL
            .appendingPathComponent("packages", isDirectory: true)
        stateDirectoryURL = applicationRoot
            .appendingPathComponent("State", isDirectory: true)
        packageSettingsURL = stateDirectoryURL.appendingPathComponent("package-settings.json")
        managedPackagesDirectoryURL = applicationRoot
            .appendingPathComponent("ManagedPackages", isDirectory: true)
        managedSourcesDirectoryURL = managedPackagesDirectoryURL
            .appendingPathComponent("sources", isDirectory: true)
        managedRegistryURL = managedPackagesDirectoryURL.appendingPathComponent("registry.json")
        managedLockfileURL = managedPackagesDirectoryURL.appendingPathComponent("packages.lock.json")
        buildCacheDirectoryURL = caches
            .appendingPathComponent("Codex Tweaks", isDirectory: true)
            .appendingPathComponent("PackageBuilds", isDirectory: true)
    }

    func prepareUserPackages(
        fileManager: FileManager = .default,
        bundle: Bundle = .main
    ) throws {
        try fileManager.createDirectory(
            at: packagesDirectoryURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try fileManager.createDirectory(
            at: buildCacheDirectoryURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        for directoryURL in [stateDirectoryURL, managedPackagesDirectoryURL, managedSourcesDirectoryURL] {
            try fileManager.createDirectory(
                at: directoryURL,
                withIntermediateDirectories: true,
                attributes: [.posixPermissions: 0o700]
            )
        }

        guard let bundledPackagesURL = bundle.url(
            forResource: "packages",
            withExtension: nil,
            subdirectory: "Tweaks"
        ) else { return }

        let bundledPackages = try directSubdirectories(
            in: bundledPackagesURL,
            fileManager: fileManager
        )
        for bundledPackageURL in bundledPackages {
            let destinationURL = packagesDirectoryURL.appendingPathComponent(
                bundledPackageURL.lastPathComponent,
                isDirectory: true
            )
            guard !fileManager.fileExists(atPath: destinationURL.path) else { continue }
            try fileManager.copyItem(at: bundledPackageURL, to: destinationURL)
        }
    }

    func loadPackages(
        fileManager: FileManager = .default,
        bundle: Bundle = .main
    ) throws -> [TweakPackage] {
        try prepareUserPackages(fileManager: fileManager, bundle: bundle)
        var settings = try loadUserSettings(fileManager: fileManager)
        var settingsChanged = false

        func normalizedPriorityOverride(for package: TweakPackage) -> Int? {
            guard package.manifest != nil,
                  let priorityOverride = settings.packages[package.id]?.priorityOverride else {
                return settings.packages[package.id]?.priorityOverride
            }
            guard priorityOverride == package.declaredPriority else {
                return priorityOverride
            }
            settings.packages.removeValue(forKey: package.id)
            settingsChanged = true
            return nil
        }

        let localPackageDirectories = try directSubdirectories(
            in: packagesDirectoryURL,
            fileManager: fileManager
        )

        var packages = localPackageDirectories.map { directoryURL in
            let package = inspectPackage(
                at: directoryURL,
                origin: .local,
                fileManager: fileManager
            )
            return package.withPriorityOverride(normalizedPriorityOverride(for: package))
        }
        let lockfile = try loadManagedLockfile(fileManager: fileManager)
        for lock in lockfile.packages.values.sorted(by: { $0.packageID < $1.packageID }) {
            let sourceURL = managedPackagesDirectoryURL
                .appendingPathComponent(lock.sourceRelativePath, isDirectory: true)
                .standardizedFileURL
            let managedPath = managedPackagesDirectoryURL.standardizedFileURL.path + "/"
            guard sourceURL.path.hasPrefix(managedPath),
                  fileManager.fileExists(atPath: sourceURL.path) else {
                packages.append(
                    TweakPackage(
                        id: lock.packageID,
                        directoryName: lock.packageID,
                        directoryURL: sourceURL,
                        manifest: nil,
                        sourceFingerprint: nil,
                        dependencyFingerprint: nil,
                        activeBuild: nil,
                        validationError: "远程功能包的锁定源码不存在。",
                        priorityOverride: settings.packages[lock.packageID]?.priorityOverride,
                        origin: .managed(lock)
                    )
                )
                continue
            }
            let package = inspectPackage(
                at: sourceURL,
                origin: .managed(lock),
                expectedPackageID: lock.packageID,
                fileManager: fileManager
            )
            packages.append(
                package.withPriorityOverride(normalizedPriorityOverride(for: package))
            )
        }
        if settingsChanged {
            try writeUserSettings(settings, fileManager: fileManager)
        }
        let duplicateIDs = Dictionary(grouping: packages.compactMap { package in
            package.manifest.map { ($0.name, package) }
        }, by: \.0)
            .filter { $0.value.count > 1 }
            .keys

        if !duplicateIDs.isEmpty {
            packages = packages.map { package in
                guard duplicateIDs.contains(package.manifest?.name ?? "") else {
                    return package
                }
                return TweakPackage(
                    id: package.id,
                    directoryName: package.directoryName,
                    directoryURL: package.directoryURL,
                    manifest: package.manifest,
                    sourceFingerprint: package.sourceFingerprint,
                    dependencyFingerprint: package.dependencyFingerprint,
                    activeBuild: package.activeBuild,
                    validationError: "包标识重复：\(package.manifest?.name ?? package.id)",
                    priorityOverride: package.priorityOverride,
                    origin: package.origin
                )
            }
        }

        return packages.sorted {
            if $0.priority != $1.priority { return $0.priority < $1.priority }
            return $0.id.localizedStandardCompare($1.id) == .orderedAscending
        }
    }

    func loadPayload(
        from packages: [TweakPackage],
        disabledPackageIDs: Set<String>,
        fileManager: FileManager = .default
    ) -> TweakPayloadLoadResult {
        var compiledPackages: [CompiledTweakPackage] = []
        var errors: [String: String] = [:]

        let resolution = TweakPackageDependencyResolver.resolve(
            packages: packages,
            disabledPackageIDs: disabledPackageIDs
        )
        for (packageID, issues) in resolution.issuesByPackageID
            where !disabledPackageIDs.contains(packageID) && !issues.isEmpty {
            errors[packageID] = issues.joined(separator: " ")
        }

        for package in resolution.loadablePackages {
            guard package.validationError == nil,
                  let manifest = package.manifest,
                  let activeBuild = package.activeBuild else {
                continue
            }

            do {
                let javascript = try String(
                    contentsOf: activeBuild.javaScriptURL,
                    encoding: .utf8
                )
                let css = activeBuild.record.hasCSS
                    ? try String(contentsOf: activeBuild.cssURL, encoding: .utf8)
                    : ""
                compiledPackages.append(
                    CompiledTweakPackage(
                        id: package.id,
                        name: manifest.name,
                        version: activeBuild.record.packageVersion,
                        buildFingerprint: activeBuild.record.sourceFingerprint
                            + "-"
                            + activeBuild.record.dependencyFingerprint
                            + "-"
                            + activeBuild.record.compilerVersion,
                        dependencyIDs: activeBuild.record.packageDependencies.keys.sorted(),
                        css: css,
                        javascript: javascript
                    )
                )
            } catch {
                errors[package.id] = "无法读取已编译产物：\(error.localizedDescription)"
            }
        }

        let versionMaterial = compiledPackages.map {
            $0.id
                + "\u{0}" + $0.version
                + "\u{0}" + $0.buildFingerprint
                + "\u{0}" + $0.css
                + "\u{0}" + $0.javascript
        }.joined(separator: "\u{0}")

        return TweakPayloadLoadResult(
            payload: TweakPayload(
                packages: compiledPackages,
                version: Self.fingerprint(versionMaterial)
            ),
            packageErrors: errors
        )
    }

    func buildsDirectoryURL(
        forPackageID packageID: String,
        fileManager: FileManager = .default
    ) throws -> URL {
        let packageCacheURL = packageCacheDirectoryURL(forPackageID: packageID)
        let buildsURL = packageCacheURL.appendingPathComponent("builds", isDirectory: true)
        try fileManager.createDirectory(
            at: buildsURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        return buildsURL
    }

    func activateBuild(
        _ record: TweakPackageBuildRecord,
        fileManager: FileManager = .default
    ) throws {
        let packageCacheURL = packageCacheDirectoryURL(forPackageID: record.packageID)
        try fileManager.createDirectory(
            at: packageCacheURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        let data = try encoder.encode(record)
        try data.write(
            to: packageCacheURL.appendingPathComponent("active.json"),
            options: .atomic
        )
    }

    func setPriorityOverride(
        _ priority: Int?,
        forPackageID packageID: String,
        fileManager: FileManager = .default
    ) throws {
        try prepareUserPackages(fileManager: fileManager)
        var settings = try loadUserSettings(fileManager: fileManager)
        if let priority {
            settings.packages[packageID] = TweakPackageUserSetting(priorityOverride: priority)
        } else {
            settings.packages.removeValue(forKey: packageID)
        }
        try writeUserSettings(settings, fileManager: fileManager)
    }

    func loadUserSettings(
        fileManager: FileManager = .default
    ) throws -> TweakPackageUserSettings {
        guard fileManager.fileExists(atPath: packageSettingsURL.path) else {
            return TweakPackageUserSettings()
        }
        return try JSONDecoder().decode(
            TweakPackageUserSettings.self,
            from: Data(contentsOf: packageSettingsURL)
        )
    }

    private func writeUserSettings(
        _ settings: TweakPackageUserSettings,
        fileManager _: FileManager
    ) throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        try encoder.encode(settings).write(to: packageSettingsURL, options: .atomic)
    }

    func loadManagedLockfile(
        fileManager: FileManager = .default
    ) throws -> TweakManagedPackageLockfile {
        guard fileManager.fileExists(atPath: managedLockfileURL.path) else {
            return TweakManagedPackageLockfile()
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(
            TweakManagedPackageLockfile.self,
            from: Data(contentsOf: managedLockfileURL)
        )
    }

    static func fingerprint(_ value: String) -> String {
        var state = FingerprintState()
        state.update(Data(value.utf8))
        return state.value
    }

    func inspectPackage(
        at directoryURL: URL,
        origin: TweakPackageOrigin = .local,
        priorityOverride: Int? = nil,
        expectedPackageID: String? = nil,
        fileManager: FileManager
    ) -> TweakPackage {
        let directoryName = directoryURL.lastPathComponent
        let manifestURL = directoryURL.appendingPathComponent("package.json")

        do {
            let manifestValues = try manifestURL.resourceValues(
                forKeys: [.isRegularFileKey, .isSymbolicLinkKey]
            )
            guard manifestValues.isRegularFile == true,
                  manifestValues.isSymbolicLink != true else {
                throw PackageValidationError.invalidManifestFile
            }
            let data = try Data(contentsOf: manifestURL)
            let manifest = try JSONDecoder().decode(TweakPackageManifest.self, from: data)
            try validateManifest(manifest, packageDirectoryURL: directoryURL, fileManager: fileManager)
            if let expectedPackageID, manifest.name != expectedPackageID {
                throw PackageValidationError.unexpectedName(
                    expected: expectedPackageID,
                    actual: manifest.name
                )
            }
            let sourceFingerprint = try fingerprintPackage(
                at: directoryURL,
                fileManager: fileManager
            )
            let dependencyFingerprint = try fingerprintDependencies(
                manifest: manifest,
                at: directoryURL,
                fileManager: fileManager
            )
            let activeBuild = try? loadActiveBuild(
                forPackageID: manifest.name,
                fileManager: fileManager
            )
            return TweakPackage(
                id: manifest.name,
                directoryName: directoryName,
                directoryURL: directoryURL,
                manifest: manifest,
                sourceFingerprint: sourceFingerprint,
                dependencyFingerprint: dependencyFingerprint,
                activeBuild: activeBuild,
                validationError: nil,
                priorityOverride: priorityOverride,
                origin: origin
            )
        } catch {
            return TweakPackage(
                id: "invalid:\(directoryName)",
                directoryName: directoryName,
                directoryURL: directoryURL,
                manifest: nil,
                sourceFingerprint: nil,
                dependencyFingerprint: nil,
                activeBuild: nil,
                validationError: error.localizedDescription,
                priorityOverride: priorityOverride,
                origin: origin
            )
        }
    }

    private func validateManifest(
        _ manifest: TweakPackageManifest,
        packageDirectoryURL: URL,
        fileManager: FileManager
    ) throws {
        guard !manifest.name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              !manifest.name.contains("\u{0}"),
              !manifest.name.contains("..") else {
            throw PackageValidationError.invalidName
        }
        guard SemanticVersion(manifest.version) != nil else {
            throw PackageValidationError.invalidVersion(manifest.version)
        }
        guard manifest.codexTweaks.apiVersion == 2 else {
            throw PackageValidationError.unsupportedAPIVersion(manifest.codexTweaks.apiVersion)
        }
        for (dependencyName, requirement) in manifest.dependencies {
            guard !dependencyName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                  !requirement.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                throw PackageValidationError.invalidNPMDependency(dependencyName)
            }
        }
        if !manifest.dependencies.isEmpty {
            let lockfileURL = packageDirectoryURL.appendingPathComponent("package-lock.json")
            let lockfileValues = try? lockfileURL.resourceValues(
                forKeys: [.isRegularFileKey, .isSymbolicLinkKey]
            )
            guard lockfileValues?.isRegularFile == true,
                  lockfileValues?.isSymbolicLink != true else {
                throw PackageValidationError.lockfileRequired
            }
        }
        for (dependencyID, dependency) in manifest.codexTweaks.packageDependencies {
            guard !dependencyID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                  dependencyID != manifest.name,
                  SemanticVersionRequirement(dependency.version) != nil else {
                throw PackageValidationError.invalidPackageDependency(dependencyID)
            }
            if let source = dependency.source {
                do {
                    try TweakPackageRemoteManager.validate(source: source)
                } catch {
                    throw PackageValidationError.invalidPackageDependencySource(dependencyID)
                }
            }
        }

        let packageRootURL = packageDirectoryURL
            .resolvingSymlinksInPath()
            .standardizedFileURL
        let entryURL = packageDirectoryURL
            .appendingPathComponent(manifest.codexTweaks.entry)
            .resolvingSymlinksInPath()
            .standardizedFileURL
        let packagePath = packageRootURL.path + "/"
        let entryValues = try? entryURL.resourceValues(
            forKeys: [.isRegularFileKey, .isSymbolicLinkKey]
        )
        guard entryURL.path.hasPrefix(packagePath),
              entryValues?.isRegularFile == true,
              entryValues?.isSymbolicLink != true else {
            throw PackageValidationError.invalidEntry(manifest.codexTweaks.entry)
        }
    }

    private func fingerprintPackage(
        at packageURL: URL,
        fileManager: FileManager
    ) throws -> String {
        guard let enumerator = fileManager.enumerator(
            at: packageURL,
            includingPropertiesForKeys: [.isDirectoryKey, .isRegularFileKey],
            options: [.skipsPackageDescendants]
        ) else {
            throw CocoaError(.fileReadUnknown)
        }

        var files: [(relativePath: String, url: URL)] = []
        for case let url as URL in enumerator {
            let relativePath = Self.relativePath(for: url, under: packageURL)
            let firstComponent = relativePath.split(separator: "/").first.map(String.init) ?? ""
            if firstComponent == ".git" || firstComponent == "node_modules" {
                let values = try url.resourceValues(forKeys: [.isDirectoryKey])
                if values.isDirectory == true { enumerator.skipDescendants() }
                continue
            }
            if relativePath == "package.json" || relativePath == "package-lock.json" {
                continue
            }
            if url.lastPathComponent == ".DS_Store" { continue }

            let values = try url.resourceValues(forKeys: [.isRegularFileKey])
            if values.isRegularFile == true {
                files.append((relativePath: relativePath, url: url))
            }
        }

        var state = FingerprintState()
        for file in files.sorted(by: { $0.relativePath < $1.relativePath }) {
            state.update(Data(file.relativePath.utf8))
            state.update(Data([0]))
            state.update(try Data(contentsOf: file.url))
            state.update(Data([0]))
        }
        return state.value
    }

    private func fingerprintDependencies(
        manifest: TweakPackageManifest,
        at packageURL: URL,
        fileManager: FileManager
    ) throws -> String {
        var state = FingerprintState()
        state.update(Data((manifest.type ?? "").utf8))
        state.update(Data([0]))
        state.update(Data(String(manifest.codexTweaks.apiVersion).utf8))
        state.update(Data([0]))
        state.update(Data(manifest.codexTweaks.entry.utf8))
        state.update(Data([0]))
        for dependency in manifest.dependencies.sorted(by: { $0.key < $1.key }) {
            state.update(Data(dependency.key.utf8))
            state.update(Data([0]))
            state.update(Data(dependency.value.utf8))
            state.update(Data([0]))
        }
        for dependency in manifest.codexTweaks.packageDependencies.sorted(by: { $0.key < $1.key }) {
            state.update(Data(dependency.key.utf8))
            state.update(Data([0]))
            state.update(Data(dependency.value.version.utf8))
            state.update(Data([0]))
            if let source = dependency.value.source {
                state.update(Data(source.url.utf8))
                state.update(Data([0]))
                state.update(Data(source.selector.type.rawValue.utf8))
                state.update(Data([0]))
                state.update(Data((source.selector.value ?? "").utf8))
                state.update(Data([0]))
            }
        }

        let lockfileURL = packageURL.appendingPathComponent("package-lock.json")
        if fileManager.fileExists(atPath: lockfileURL.path) {
            state.update(try Data(contentsOf: lockfileURL))
        }
        return state.value
    }

    private func loadActiveBuild(
        forPackageID packageID: String,
        fileManager: FileManager
    ) throws -> ActiveTweakPackageBuild? {
        let packageCacheURL = packageCacheDirectoryURL(forPackageID: packageID)
        let activeRecordURL = packageCacheURL.appendingPathComponent("active.json")
        guard fileManager.fileExists(atPath: activeRecordURL.path) else { return nil }

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let record = try decoder.decode(
            TweakPackageBuildRecord.self,
            from: Data(contentsOf: activeRecordURL)
        )
        guard record.packageID == packageID else {
            throw PackageValidationError.invalidBuildRecord
        }
        let outputURL = packageCacheURL
            .appendingPathComponent("builds", isDirectory: true)
            .appendingPathComponent(record.buildDirectoryName, isDirectory: true)
        let javaScriptURL = outputURL.appendingPathComponent("bundle.js")
        let cssURL = outputURL.appendingPathComponent("bundle.css")
        guard fileManager.fileExists(atPath: javaScriptURL.path),
              !record.hasCSS || fileManager.fileExists(atPath: cssURL.path) else {
            throw PackageValidationError.invalidBuildRecord
        }
        return ActiveTweakPackageBuild(record: record, outputDirectoryURL: outputURL)
    }

    private func packageCacheDirectoryURL(forPackageID packageID: String) -> URL {
        buildCacheDirectoryURL.appendingPathComponent(
            Self.fingerprint(packageID),
            isDirectory: true
        )
    }

    private func directSubdirectories(
        in directoryURL: URL,
        fileManager: FileManager
    ) throws -> [URL] {
        try fileManager.contentsOfDirectory(
            at: directoryURL,
            includingPropertiesForKeys: [.isDirectoryKey],
            options: [.skipsHiddenFiles]
        ).filter {
            try $0.resourceValues(forKeys: [.isDirectoryKey]).isDirectory == true
        }.sorted {
            $0.lastPathComponent.localizedStandardCompare($1.lastPathComponent) == .orderedAscending
        }
    }

    private static func relativePath(for url: URL, under directoryURL: URL) -> String {
        let directoryPath = directoryURL.standardizedFileURL.path + "/"
        let path = url.standardizedFileURL.path
        guard path.hasPrefix(directoryPath) else { return url.lastPathComponent }
        return String(path.dropFirst(directoryPath.count))
    }
}

private enum PackageValidationError: LocalizedError {
    case invalidManifestFile
    case invalidName
    case invalidVersion(String)
    case unsupportedAPIVersion(Int)
    case invalidNPMDependency(String)
    case lockfileRequired
    case invalidPackageDependency(String)
    case invalidPackageDependencySource(String)
    case unexpectedName(expected: String, actual: String)
    case invalidEntry(String)
    case invalidBuildRecord

    var errorDescription: String? {
        switch self {
        case .invalidManifestFile:
            return "package.json 必须是包目录中的普通文件。"
        case .invalidName:
            return "package.json 中的 name 无效。"
        case let .invalidVersion(version):
            return "包版本不是有效的 SemVer：\(version)"
        case let .unsupportedAPIVersion(version):
            return "不支持 Codex Tweaks API v\(version)。"
        case let .invalidNPMDependency(dependencyName):
            return "npm 依赖名称或版本要求无效：\(dependencyName)。"
        case .lockfileRequired:
            return "声明 npm dependencies 时必须提供 package-lock.json。"
        case let .invalidPackageDependency(dependencyID):
            return "功能包依赖无效：\(dependencyID)。"
        case let .invalidPackageDependencySource(dependencyID):
            return "功能包依赖 \(dependencyID) 的远程来源无效。"
        case let .unexpectedName(expected, actual):
            return "远程功能包标识不匹配：期望 \(expected)，实际为 \(actual)。"
        case let .invalidEntry(entry):
            return "入口文件不存在或超出包目录：\(entry)"
        case .invalidBuildRecord:
            return "当前构建记录或编译产物无效。"
        }
    }
}

private struct FingerprintState {
    private var hash: UInt64 = 14_695_981_039_346_656_037

    mutating func update(_ data: Data) {
        for byte in data {
            hash ^= UInt64(byte)
            hash &*= 1_099_511_628_211
        }
    }

    var value: String {
        String(format: "%016llx", hash)
    }
}
