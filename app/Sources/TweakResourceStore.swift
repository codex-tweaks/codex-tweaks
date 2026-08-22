import Foundation

struct TweakPayload: Equatable {
    let css: String
    let javascript: String
    let version: String
}

final class TweakResourceStore: @unchecked Sendable {
    let tweaksDirectoryURL: URL
    let customCSSURL: URL
    let customJavaScriptURL: URL
    let vendorDirectoryURL: URL
    let stylesDirectoryURL: URL
    let scriptsDirectoryURL: URL

    init(
        fileManager: FileManager = .default,
        applicationSupportURL: URL? = nil
    ) {
        let applicationSupport = applicationSupportURL ?? fileManager.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first!
        tweaksDirectoryURL = applicationSupport
            .appendingPathComponent("Codex Tweaks", isDirectory: true)
            .appendingPathComponent("Tweaks", isDirectory: true)
        customCSSURL = tweaksDirectoryURL.appendingPathComponent("ui.css")
        customJavaScriptURL = tweaksDirectoryURL.appendingPathComponent("ui.js")
        vendorDirectoryURL = tweaksDirectoryURL
            .appendingPathComponent("vendor", isDirectory: true)
        stylesDirectoryURL = tweaksDirectoryURL
            .appendingPathComponent("styles", isDirectory: true)
        scriptsDirectoryURL = tweaksDirectoryURL
            .appendingPathComponent("scripts", isDirectory: true)
    }

