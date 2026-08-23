import Foundation

struct TweakLocalPackageInstallResult: Equatable, Sendable {
    let packageID: String
    let manifest: TweakPackageManifest
    let directoryURL: URL
}

enum TweakPackageLocalInstallError: LocalizedError {
    case unsupportedSelection
    case packageRootNotFound
    case ambiguousPackageRoots
    case invalidArchive(String)
    case unsafeArchiveEntry(String)
    case symbolicLinkNotAllowed(String)
    case unsupportedFile(String)
    case tooManyFiles(Int)
    case packageTooLarge(Int64)
    case invalidPackage(String)
    case packageAlreadyInstalled(String)
    case destinationExists(String)

    var errorDescription: String? {
        switch self {
        case .unsupportedSelection:
            return "请选择一个功能包目录或 ZIP 压缩包。"
        case .packageRootNotFound:
            return "没有找到 package.json；它必须位于所选目录或 ZIP 的唯一一级目录中。"
        case .ambiguousPackageRoots:
            return "检测到多个包含 package.json 的目录，无法确定要安装哪个功能包。"
        case let .invalidArchive(message):
            return "ZIP 压缩包无效：\(message)"
        case let .unsafeArchiveEntry(path):
            return "ZIP 包含不安全的路径：\(path)"
        case let .symbolicLinkNotAllowed(path):
            return "功能包不能包含符号链接：\(path)"
        case let .unsupportedFile(path):
            return "功能包包含不支持的特殊文件：\(path)"
        case let .tooManyFiles(limit):
            return "功能包文件数量超过上限（\(limit) 个）。"
        case let .packageTooLarge(limit):
            return "功能包解压后大小超过上限（\(ByteCountFormatter.string(fromByteCount: limit, countStyle: .file))）。"
        case let .invalidPackage(message):
            return "不是有效的 Codex Tweaks 功能包：\(message)"
        case let .packageAlreadyInstalled(packageID):
            return "功能包 \(packageID) 已经安装。"
        case let .destinationExists(name):
            return "本地 packages 目录中已经存在目录：\(name)"
        }
    }
}

