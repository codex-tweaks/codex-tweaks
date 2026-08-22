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
    }

    func loadPayload() throws -> TweakPayload {
        try prepareUserTweaks()
        let css = try String(contentsOf: customCSSURL, encoding: .utf8)
        let javascript = try String(contentsOf: customJavaScriptURL, encoding: .utf8)
        let version = Self.fingerprint(css + "\u{0}" + javascript)
        return TweakPayload(
            css: css,
            javascript: javascript,
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
}
