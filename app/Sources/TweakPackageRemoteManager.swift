import Foundation

struct GitEnvironment: Equatable, Sendable {
    let gitURL: URL
    let version: String
}

struct TweakPackageRemoteResolution: Equatable, Sendable {
    let reference: String
    let commit: String
    let fetchReference: String
    let etag: String?
}

enum TweakPackageRemoteUpdateStatus: Equatable, Sendable {
    case current
    case available
    case pinnedReferenceChanged
}

struct TweakPackageRemoteUpdate: Equatable, Sendable {
    let packageID: String
    let currentCommit: String
    let candidateReference: String
    let candidateCommit: String
    let checkedAt: Date
    let status: TweakPackageRemoteUpdateStatus

    var isInstallable: Bool { status == .available }
}

struct TweakManagedPackageInstallResult: Equatable, Sendable {
    let packageID: String
    let manifest: TweakPackageManifest
    let lock: TweakManagedPackageLock
}

enum TweakPackageRemoteError: LocalizedError {
    case gitUnavailable
    case invalidRepositoryURL
    case missingSelectorValue(String)
    case remoteReferenceNotFound(String)
    case noSemanticVersionTag(String)
    case githubRepositoryRequired
    case githubRequestFailed(Int)
    case invalidGitHubResponse
    case commandFailed(command: String, status: Int32, output: String)
    case checkoutMismatch(expected: String, actual: String)
    case invalidPackage(String)
    case unexpectedPackageID(expected: String, actual: String)
    case versionMismatch(packageID: String, version: String, requirement: String)
    case missingDependencySource(packageID: String)
    case sourceConflict(packageID: String)
    case packageNotManaged(String)

    var errorDescription: String? {
        switch self {
        case .gitUnavailable:
            return "没有找到可用的 Git。"
        case .invalidRepositoryURL:
            return "仓库地址无效；仅支持 HTTPS、SSH URL 或 git@host:path 格式。"
        case let .missingSelectorValue(label):
            return "必须填写\(label)。"
        case let .remoteReferenceNotFound(reference):
            return "远程仓库中没有找到引用：\(reference)。"
        case let .noSemanticVersionTag(requirement):
            return requirement.isEmpty
                ? "远程仓库没有有效的 SemVer Tag。"
                : "没有找到满足 \(requirement) 的 SemVer Tag。"
        case .githubRepositoryRequired:
            return "Release 选择方式目前只支持 github.com 仓库。"
        case let .githubRequestFailed(status):
            return "GitHub Release 查询失败（HTTP \(status)）。"
        case .invalidGitHubResponse:
            return "GitHub Release 返回内容无效。"
        case let .commandFailed(command, status, output):
            let detail = output.trimmingCharacters(in: .whitespacesAndNewlines)
            return detail.isEmpty
                ? "\(command) 执行失败（退出码 \(status)）。"
                : "\(command) 执行失败（退出码 \(status)）：\n\(detail)"
        case let .checkoutMismatch(expected, actual):
            return "检出的 commit 与锁定结果不一致：期望 \(expected)，实际为 \(actual)。"
        case let .invalidPackage(message):
            return "远程仓库不是有效的 Codex Tweaks 功能包：\(message)"
        case let .unexpectedPackageID(expected, actual):
            return "功能包标识不匹配：期望 \(expected)，实际为 \(actual)。"
        case let .versionMismatch(packageID, version, requirement):
            return "功能包 \(packageID) 的 v\(version) 不满足版本要求 \(requirement)。"
        case let .missingDependencySource(packageID):
            return "缺少依赖 \(packageID)，但清单没有提供可安装的 Git 来源。"
        case let .sourceConflict(packageID):
            return "依赖 \(packageID) 声明的仓库与本机已登记来源不一致。"
        case let .packageNotManaged(packageID):
            return "功能包 \(packageID) 不是由 Git 管理的包。"
        }
    }
}

