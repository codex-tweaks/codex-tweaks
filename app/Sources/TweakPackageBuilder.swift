import Foundation

struct NodeEnvironment: Equatable, Sendable {
    let nodeURL: URL
    let npmURL: URL
    let npxURL: URL
    let version: String
}

enum TweakPackageBuildError: LocalizedError {
    case invalidPackage
    case nodeUnavailable
    case lockfileRequired
    case commandFailed(command: String, status: Int32, output: String)
    case missingOutput

    var errorDescription: String? {
        switch self {
        case .invalidPackage:
            return "包配置无效，无法编译。"
        case .nodeUnavailable:
            return "没有找到可用的 Node.js、npm 和 npx。"
        case .lockfileRequired:
            return "包含 npm 依赖的包必须提供 package-lock.json。"
        case let .commandFailed(command, status, output):
            let detail = output.trimmingCharacters(in: .whitespacesAndNewlines)
            if detail.isEmpty {
                return "\(command) 执行失败（退出码 \(status)）。"
            }
            return "\(command) 执行失败（退出码 \(status)）：\n\(detail)"
        case .missingOutput:
            return "编译器没有生成 bundle.js。"
        }
    }
}

actor TweakPackageBuilder {
    static let dependencyInstallArguments = [
        "ci",
        "--ignore-scripts",
        "--no-audit",
        "--no-fund",
    ]

    private let store: TweakPackageStore
    private let fileManager: FileManager

    init(store: TweakPackageStore, fileManager: FileManager = .default) {
        self.store = store
        self.fileManager = fileManager
    }

    func detectNodeEnvironment() async -> NodeEnvironment? {
        for nodeURL in Self.nodeCandidates(fileManager: fileManager) {
            let binURL = nodeURL.deletingLastPathComponent()
            let npmURL = binURL.appendingPathComponent("npm")
            let npxURL = binURL.appendingPathComponent("npx")
            guard fileManager.isExecutableFile(atPath: nodeURL.path),
                  fileManager.isExecutableFile(atPath: npmURL.path),
                  fileManager.isExecutableFile(atPath: npxURL.path) else {
                continue
            }
            guard let result = try? await run(
                executableURL: nodeURL,
                arguments: ["--version"],
                currentDirectoryURL: binURL,
                environment: Self.processEnvironment(nodeBinURL: binURL)
            ), result.status == 0 else {
                continue
            }
            let version = result.output.trimmingCharacters(in: .whitespacesAndNewlines)
            return NodeEnvironment(
                nodeURL: nodeURL,
                npmURL: npmURL,
                npxURL: npxURL,
                version: version
            )
        }
        return nil
    }

    func build(
        package: TweakPackage,
        installDependencies: Bool,
        allowCompilerDownload: Bool
    ) async throws -> TweakPackageBuildRecord {
        guard package.validationError == nil,
              let manifest = package.manifest,
              let sourceFingerprint = package.sourceFingerprint,
              let dependencyFingerprint = package.dependencyFingerprint else {
            throw TweakPackageBuildError.invalidPackage
        }
        guard let node = await detectNodeEnvironment() else {
            throw TweakPackageBuildError.nodeUnavailable
        }

        let environment = Self.processEnvironment(
            nodeBinURL: node.nodeURL.deletingLastPathComponent()
        )
        if installDependencies, !manifest.dependencies.isEmpty {
            let lockfileURL = package.directoryURL.appendingPathComponent("package-lock.json")
            guard fileManager.fileExists(atPath: lockfileURL.path) else {
                throw TweakPackageBuildError.lockfileRequired
            }
            let result = try await run(
                executableURL: node.npmURL,
                arguments: Self.dependencyInstallArguments,
                currentDirectoryURL: package.directoryURL,
                environment: environment
            )
            try Self.requireSuccess(result, command: "npm ci")
        }

        let buildsURL = try store.buildsDirectoryURL(forPackageID: package.id)
        let stagingURL = buildsURL.appendingPathComponent(
            ".staging-\(UUID().uuidString)",
            isDirectory: true
        )
        try fileManager.createDirectory(
            at: stagingURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )

        do {
            let entryURL = package.directoryURL
                .appendingPathComponent(manifest.codexTweaks.entry)
                .standardizedFileURL
            let javaScriptURL = stagingURL.appendingPathComponent("bundle.js")
            let arguments = Self.esbuildArguments(
                entryURL: entryURL,
                outputURL: javaScriptURL,
                allowCompilerDownload: allowCompilerDownload
            )

            let result = try await run(
                executableURL: node.npxURL,
                arguments: arguments,
                currentDirectoryURL: package.directoryURL,
                environment: environment
            )
            try Self.requireSuccess(result, command: "esbuild")
            guard fileManager.fileExists(atPath: javaScriptURL.path) else {
                throw TweakPackageBuildError.missingOutput
            }

            let hasCSS = fileManager.fileExists(
                atPath: stagingURL.appendingPathComponent("bundle.css").path
            )
            let buildDirectoryName = sourceFingerprint
                + "-esbuild-"
                + TweakPackageStore.compilerVersion
                + "-"
                + String(UUID().uuidString.prefix(8)).lowercased()
            let finalURL = buildsURL.appendingPathComponent(
                buildDirectoryName,
                isDirectory: true
            )
            try fileManager.moveItem(at: stagingURL, to: finalURL)

            let record = TweakPackageBuildRecord(
                packageID: manifest.name,
                packageVersion: manifest.version,
                packageDependencies: manifest.codexTweaks.packageDependencies.mapValues(\.version),
                sourceFingerprint: sourceFingerprint,
                dependencyFingerprint: dependencyFingerprint,
                compilerVersion: TweakPackageStore.compilerVersion,
                nodeVersion: node.version,
                buildDirectoryName: buildDirectoryName,
                hasCSS: hasCSS,
                builtAt: Date()
            )
            try store.activateBuild(record)
            return record
        } catch {
            try? fileManager.removeItem(at: stagingURL)
            throw error
        }
    }

    static func nodeCandidates(
        fileManager: FileManager = .default,
        environment: [String: String] = ProcessInfo.processInfo.environment,
        homeDirectoryURL: URL = FileManager.default.homeDirectoryForCurrentUser
    ) -> [URL] {
        var candidates: [URL] = []
        for path in (environment["PATH"] ?? "").split(separator: ":") {
            candidates.append(
                URL(fileURLWithPath: String(path), isDirectory: true)
                    .appendingPathComponent("node")
            )
        }

        for binPath in [
            "/opt/homebrew/bin",
            "/usr/local/bin",
            homeDirectoryURL.appendingPathComponent(".local/bin").path,
            homeDirectoryURL.appendingPathComponent(".local/share/mise/shims").path,
            homeDirectoryURL.appendingPathComponent(".volta/bin").path,
        ] {
            candidates.append(
                URL(fileURLWithPath: binPath, isDirectory: true)
                    .appendingPathComponent("node")
            )
        }

        let versionRoots = [
            homeDirectoryURL.appendingPathComponent(".local/share/mise/installs/node"),
            homeDirectoryURL.appendingPathComponent(".nvm/versions/node"),
        ]
        for root in versionRoots {
            guard let versions = try? fileManager.contentsOfDirectory(
                at: root,
                includingPropertiesForKeys: [.isDirectoryKey],
                options: [.skipsHiddenFiles]
            ) else { continue }
            for versionURL in versions.sorted(by: { $0.lastPathComponent > $1.lastPathComponent }) {
                candidates.append(
                    versionURL.appendingPathComponent("bin", isDirectory: true)
                        .appendingPathComponent("node")
                )
            }
        }

        var seen = Set<String>()
        return candidates.filter { seen.insert($0.standardizedFileURL.path).inserted }
    }

    static func esbuildArguments(
        entryURL: URL,
        outputURL: URL,
        allowCompilerDownload: Bool
    ) -> [String] {
        var arguments = ["--yes"]
        if !allowCompilerDownload { arguments.append("--offline") }
        arguments += [
            "esbuild@\(TweakPackageStore.compilerVersion)",
            entryURL.path,
            "--bundle",
            "--platform=browser",
            "--format=cjs",
            "--target=chrome120",
            "--sourcemap=inline",
            "--log-level=warning",
            "--define:process.env.NODE_ENV=\"production\"",
            "--outfile=\(outputURL.path)",
        ]
        return arguments
    }

    private static func processEnvironment(nodeBinURL: URL) -> [String: String] {
        var environment = ProcessInfo.processInfo.environment
        let existingPath = environment["PATH"] ?? "/usr/bin:/bin:/usr/sbin:/sbin"
        environment["PATH"] = nodeBinURL.path + ":" + existingPath
        environment["NO_UPDATE_NOTIFIER"] = "1"
        return environment
    }

    private static func requireSuccess(_ result: CommandResult, command: String) throws {
        guard result.status == 0 else {
            throw TweakPackageBuildError.commandFailed(
                command: command,
                status: result.status,
                output: String(result.output.suffix(8_000))
            )
        }
    }

    private func run(
        executableURL: URL,
        arguments: [String],
        currentDirectoryURL: URL,
        environment: [String: String]
    ) async throws -> CommandResult {
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

            let stdoutData = try Data(contentsOf: stdoutURL)
            let stderrData = try Data(contentsOf: stderrURL)
            let combined = String(decoding: stdoutData + stderrData, as: UTF8.self)
            return CommandResult(status: process.terminationStatus, output: combined)
        }.value
    }
}

private struct CommandResult {
    let status: Int32
    let output: String
}