    func prepareUserTweaks(
        fileManager: FileManager = .default,
        bundle: Bundle = .main
    ) throws {
        try fileManager.createDirectory(
            at: tweaksDirectoryURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try fileManager.createDirectory(
            at: vendorDirectoryURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try fileManager.createDirectory(
            at: stylesDirectoryURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try fileManager.createDirectory(
            at: scriptsDirectoryURL,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        try copyDefaultIfMissing(
            named: "ui",
            extension: "css",
            to: customCSSURL,
            fileManager: fileManager,
            bundle: bundle
        )
        try copyDefaultIfMissing(
            named: "ui",
            extension: "js",
            to: customJavaScriptURL,
            fileManager: fileManager,
            bundle: bundle
        )
        try copyBundledDirectoryContentsIfMissing(
            named: "vendor",
            to: vendorDirectoryURL,
            fileManager: fileManager,
            bundle: bundle
        )
        try copyBundledDirectoryContentsIfMissing(
            named: "styles",
            to: stylesDirectoryURL,
            fileManager: fileManager,
            bundle: bundle
        )
        try copyBundledDirectoryContentsIfMissing(
            named: "scripts",
            to: scriptsDirectoryURL,
            fileManager: fileManager,
            bundle: bundle
        )
    }

    func loadPayload(
        fileManager: FileManager = .default,
        bundle: Bundle = .main
    ) throws -> TweakPayload {
        try prepareUserTweaks(fileManager: fileManager, bundle: bundle)
        let cssSources = try loadSourceSet(
            entryURL: customCSSURL,
            vendorDirectoryURL: vendorDirectoryURL,
            moduleDirectoryURL: stylesDirectoryURL,
            fileExtension: "css",
            separator: "\n",
            wrapJavaScriptModules: false,
            fileManager: fileManager
        )
        let javaScriptSources = try loadSourceSet(
            entryURL: customJavaScriptURL,
            vendorDirectoryURL: vendorDirectoryURL,
            moduleDirectoryURL: scriptsDirectoryURL,
            fileExtension: "js",
            separator: "\n;\n",
            wrapJavaScriptModules: true,
            fileManager: fileManager
        )
        let version = Self.fingerprint(
            cssSources.fingerprintMaterial
                + "\u{0}"
                + javaScriptSources.fingerprintMaterial
        )
        return TweakPayload(
            css: cssSources.source,
            javascript: javaScriptSources.source,
            version: version
        )
    }

    static func fingerprint(_ value: String) -> String {
        var hash: UInt64 = 14_695_981_039_346_656_037
        for byte in value.utf8 {
            hash ^= UInt64(byte)
            hash &*= 1_099_511_628_211
        }
        return String(format: "%016llx", hash)
    }

    private func copyDefaultIfMissing(
        named name: String,
        extension fileExtension: String,
        to destination: URL,
        fileManager: FileManager,
        bundle: Bundle
    ) throws {
        guard !fileManager.fileExists(atPath: destination.path) else { return }
        guard let source = bundle.url(
            forResource: name,
            withExtension: fileExtension,
            subdirectory: "Tweaks"
        ) else {
            throw CocoaError(.fileNoSuchFile)
        }
        try fileManager.copyItem(at: source, to: destination)
        try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: destination.path)
    }

    private func copyBundledDirectoryContentsIfMissing(
        named name: String,
        to destinationDirectory: URL,
        fileManager: FileManager,
        bundle: Bundle
    ) throws {
        guard let sourceDirectory = bundle.url(
            forResource: name,
            withExtension: nil,
            subdirectory: "Tweaks"
        ) else { return }
        guard let enumerator = fileManager.enumerator(
            at: sourceDirectory,
            includingPropertiesForKeys: [.isDirectoryKey, .isRegularFileKey],
            options: [.skipsHiddenFiles, .skipsPackageDescendants]
        ) else { return }

        for case let sourceURL as URL in enumerator {
            let values = try sourceURL.resourceValues(
                forKeys: [.isDirectoryKey, .isRegularFileKey]
            )
            let relativePath = Self.relativePath(
                for: sourceURL,
                under: sourceDirectory
            )
            let destinationURL = destinationDirectory
                .appendingPathComponent(relativePath)

            if values.isDirectory == true {
                try fileManager.createDirectory(
                    at: destinationURL,
                    withIntermediateDirectories: true,
                    attributes: [.posixPermissions: 0o700]
                )
            } else if values.isRegularFile == true,
                      !fileManager.fileExists(atPath: destinationURL.path)
            {
                try fileManager.createDirectory(
                    at: destinationURL.deletingLastPathComponent(),
                    withIntermediateDirectories: true,
                    attributes: [.posixPermissions: 0o700]
                )
                try fileManager.copyItem(at: sourceURL, to: destinationURL)
                try fileManager.setAttributes(
                    [.posixPermissions: 0o600],
                    ofItemAtPath: destinationURL.path
                )
            }
        }
    }

    private func loadSourceSet(
        entryURL: URL,
        vendorDirectoryURL: URL,
        moduleDirectoryURL: URL,
        fileExtension: String,
        separator: String,
        wrapJavaScriptModules: Bool,
        fileManager: FileManager = .default
    ) throws -> (source: String, fingerprintMaterial: String) {
        let vendorSourceURLs = try sourceURLs(
            in: vendorDirectoryURL,
            fileExtension: fileExtension,
            fileManager: fileManager
        )
        let moduleSourceURLs = try sourceURLs(
            in: moduleDirectoryURL,
            fileExtension: fileExtension,
            fileManager: fileManager
        )
        let orderedSourceURLs = vendorSourceURLs + [entryURL] + moduleSourceURLs

        let sources = try orderedSourceURLs.map { sourceURL in
            let relativePath = sourceURL == entryURL
                ? entryURL.lastPathComponent
                : Self.relativePath(for: sourceURL, under: tweaksDirectoryURL)
            let source = try String(contentsOf: sourceURL, encoding: .utf8)
            return (relativePath: relativePath, source: source)
        }

        return (
            source: sources
                .map { item in
                    guard wrapJavaScriptModules else { return item.source }
                    return """
                    api.runModule(\(Self.javaScriptStringLiteral(item.relativePath)), function (api, root) {
                    \(item.source)
                    });
                    """
                }
                .joined(separator: separator),
            fingerprintMaterial: sources
                .map { $0.relativePath + "\u{0}" + $0.source }
                .joined(separator: "\u{0}")
        )
    }

    private func sourceURLs(
        in directoryURL: URL,
        fileExtension: String,
        fileManager: FileManager
    ) throws -> [URL] {
        guard let enumerator = fileManager.enumerator(
            at: directoryURL,
            includingPropertiesForKeys: [.isRegularFileKey],
            options: [.skipsHiddenFiles, .skipsPackageDescendants]
        ) else { return [] }

        var sourceURLs: [URL] = []
        for case let sourceURL as URL in enumerator {
            let values = try sourceURL.resourceValues(forKeys: [.isRegularFileKey])
            guard values.isRegularFile == true,
                  sourceURL.pathExtension.caseInsensitiveCompare(fileExtension) == .orderedSame
            else { continue }
            sourceURLs.append(sourceURL)
        }

        return sourceURLs.sorted {
            Self.relativePath(for: $0, under: directoryURL)
                < Self.relativePath(for: $1, under: directoryURL)
        }
    }

    private static func relativePath(for url: URL, under directoryURL: URL) -> String {
        let directoryPath = directoryURL.standardizedFileURL.path + "/"
        let path = url.standardizedFileURL.path
        guard path.hasPrefix(directoryPath) else { return url.lastPathComponent }
        return String(path.dropFirst(directoryPath.count))
    }

    private static func javaScriptStringLiteral(_ value: String) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.withoutEscapingSlashes]
        let data = try! encoder.encode(value)
        return String(decoding: data, as: UTF8.self)
    }
}