actor TweakPackageLocalInstaller {
    private struct CommandResult {
        let status: Int32
        let output: String
    }

    private static let maximumFileCount = 20_000
    private static let maximumExpandedSize: Int64 = 1_073_741_824
    private static let maximumArchiveSize: Int64 = 536_870_912
    private static let ignoredRootNames: Set<String> = [".git", "node_modules", "__MACOSX"]

    private let store: TweakPackageStore
    private let fileManager: FileManager

    init(
        store: TweakPackageStore,
        fileManager: FileManager = .default
    ) {
        self.store = store
        self.fileManager = fileManager
    }

    func install(from sourceURL: URL) throws -> TweakLocalPackageInstallResult {
        let hasSecurityScope = sourceURL.startAccessingSecurityScopedResource()
        defer {
            if hasSecurityScope { sourceURL.stopAccessingSecurityScopedResource() }
        }

        try store.prepareUserPackages(fileManager: fileManager)
        let containerURL = store.packagesDirectoryURL.appendingPathComponent(
            ".installing-\(UUID().uuidString)",
            isDirectory: true
        )
        try fileManager.createDirectory(
            at: containerURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        defer { try? fileManager.removeItem(at: containerURL) }

        let sourceValues = try sourceURL.resourceValues(
            forKeys: [.isDirectoryKey, .isRegularFileKey, .fileSizeKey]
        )
        let packageSourceURL: URL
        if sourceValues.isDirectory == true {
            packageSourceURL = try locatePackageRoot(in: sourceURL)
        } else if sourceValues.isRegularFile == true,
                  sourceURL.pathExtension.lowercased() == "zip" {
            let archiveSize = Int64(sourceValues.fileSize ?? 0)
            guard archiveSize <= Self.maximumArchiveSize else {
                throw TweakPackageLocalInstallError.packageTooLarge(Self.maximumArchiveSize)
            }
            try validateArchive(at: sourceURL)
            let extractionURL = containerURL.appendingPathComponent("extracted", isDirectory: true)
            try fileManager.createDirectory(at: extractionURL, withIntermediateDirectories: true)
            let extraction = try run(
                executableURL: URL(fileURLWithPath: "/usr/bin/ditto"),
                arguments: ["-x", "-k", sourceURL.path, extractionURL.path]
            )
            guard extraction.status == 0 else {
                throw TweakPackageLocalInstallError.invalidArchive(
                    commandFailureDetail(extraction.output)
                )
            }
            packageSourceURL = try locatePackageRoot(in: extractionURL)
        } else {
            throw TweakPackageLocalInstallError.unsupportedSelection
        }

        let stagedPackageURL = containerURL.appendingPathComponent("package", isDirectory: true)
        try copySafePackageTree(from: packageSourceURL, to: stagedPackageURL)

        let inspected = store.inspectPackage(
            at: stagedPackageURL,
            fileManager: fileManager
        )
        guard inspected.validationError == nil, let manifest = inspected.manifest else {
            throw TweakPackageLocalInstallError.invalidPackage(
                inspected.validationError ?? "无法读取 package.json。"
            )
        }

        let existingPackages = try store.loadPackages(fileManager: fileManager)
        if existingPackages.contains(where: { $0.manifest?.name == manifest.name }) {
            throw TweakPackageLocalInstallError.packageAlreadyInstalled(manifest.name)
        }

        let directoryName = destinationDirectoryName(forPackageID: manifest.name)
        let destinationURL = store.packagesDirectoryURL.appendingPathComponent(
            directoryName,
            isDirectory: true
        )
        guard !fileManager.fileExists(atPath: destinationURL.path) else {
            throw TweakPackageLocalInstallError.destinationExists(directoryName)
        }

        try fileManager.moveItem(at: stagedPackageURL, to: destinationURL)
        return TweakLocalPackageInstallResult(
            packageID: manifest.name,
            manifest: manifest,
            directoryURL: destinationURL
        )
    }

    private func locatePackageRoot(in rootURL: URL) throws -> URL {
        if isRegularFile(rootURL.appendingPathComponent("package.json")) {
            return rootURL
        }

        let children = try fileManager.contentsOfDirectory(
            at: rootURL,
            includingPropertiesForKeys: [.isDirectoryKey, .isSymbolicLinkKey],
            options: []
        ).filter { child in
            child.lastPathComponent != ".DS_Store"
                && !Self.ignoredRootNames.contains(child.lastPathComponent)
        }
        let candidates = try children.filter { child in
            let values = try child.resourceValues(
                forKeys: [.isDirectoryKey, .isSymbolicLinkKey]
            )
            guard values.isDirectory == true, values.isSymbolicLink != true else { return false }
            return isRegularFile(child.appendingPathComponent("package.json"))
        }

        guard candidates.count <= 1 else {
            throw TweakPackageLocalInstallError.ambiguousPackageRoots
        }
        guard let candidate = candidates.first else {
            throw TweakPackageLocalInstallError.packageRootNotFound
        }
        return candidate
    }

    private func isRegularFile(_ url: URL) -> Bool {
        guard let values = try? url.resourceValues(
            forKeys: [.isRegularFileKey, .isSymbolicLinkKey]
        ) else {
            return false
        }
        return values.isRegularFile == true && values.isSymbolicLink != true
    }

    private func copySafePackageTree(from sourceURL: URL, to destinationURL: URL) throws {
        let rootValues = try sourceURL.resourceValues(
            forKeys: [.isDirectoryKey, .isSymbolicLinkKey]
        )
        guard rootValues.isDirectory == true else {
            throw TweakPackageLocalInstallError.packageRootNotFound
        }
        guard rootValues.isSymbolicLink != true else {
            throw TweakPackageLocalInstallError.symbolicLinkNotAllowed(
                sourceURL.lastPathComponent
            )
        }

        try fileManager.createDirectory(
            at: destinationURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        guard let enumerator = fileManager.enumerator(
            at: sourceURL,
            includingPropertiesForKeys: [
                .isDirectoryKey,
                .isRegularFileKey,
                .isSymbolicLinkKey,
                .fileSizeKey,
            ],
            options: [.skipsPackageDescendants]
        ) else {
            throw CocoaError(.fileReadUnknown)
        }

        var fileCount = 0
        var expandedSize: Int64 = 0
        for case let itemURL as URL in enumerator {
            let relativePath = relativePath(for: itemURL, under: sourceURL)
            let firstComponent = relativePath.split(separator: "/").first.map(String.init) ?? ""
            let values = try itemURL.resourceValues(
                forKeys: [
                    .isDirectoryKey,
                    .isRegularFileKey,
                    .isSymbolicLinkKey,
                    .fileSizeKey,
                ]
            )

            if Self.ignoredRootNames.contains(firstComponent) {
                if values.isDirectory == true { enumerator.skipDescendants() }
                continue
            }
            if itemURL.lastPathComponent == ".DS_Store" { continue }
            if values.isSymbolicLink == true {
                throw TweakPackageLocalInstallError.symbolicLinkNotAllowed(relativePath)
            }

            let targetURL = destinationURL.appendingPathComponent(relativePath)
            if values.isDirectory == true {
                try fileManager.createDirectory(
                    at: targetURL,
                    withIntermediateDirectories: true,
                    attributes: [.posixPermissions: 0o700]
                )
            } else if values.isRegularFile == true {
                fileCount += 1
                expandedSize += Int64(values.fileSize ?? 0)
                guard fileCount <= Self.maximumFileCount else {
                    throw TweakPackageLocalInstallError.tooManyFiles(Self.maximumFileCount)
                }
                guard expandedSize <= Self.maximumExpandedSize else {
                    throw TweakPackageLocalInstallError.packageTooLarge(Self.maximumExpandedSize)
                }
                try fileManager.createDirectory(
                    at: targetURL.deletingLastPathComponent(),
                    withIntermediateDirectories: true
                )
                try fileManager.copyItem(at: itemURL, to: targetURL)
            } else {
                throw TweakPackageLocalInstallError.unsupportedFile(relativePath)
            }
        }
    }

    private func validateArchive(at archiveURL: URL) throws {
        let namesResult = try run(
            executableURL: URL(fileURLWithPath: "/usr/bin/zipinfo"),
            arguments: ["-1", archiveURL.path]
        )
        guard namesResult.status == 0 else {
            throw TweakPackageLocalInstallError.invalidArchive(
                commandFailureDetail(namesResult.output)
            )
        }
        let entries = namesResult.output.split(
            separator: "\n",
            omittingEmptySubsequences: true
        ).map { String($0).trimmingCharacters(in: .newlines) }
        guard !entries.isEmpty else {
            throw TweakPackageLocalInstallError.invalidArchive("压缩包为空。")
        }
        guard entries.count <= Self.maximumFileCount else {
            throw TweakPackageLocalInstallError.tooManyFiles(Self.maximumFileCount)
        }
        for entry in entries {
            try validateArchivePath(entry)
        }

        let detailResult = try run(
            executableURL: URL(fileURLWithPath: "/usr/bin/zipinfo"),
            arguments: ["-lt", archiveURL.path]
        )
        guard detailResult.status == 0 else {
            throw TweakPackageLocalInstallError.invalidArchive(
                commandFailureDetail(detailResult.output)
            )
        }
        for line in detailResult.output.split(separator: "\n") {
            guard let kind = line.first else { continue }
            if kind == "l" {
                throw TweakPackageLocalInstallError.symbolicLinkNotAllowed("ZIP 中的符号链接")
            }
            if "bcps".contains(kind) {
                throw TweakPackageLocalInstallError.unsupportedFile("ZIP 中的特殊文件")
            }
        }
        if let expandedSize = archiveExpandedSize(from: detailResult.output),
           expandedSize > Self.maximumExpandedSize {
            throw TweakPackageLocalInstallError.packageTooLarge(Self.maximumExpandedSize)
        }
    }

    private func validateArchivePath(_ path: String) throws {
        var normalized = path
        while normalized.hasSuffix("/") { normalized.removeLast() }
        let components = normalized.split(
            separator: "/",
            omittingEmptySubsequences: false
        )
        guard !normalized.isEmpty,
              !normalized.hasPrefix("/"),
              !normalized.contains("\\"),
              !normalized.contains("\u{0}"),
              components.allSatisfy({ !$0.isEmpty && $0 != "." && $0 != ".." }) else {
            throw TweakPackageLocalInstallError.unsafeArchiveEntry(path)
        }
    }

    private func archiveExpandedSize(from output: String) -> Int64? {
        let pattern = #"([0-9]+) bytes uncompressed"#
        guard let expression = try? NSRegularExpression(pattern: pattern),
              let match = expression.firstMatch(
                in: output,
                range: NSRange(output.startIndex..., in: output)
              ),
              let range = Range(match.range(at: 1), in: output) else {
            return nil
        }
        return Int64(output[range])
    }

    private func destinationDirectoryName(forPackageID packageID: String) -> String {
        let allowed = CharacterSet.alphanumerics.union(
            CharacterSet(charactersIn: "._-")
        )
        let isSafe = !packageID.hasPrefix(".")
            && packageID.utf8.count <= 120
            && packageID.unicodeScalars.allSatisfy(allowed.contains)
        if isSafe { return packageID }

        var slug = packageID.unicodeScalars.map { scalar in
            allowed.contains(scalar) ? String(scalar) : "-"
        }.joined()
        while slug.contains("--") { slug = slug.replacingOccurrences(of: "--", with: "-") }
        slug = slug.trimmingCharacters(in: CharacterSet(charactersIn: ".-_"))
        if slug.isEmpty { slug = "package" }
        slug = String(slug.prefix(80))
        return "\(slug)-\(TweakPackageStore.fingerprint(packageID).prefix(8))"
    }

    private func relativePath(for url: URL, under directoryURL: URL) -> String {
        let directoryPath = directoryURL.standardizedFileURL.path + "/"
        let path = url.standardizedFileURL.path
        guard path.hasPrefix(directoryPath) else { return url.lastPathComponent }
        return String(path.dropFirst(directoryPath.count))
    }

    private func run(
        executableURL: URL,
        arguments: [String]
    ) throws -> CommandResult {
        let process = Process()
        let outputPipe = Pipe()
        process.executableURL = executableURL
        process.arguments = arguments
        process.standardOutput = outputPipe
        process.standardError = outputPipe
        try process.run()
        let outputData = outputPipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        return CommandResult(
            status: process.terminationStatus,
            output: String(decoding: outputData, as: UTF8.self)
        )
    }

    private func commandFailureDetail(_ output: String) -> String {
        let detail = output.trimmingCharacters(in: .whitespacesAndNewlines)
        return detail.isEmpty ? "系统解压工具执行失败。" : detail
    }
}