actor TweakPackageRemoteManager {
    private struct GitHubPackageRelease: Decodable {
        let tagName: String

        private enum CodingKeys: String, CodingKey {
            case tagName = "tag_name"
        }
    }

    private struct GitCommandResult {
        let status: Int32
        let output: String
    }

    private let store: TweakPackageStore
    private let fileManager: FileManager
    private let session: URLSession
    private let processEnvironment: [String: String]

    init(
        store: TweakPackageStore,
        fileManager: FileManager = .default,
        session: URLSession = .shared,
        processEnvironment: [String: String] = ProcessInfo.processInfo.environment
    ) {
        self.store = store
        self.fileManager = fileManager
        self.session = session
        self.processEnvironment = processEnvironment
    }

    func detectGitEnvironment() async -> GitEnvironment? {
        for gitURL in Self.gitCandidates(
            fileManager: fileManager,
            environment: processEnvironment
        ) {
            guard fileManager.isExecutableFile(atPath: gitURL.path),
                  let result = try? await run(
                    executableURL: gitURL,
                    arguments: ["--version"],
                    currentDirectoryURL: fileManager.temporaryDirectory
                  ),
                  result.status == 0 else {
                continue
            }
            return GitEnvironment(
                gitURL: gitURL,
                version: result.output.trimmingCharacters(in: .whitespacesAndNewlines)
            )
        }
        return nil
    }

    func managedPackageIDs() throws -> [String] {
        try loadRegistry().packages.keys.sorted()
    }

    func registration(for packageID: String) throws -> TweakManagedPackageRegistration? {
        try loadRegistry().packages[packageID]
    }

    func install(
        source: TweakPackageSource,
        expectedPackageID: String? = nil,
        requirement: String? = nil
    ) async throws -> TweakManagedPackageInstallResult {
        try store.prepareUserPackages(fileManager: fileManager)
        guard let git = await detectGitEnvironment() else {
            throw TweakPackageRemoteError.gitUnavailable
        }
        try Self.validate(source: source)

        var registry = try loadRegistry()
        var lockfile = try store.loadManagedLockfile(fileManager: fileManager)
        if let expectedPackageID,
           let existing = registry.packages[expectedPackageID],
           !Self.sourcesMatch(existing.source, source) {
            throw TweakPackageRemoteError.sourceConflict(packageID: expectedPackageID)
        }

        let cachedRegistration = expectedPackageID.flatMap { registry.packages[$0] }
            ?? registry.packages.values.first { Self.sourcesMatch($0.source, source) }
        var versionRequirements = cachedRegistration?.versionRequirements ?? []
        if let requirement, !versionRequirements.contains(requirement) {
            versionRequirements.append(requirement)
        }
        let resolution = try await resolve(
            source: source,
            requirements: versionRequirements,
            cachedRegistration: cachedRegistration
        )
        let stagingURL = store.managedPackagesDirectoryURL.appendingPathComponent(
            ".staging-\(UUID().uuidString)",
            isDirectory: true
        )
        try fileManager.createDirectory(
            at: stagingURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )

        do {
            try await checkout(
                source: source,
                resolution: resolution,
                git: git,
                destinationURL: stagingURL
            )
            try? fileManager.removeItem(at: stagingURL.appendingPathComponent(".git"))
            let inspected = store.inspectPackage(
                at: stagingURL,
                fileManager: fileManager
            )
            guard inspected.validationError == nil, let manifest = inspected.manifest else {
                throw TweakPackageRemoteError.invalidPackage(
                    inspected.validationError ?? "无法读取 package.json。"
                )
            }
            if let expectedPackageID, manifest.name != expectedPackageID {
                throw TweakPackageRemoteError.unexpectedPackageID(
                    expected: expectedPackageID,
                    actual: manifest.name
                )
            }
            for requirement in versionRequirements {
                if let parsedRequirement = SemanticVersionRequirement(requirement),
                   !parsedRequirement.contains(manifest.version) {
                    throw TweakPackageRemoteError.versionMismatch(
                        packageID: manifest.name,
                        version: manifest.version,
                        requirement: requirement
                    )
                }
            }

            if let existing = registry.packages[manifest.name],
               !Self.sourcesMatch(existing.source, source) {
                throw TweakPackageRemoteError.sourceConflict(packageID: manifest.name)
            }

            let packageSourceRoot = store.managedSourcesDirectoryURL.appendingPathComponent(
                TweakPackageStore.fingerprint(manifest.name),
                isDirectory: true
            )
            try fileManager.createDirectory(
                at: packageSourceRoot,
                withIntermediateDirectories: true,
                attributes: [.posixPermissions: 0o700]
            )
            let finalSourceURL = packageSourceRoot.appendingPathComponent(
                resolution.commit.lowercased(),
                isDirectory: true
            )
            if fileManager.fileExists(atPath: finalSourceURL.path) {
                try fileManager.removeItem(at: stagingURL)
            } else {
                try fileManager.moveItem(at: stagingURL, to: finalSourceURL)
            }

            let relativePath = Self.relativePath(
                for: finalSourceURL,
                under: store.managedPackagesDirectoryURL
            )
            let now = Date()
            let lock = TweakManagedPackageLock(
                packageID: manifest.name,
                packageVersion: manifest.version,
                source: source,
                resolvedReference: resolution.reference,
                resolvedCommit: resolution.commit.lowercased(),
                sourceRelativePath: relativePath,
                installedAt: now
            )
            let existingRegistration = registry.packages[manifest.name]
            registry.packages[manifest.name] = TweakManagedPackageRegistration(
                packageID: manifest.name,
                source: source,
                addedAt: existingRegistration?.addedAt ?? now,
                versionRequirements: versionRequirements,
                lastCheckedAt: now,
                remoteETag: resolution.etag,
                lastResolvedReference: resolution.reference,
                lastResolvedCommit: resolution.commit.lowercased()
            )
            lockfile.packages[manifest.name] = lock
            try writeRegistry(registry)
            try writeLockfile(lockfile)
            return TweakManagedPackageInstallResult(
                packageID: manifest.name,
                manifest: manifest,
                lock: lock
            )
        } catch {
            try? fileManager.removeItem(at: stagingURL)
            throw error
        }
    }

    func installMissingDependencies(
        for package: TweakPackage,
        installedPackages: [TweakPackage]
    ) async throws -> [TweakManagedPackageInstallResult] {
        guard let manifest = package.manifest else {
            throw TweakPackageRemoteError.invalidPackage("根功能包配置无效。")
        }
        var available: [String: (TweakPackage, TweakPackageManifest)] = [:]
        for item in installedPackages where item.validationError == nil {
            guard let installedManifest = item.manifest,
                  available[installedManifest.name] == nil
            else {
                continue
            }
            available[installedManifest.name] = (item, installedManifest)
        }
        var pending = manifest.codexTweaks.packageDependencies.map {
            (parentID: manifest.name, dependencyID: $0.key, dependency: $0.value)
        }
        var expanded = Set<String>()
        var results: [TweakManagedPackageInstallResult] = []

        while !pending.isEmpty {
            let item = pending.removeFirst()
            guard let requirement = SemanticVersionRequirement(item.dependency.version) else {
                throw TweakPackageRemoteError.versionMismatch(
                    packageID: item.dependencyID,
                    version: "—",
                    requirement: item.dependency.version
                )
            }
            if let (installed, installedManifest) = available[item.dependencyID] {
                if !requirement.contains(installedManifest.version) {
                    throw TweakPackageRemoteError.versionMismatch(
                        packageID: item.dependencyID,
                        version: installedManifest.version,
                        requirement: item.dependency.version
                    )
                }
                if let source = item.dependency.source,
                   let lock = installed.managedLock,
                   !Self.sourcesMatch(source, lock.source) {
                    throw TweakPackageRemoteError.sourceConflict(packageID: item.dependencyID)
                }
                if installed.isManaged {
                    try addVersionRequirement(
                        item.dependency.version,
                        forPackageID: item.dependencyID
                    )
                }
                if expanded.insert(item.dependencyID).inserted {
                    pending.append(contentsOf: installedManifest.codexTweaks.packageDependencies.map {
                        (item.dependencyID, $0.key, $0.value)
                    })
                }
                continue
            }

            guard let source = item.dependency.source else {
                throw TweakPackageRemoteError.missingDependencySource(
                    packageID: item.dependencyID
                )
            }
            let result = try await install(
                source: source,
                expectedPackageID: item.dependencyID,
                requirement: item.dependency.version
            )
            results.append(result)
            let installed = TweakPackage(
                id: result.packageID,
                directoryName: result.packageID,
                directoryURL: store.managedPackagesDirectoryURL
                    .appendingPathComponent(result.lock.sourceRelativePath),
                manifest: result.manifest,
                sourceFingerprint: nil,
                dependencyFingerprint: nil,
                activeBuild: nil,
                validationError: nil,
                priorityOverride: nil,
                origin: .managed(result.lock)
            )
            available[result.packageID] = (installed, result.manifest)
            if expanded.insert(result.packageID).inserted {
                pending.append(contentsOf: result.manifest.codexTweaks.packageDependencies.map {
                    (result.packageID, $0.key, $0.value)
                })
            }
        }
        return results
    }

    func checkForUpdate(packageID: String) async throws -> TweakPackageRemoteUpdate {
        var registry = try loadRegistry()
        let lockfile = try store.loadManagedLockfile(fileManager: fileManager)
        guard var registration = registry.packages[packageID],
              let lock = lockfile.packages[packageID] else {
            throw TweakPackageRemoteError.packageNotManaged(packageID)
        }
        let checkedAt = Date()
        if registration.source.selector.type == .commit {
            registration.lastCheckedAt = checkedAt
            registry.packages[packageID] = registration
            try writeRegistry(registry)
            return TweakPackageRemoteUpdate(
                packageID: packageID,
                currentCommit: lock.resolvedCommit,
                candidateReference: lock.resolvedReference,
                candidateCommit: lock.resolvedCommit,
                checkedAt: checkedAt,
                status: .current
            )
        }

        let resolution = try await resolve(
            source: registration.source,
            requirements: registration.versionRequirements,
            cachedRegistration: registration
        )
        registration.lastCheckedAt = checkedAt
        registration.remoteETag = resolution.etag ?? registration.remoteETag
        registration.lastResolvedReference = resolution.reference
        registration.lastResolvedCommit = resolution.commit.lowercased()
        registry.packages[packageID] = registration
        try writeRegistry(registry)

        let candidateCommit = resolution.commit.lowercased()
        let status: TweakPackageRemoteUpdateStatus
        if candidateCommit == lock.resolvedCommit.lowercased() {
            status = .current
        } else if registration.source.selector.type.isPinned {
            status = .pinnedReferenceChanged
        } else {
            status = .available
        }
        return TweakPackageRemoteUpdate(
            packageID: packageID,
            currentCommit: lock.resolvedCommit,
            candidateReference: resolution.reference,
            candidateCommit: candidateCommit,
            checkedAt: checkedAt,
            status: status
        )
    }

    private func resolve(
        source: TweakPackageSource,
        requirements: [String],
        cachedRegistration: TweakManagedPackageRegistration?
    ) async throws -> TweakPackageRemoteResolution {
        guard let git = await detectGitEnvironment() else {
            throw TweakPackageRemoteError.gitUnavailable
        }
        switch source.selector.type {
        case .branch:
            let branch = try Self.requiredSelectorValue(source.selector)
            let ref = "refs/heads/\(branch)"
            let commit = try await resolveSingleRef(ref, source: source, git: git)
            return TweakPackageRemoteResolution(
                reference: branch,
                commit: commit,
                fetchReference: ref,
                etag: nil
            )
        case .tag:
            return try await resolveTag(
                try Self.requiredSelectorValue(source.selector),
                source: source,
                git: git,
                etag: nil
            )
        case .latestSemverTag:
            let result = try await gitCommand(
                git: git,
                arguments: ["ls-remote", "--tags", "--refs", source.url]
            )
            let parsedRequirements = requirements.compactMap(SemanticVersionRequirement.init)
            let candidates: [(name: String, version: SemanticVersion)] = result.output
                .split(separator: "\n")
                .compactMap { line in
                    let columns = line.split(whereSeparator: { $0 == "\t" || $0 == " " })
                    guard columns.count >= 2,
                          columns[1].hasPrefix("refs/tags/") else { return nil }
                    let name = String(columns[1].dropFirst("refs/tags/".count))
                    guard let version = SemanticVersion(name),
                          parsedRequirements.allSatisfy({ $0.contains(name) }) else { return nil }
                    return (name, version)
                }
            guard let selected = candidates.max(by: { $0.version < $1.version }) else {
                throw TweakPackageRemoteError.noSemanticVersionTag(
                    requirements.joined(separator: " ∩ ")
                )
            }
            return try await resolveTag(
                selected.name,
                source: source,
                git: git,
                etag: nil
            )
        case .githubLatestRelease, .githubRelease:
            let release = try await resolveGitHubRelease(
                source: source,
                cachedRegistration: cachedRegistration
            )
            return try await resolveTag(
                release.tag,
                source: source,
                git: git,
                etag: release.etag
            )
        case .commit:
            let commit = try Self.requiredSelectorValue(source.selector).lowercased()
            guard Self.isFullCommitSHA(commit) else {
                throw TweakPackageRemoteError.remoteReferenceNotFound(commit)
            }
            return TweakPackageRemoteResolution(
                reference: commit,
                commit: commit,
                fetchReference: commit,
                etag: nil
            )
        }
    }

    private func resolveSingleRef(
        _ ref: String,
        source: TweakPackageSource,
        git: GitEnvironment
    ) async throws -> String {
        let result = try await gitCommand(
            git: git,
            arguments: ["ls-remote", source.url, ref]
        )
        guard let line = result.output.split(separator: "\n").first,
              let commit = line.split(whereSeparator: { $0 == "\t" || $0 == " " }).first else {
            throw TweakPackageRemoteError.remoteReferenceNotFound(ref)
        }
        return String(commit).lowercased()
    }

    private func resolveTag(
        _ tag: String,
        source: TweakPackageSource,
        git: GitEnvironment,
        etag: String?
    ) async throws -> TweakPackageRemoteResolution {
        let ref = "refs/tags/\(tag)"
        let result = try await gitCommand(
            git: git,
            arguments: ["ls-remote", "--tags", source.url, ref, "\(ref)^{}"]
        )
        let lines = result.output.split(separator: "\n")
        let peeled = lines.first { $0.contains("\(ref)^{}") } ?? lines.first
        guard let peeled,
              let commit = peeled.split(whereSeparator: { $0 == "\t" || $0 == " " }).first else {
            throw TweakPackageRemoteError.remoteReferenceNotFound(tag)
        }
        return TweakPackageRemoteResolution(
            reference: tag,
            commit: String(commit).lowercased(),
            fetchReference: ref,
            etag: etag
        )
    }

    private func resolveGitHubRelease(
        source: TweakPackageSource,
        cachedRegistration: TweakManagedPackageRegistration?
    ) async throws -> (tag: String, etag: String?) {
        guard let repository = Self.githubRepository(from: source.url) else {
            throw TweakPackageRemoteError.githubRepositoryRequired
        }
        let path: String
        if source.selector.type == .githubLatestRelease {
            path = "repos/\(repository.owner)/\(repository.repo)/releases/latest"
        } else {
            let tag = try Self.requiredSelectorValue(source.selector)
            guard let encodedTag = tag.addingPercentEncoding(
                withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/"))
            ) else {
                throw TweakPackageRemoteError.invalidGitHubResponse
            }
            path = "repos/\(repository.owner)/\(repository.repo)/releases/tags/\(encodedTag)"
        }
        guard let url = URL(string: "https://api.github.com/\(path)") else {
            throw TweakPackageRemoteError.invalidGitHubResponse
        }
        var request = URLRequest(url: url)
        request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        request.setValue("Codex-Tweaks", forHTTPHeaderField: "User-Agent")
        request.setValue("2022-11-28", forHTTPHeaderField: "X-GitHub-Api-Version")
        if let etag = cachedRegistration?.remoteETag {
            request.setValue(etag, forHTTPHeaderField: "If-None-Match")
        }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw TweakPackageRemoteError.invalidGitHubResponse
        }
        if http.statusCode == 304,
           let cachedTag = cachedRegistration?.lastResolvedReference {
            return (cachedTag, cachedRegistration?.remoteETag)
        }
        guard http.statusCode == 200 else {
            throw TweakPackageRemoteError.githubRequestFailed(http.statusCode)
        }
        let release = try JSONDecoder().decode(GitHubPackageRelease.self, from: data)
        return (release.tagName, http.value(forHTTPHeaderField: "ETag"))
    }

    private func checkout(
        source: TweakPackageSource,
        resolution: TweakPackageRemoteResolution,
        git: GitEnvironment,
        destinationURL: URL
    ) async throws {
        try await requireSuccess(
            git: git,
            arguments: ["init", "--quiet", destinationURL.path],
            command: "git init"
        )
        try await requireSuccess(
            git: git,
            arguments: ["-C", destinationURL.path, "remote", "add", "origin", source.url],
            command: "git remote add"
        )
        try await requireSuccess(
            git: git,
            arguments: [
                "-C", destinationURL.path,
                "fetch", "--quiet", "--depth", "1", "--no-tags",
                "origin", resolution.fetchReference,
            ],
            command: "git fetch"
        )
        try await requireSuccess(
            git: git,
            arguments: ["-C", destinationURL.path, "checkout", "--quiet", "--detach", "FETCH_HEAD"],
            command: "git checkout"
        )
        let actual = try await gitCommand(
            git: git,
            arguments: ["-C", destinationURL.path, "rev-parse", "HEAD"]
        ).output.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard actual == resolution.commit.lowercased() else {
            throw TweakPackageRemoteError.checkoutMismatch(
                expected: resolution.commit,
                actual: actual
            )
        }
    }

    private func gitCommand(
        git: GitEnvironment,
        arguments: [String]
    ) async throws -> GitCommandResult {
        let result = try await run(
            executableURL: git.gitURL,
            arguments: arguments,
            currentDirectoryURL: store.managedPackagesDirectoryURL
        )
        guard result.status == 0 else {
            throw TweakPackageRemoteError.commandFailed(
                command: "git \(arguments.first ?? "")",
                status: result.status,
                output: String(result.output.suffix(8_000))
            )
        }
        return result
    }

    private func requireSuccess(
        git: GitEnvironment,
        arguments: [String],
        command: String
    ) async throws {
        let result = try await run(
            executableURL: git.gitURL,
            arguments: arguments,
            currentDirectoryURL: store.managedPackagesDirectoryURL
        )
        guard result.status == 0 else {
            throw TweakPackageRemoteError.commandFailed(
                command: command,
                status: result.status,
                output: String(result.output.suffix(8_000))
            )
        }
    }

    private func loadRegistry() throws -> TweakManagedPackageRegistry {
        guard fileManager.fileExists(atPath: store.managedRegistryURL.path) else {
            return TweakManagedPackageRegistry()
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(
            TweakManagedPackageRegistry.self,
            from: Data(contentsOf: store.managedRegistryURL)
        )
    }

    private func writeRegistry(_ registry: TweakManagedPackageRegistry) throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        try encoder.encode(registry).write(to: store.managedRegistryURL, options: .atomic)
    }

    private func writeLockfile(_ lockfile: TweakManagedPackageLockfile) throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        try encoder.encode(lockfile).write(to: store.managedLockfileURL, options: .atomic)
    }

    private func addVersionRequirement(
        _ requirement: String,
        forPackageID packageID: String
    ) throws {
        var registry = try loadRegistry()
        guard var registration = registry.packages[packageID] else { return }
        if !registration.versionRequirements.contains(requirement) {
            registration.versionRequirements.append(requirement)
            registry.packages[packageID] = registration
            try writeRegistry(registry)
        }
    }

    private func run(
        executableURL: URL,
        arguments: [String],
        currentDirectoryURL: URL
    ) async throws -> GitCommandResult {
        try await Task.detached(priority: .utility) {
            let temporaryDirectory = FileManager.default.temporaryDirectory
                .appendingPathComponent(UUID().uuidString, isDirectory: true)
            try FileManager.default.createDirectory(
                at: temporaryDirectory,
                withIntermediateDirectories: true
            )
            defer { try? FileManager.default.removeItem(at: temporaryDirectory) }

            let stdoutURL = temporaryDirectory.appendingPathComponent("stdout.log")
            let stderrURL = temporaryDirectory.appendingPathComponent("stderr.log")
            FileManager.default.createFile(atPath: stdoutURL.path, contents: nil)
            FileManager.default.createFile(atPath: stderrURL.path, contents: nil)
            let stdout = try FileHandle(forWritingTo: stdoutURL)
            let stderr = try FileHandle(forWritingTo: stderrURL)
            defer {
                try? stdout.close()
                try? stderr.close()
            }

            var environment = self.processEnvironment
            environment["GIT_TERMINAL_PROMPT"] = "0"
            environment["GIT_ASKPASS"] = "/usr/bin/false"
            environment["GIT_SSH_COMMAND"] = "ssh -o BatchMode=yes"
            let process = Process()
            process.executableURL = executableURL
            process.arguments = arguments
            process.currentDirectoryURL = currentDirectoryURL
            process.environment = environment
            process.standardOutput = stdout
            process.standardError = stderr
            try process.run()
            process.waitUntilExit()
            try stdout.synchronize()
            try stderr.synchronize()

            let output = String(
                decoding: try Data(contentsOf: stdoutURL) + Data(contentsOf: stderrURL),
                as: UTF8.self
            )
            return GitCommandResult(status: process.terminationStatus, output: output)
        }.value
    }

    static func gitCandidates(
        fileManager: FileManager = .default,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) -> [URL] {
        var candidates: [URL] = []
        for path in (environment["PATH"] ?? "").split(separator: ":") {
            candidates.append(
                URL(fileURLWithPath: String(path), isDirectory: true)
                    .appendingPathComponent("git")
            )
        }
        for path in ["/usr/bin/git", "/opt/homebrew/bin/git", "/usr/local/bin/git"] {
            candidates.append(URL(fileURLWithPath: path))
        }
        var seen = Set<String>()
        return candidates.filter { seen.insert($0.standardizedFileURL.path).inserted }
    }

    nonisolated static func validate(source: TweakPackageSource) throws {
        let value = source.url.trimmingCharacters(in: .whitespacesAndNewlines)
        let isSCPStyle = value.range(
            of: #"^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[^\s]+$"#,
            options: .regularExpression
        ) != nil
        if !isSCPStyle {
            guard let url = URL(string: value),
                  let scheme = url.scheme?.lowercased(),
                  ["https", "ssh"].contains(scheme),
                  url.host != nil,
                  url.password == nil,
                  scheme != "https" || url.user == nil else {
                throw TweakPackageRemoteError.invalidRepositoryURL
            }
        }
        if let label = source.selector.type.valueLabel,
           source.selector.value == nil {
            throw TweakPackageRemoteError.missingSelectorValue(label)
        }
        if [.githubLatestRelease, .githubRelease].contains(source.selector.type),
           githubRepository(from: source.url) == nil {
            throw TweakPackageRemoteError.githubRepositoryRequired
        }
    }

    private static func requiredSelectorValue(
        _ selector: TweakPackageRemoteSelector
    ) throws -> String {
        guard let value = selector.value else {
            throw TweakPackageRemoteError.missingSelectorValue(
                selector.type.valueLabel ?? "选择值"
            )
        }
        return value
    }

    private static func normalizedRepositoryURL(_ rawValue: String) -> String {
        var value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .lowercased()
        if value.hasSuffix(".git") { value.removeLast(4) }
        return value
    }

    static func sourcesMatch(
        _ lhs: TweakPackageSource,
        _ rhs: TweakPackageSource
    ) -> Bool {
        normalizedRepositoryURL(lhs.url) == normalizedRepositoryURL(rhs.url)
            && lhs.selector == rhs.selector
    }

    private static func githubRepository(from rawValue: String) -> (owner: String, repo: String)? {
        let value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        var path: String
        if value.hasPrefix("git@github.com:") {
            path = String(value.dropFirst("git@github.com:".count))
        } else if let url = URL(string: value),
                  url.host?.lowercased() == "github.com" {
            path = url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        } else {
            return nil
        }
        if path.hasSuffix(".git") { path.removeLast(4) }
        let components = path.split(separator: "/")
        guard components.count == 2 else { return nil }
        return (String(components[0]), String(components[1]))
    }

    private static func isFullCommitSHA(_ value: String) -> Bool {
        value.count == 40 && value.allSatisfy { $0.isHexDigit }
    }

    private static func relativePath(for url: URL, under directoryURL: URL) -> String {
        let root = directoryURL.standardizedFileURL.path + "/"
        let path = url.standardizedFileURL.path
        guard path.hasPrefix(root) else { return url.lastPathComponent }
        return String(path.dropFirst(root.count))
    }
}
