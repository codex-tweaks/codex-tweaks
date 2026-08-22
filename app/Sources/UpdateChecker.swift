import Combine
import Darwin
import Foundation

struct GitHubRelease: Decodable, Identifiable {
    let tagName: String
    let draft: Bool
    let prerelease: Bool
    let publishedAt: Date?
    let htmlURL: URL?
    let assets: [GitHubAsset]

    var id: String { tagName }

    enum CodingKeys: String, CodingKey {
        case tagName = "tag_name"
        case draft
        case prerelease
        case publishedAt = "published_at"
        case htmlURL = "html_url"
        case assets
    }
}

struct GitHubAsset: Decodable {
    let name: String
    let browserDownloadURL: URL?

    enum CodingKeys: String, CodingKey {
        case name
        case browserDownloadURL = "browser_download_url"
    }
}

/// Semantic Versioning 2.0.0 precedence. Build metadata is validated but does
/// not participate in ordering.
struct SemanticVersion: Comparable {
    private enum PrereleaseIdentifier: Equatable {
        case numeric(String)
        case text(String)
    }

    private let major: String
    private let minor: String
    private let patch: String
    private let prerelease: [PrereleaseIdentifier]?

    init?(_ rawValue: String) {
        let normalized = Self.normalizedString(rawValue)
        let buildParts = normalized.split(
            separator: "+",
            maxSplits: 1,
            omittingEmptySubsequences: false
        )
        guard let precedence = buildParts.first, !precedence.isEmpty else { return nil }
        if buildParts.count == 2 {
            guard Self.validDotSeparatedIdentifiers(buildParts[1]) else { return nil }
        }

        let precedenceParts = precedence.split(
            separator: "-",
            maxSplits: 1,
            omittingEmptySubsequences: false
        )
        let core = precedenceParts[0].split(separator: ".", omittingEmptySubsequences: false)
        guard core.count == 3,
              Self.isValidNumericIdentifier(core[0]),
              Self.isValidNumericIdentifier(core[1]),
              Self.isValidNumericIdentifier(core[2]) else {
            return nil
        }

        major = String(core[0])
        minor = String(core[1])
        patch = String(core[2])

        if precedenceParts.count == 2 {
            let rawIdentifiers = precedenceParts[1].split(
                separator: ".",
                omittingEmptySubsequences: false
            )
            guard !rawIdentifiers.isEmpty else { return nil }

            var identifiers: [PrereleaseIdentifier] = []
            identifiers.reserveCapacity(rawIdentifiers.count)
            for identifier in rawIdentifiers {
                guard !identifier.isEmpty, Self.validIdentifier(identifier) else { return nil }
                if Self.isASCIIInteger(identifier) {
                    guard Self.isValidNumericIdentifier(identifier) else { return nil }
                    identifiers.append(.numeric(String(identifier)))
                } else {
                    identifiers.append(.text(String(identifier)))
                }
            }
            prerelease = identifiers
        } else {
            prerelease = nil
        }
    }

    static func normalizedString(_ rawValue: String) -> String {
        var value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        if value.first == "v" || value.first == "V" {
            value.removeFirst()
        }
        return value
    }

    var isStable: Bool {
        prerelease == nil
    }

    var isBetaOrReleaseCandidate: Bool {
        guard let firstIdentifier = prerelease?.first else { return false }
        guard case let .text(value) = firstIdentifier else { return false }
        return value.lowercased() == "beta" || value.lowercased() == "rc"
    }

    static func < (lhs: SemanticVersion, rhs: SemanticVersion) -> Bool {
        let leftCore = [lhs.major, lhs.minor, lhs.patch]
        let rightCore = [rhs.major, rhs.minor, rhs.patch]
        for (left, right) in zip(leftCore, rightCore) where left != right {
            return compareNumeric(left, right) < 0
        }

        switch (lhs.prerelease, rhs.prerelease) {
        case (nil, nil):
            return false
        case (nil, _):
            return false
        case (_, nil):
            return true
        case let (leftIdentifiers?, rightIdentifiers?):
            for index in 0..<min(leftIdentifiers.count, rightIdentifiers.count) {
                let left = leftIdentifiers[index]
                let right = rightIdentifiers[index]
                if left == right { continue }

                switch (left, right) {
                case let (.numeric(leftValue), .numeric(rightValue)):
                    return compareNumeric(leftValue, rightValue) < 0
                case (.numeric, .text):
                    return true
                case (.text, .numeric):
                    return false
                case let (.text(leftValue), .text(rightValue)):
                    return leftValue < rightValue
                }
            }
            return leftIdentifiers.count < rightIdentifiers.count
        }
    }

    private static func compareNumeric(_ lhs: String, _ rhs: String) -> Int {
        if lhs.count != rhs.count {
            return lhs.count < rhs.count ? -1 : 1
        }
        if lhs == rhs { return 0 }
        return lhs < rhs ? -1 : 1
    }

    private static func isValidNumericIdentifier(_ value: Substring) -> Bool {
        isASCIIInteger(value) && !(value.count > 1 && value.first == "0")
    }

    private static func isASCIIInteger(_ value: Substring) -> Bool {
        !value.isEmpty && value.unicodeScalars.allSatisfy { (48...57).contains($0.value) }
    }

    private static func validIdentifier(_ value: Substring) -> Bool {
        !value.isEmpty && value.unicodeScalars.allSatisfy {
            (48...57).contains($0.value)
                || (65...90).contains($0.value)
                || (97...122).contains($0.value)
                || $0.value == 45
        }
    }

    private static func validDotSeparatedIdentifiers(_ value: Substring) -> Bool {
        let identifiers = value.split(separator: ".", omittingEmptySubsequences: false)
        return !identifiers.isEmpty && identifiers.allSatisfy(validIdentifier)
    }
}

private enum UpdateCheckError: LocalizedError {
    case invalidRepositoryURL
    case invalidResponse
    case httpStatus(Int)

    var errorDescription: String? {
        switch self {
        case .invalidRepositoryURL:
            return "更新地址无效。"
        case .invalidResponse:
            return "GitHub 返回了无法识别的响应。"
        case .httpStatus(403):
            return "GitHub 暂时限制了更新请求，请稍后再试。"
        case .httpStatus(404):
            return "没有找到 Codex Tweaks 的 Release 仓库。"
        case let .httpStatus(statusCode):
            return "检查更新失败（HTTP \(statusCode)）。"
        }
    }
}

@MainActor
final class UpdateChecker: ObservableObject {
    static let shared = UpdateChecker()

    enum Channel: String, CaseIterable, Identifiable {
        case stable = "正式版"
        case beta = "测试版"

        var id: String { rawValue }

        var detail: String {
            switch self {
            case .stable:
                return "仅检查正式发布版本"
            case .beta:
                return "检查正式版、Beta 与 RC"
            }
        }
    }

    static let repository = "cr-zhichen/codex-tweaks"
    static let repositoryURL = URL(string: "https://github.com/\(repository)")!

    @Published var channel: Channel {
        didSet {
            defaults.set(channel.rawValue, forKey: Self.channelKey)
            guard channel != oldValue else { return }
            latestRelease = nil
            pendingUpdate = nil
            lastError = nil
        }
    }

    @Published var autoCheck: Bool {
        didSet {
            defaults.set(autoCheck, forKey: Self.autoCheckKey)
        }
    }

    @Published private(set) var checking = false
    @Published private(set) var latestRelease: GitHubRelease?
    @Published private(set) var lastError: String?
    @Published private(set) var lastCheckDate: Date?
    @Published var pendingUpdate: GitHubRelease?

    private static let releaseVersionInfoKey = "CodexTweaksReleaseVersion"
    private static let channelKey = "update.channel"
    private static let autoCheckKey = "update.autoCheck"
    private static let lastCheckKey = "update.lastCheckDate"
    private static let skippedKey = "update.skippedVersions"

    private let defaults: UserDefaults
    private let session: URLSession
    private let bundle: Bundle
    private let installedVersionOverride: String?

    private var skippedVersions: Set<String> {
        didSet {
            defaults.set(Array(skippedVersions).sorted(), forKey: Self.skippedKey)
        }
    }

    init(
        defaults: UserDefaults = .standard,
        session: URLSession = .shared,
        bundle: Bundle = .main,
        installedVersion: String? = nil
    ) {
        self.defaults = defaults
        self.session = session
        self.bundle = bundle
        installedVersionOverride = installedVersion

        channel = Channel(rawValue: defaults.string(forKey: Self.channelKey) ?? "") ?? .stable
        if defaults.object(forKey: Self.autoCheckKey) == nil {
            defaults.set(true, forKey: Self.autoCheckKey)
        }
        autoCheck = defaults.bool(forKey: Self.autoCheckKey)
        lastCheckDate = defaults.object(forKey: Self.lastCheckKey) as? Date
        skippedVersions = Set(defaults.stringArray(forKey: Self.skippedKey) ?? [])
    }

    var currentVersion: String {
        if let installedVersionOverride, SemanticVersion(installedVersionOverride) != nil {
            return SemanticVersion.normalizedString(installedVersionOverride)
        }

        let releaseVersion = bundle.object(
            forInfoDictionaryKey: Self.releaseVersionInfoKey
        ) as? String
        let marketingVersion = bundle.object(
            forInfoDictionaryKey: "CFBundleShortVersionString"
        ) as? String

        for candidate in [releaseVersion, marketingVersion].compactMap({ $0 }) {
            if SemanticVersion(candidate) != nil {
                return SemanticVersion.normalizedString(candidate)
            }
        }
        return marketingVersion ?? "-"
    }

    var buildNumber: String {
        bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "-"
    }

    var hasNewerVersion: Bool {
        guard let latestRelease,
              let latestVersion = SemanticVersion(latestRelease.tagName),
              let installedVersion = SemanticVersion(currentVersion) else {
            return false
        }
        return latestVersion > installedVersion
    }

    var updateAvailable: Bool {
        guard hasNewerVersion, let latestRelease else { return false }
        return !isSkipped(latestRelease)
    }

    var latestVersionString: String {
        guard let latestRelease else { return "-" }
        return SemanticVersion.normalizedString(latestRelease.tagName)
    }

    var latestVersionIsSkipped: Bool {
        guard let latestRelease else { return false }
        return isSkipped(latestRelease)
    }

    var downloadURL: URL? {
        guard let latestRelease else { return nil }
        return downloadURL(for: latestRelease)
    }

    func downloadURL(for release: GitHubRelease) -> URL? {
        Self.preferredDownloadURL(for: release, architecture: Self.machineArchitecture)
    }

    func autoCheckIfNeeded() async {
        guard autoCheck else { return }
        await check(prompt: true)
    }

    func check(prompt: Bool = false) async {
        guard !checking else { return }
        checking = true
        lastError = nil
        defer { checking = false }

        do {
            let request = try makeRequest()
            let (data, response) = try await session.data(for: request)
            guard let httpResponse = response as? HTTPURLResponse else {
                throw UpdateCheckError.invalidResponse
            }
            guard httpResponse.statusCode == 200 else {
                throw UpdateCheckError.httpStatus(httpResponse.statusCode)
            }

            let releases = try Self.makeDecoder().decode([GitHubRelease].self, from: data)
            let matched = Self.selectLatestRelease(from: releases, for: channel)
            latestRelease = matched

            let checkedAt = Date()
            lastCheckDate = checkedAt
            defaults.set(checkedAt, forKey: Self.lastCheckKey)

            if updateAvailable, prompt {
                pendingUpdate = matched
            } else if pendingUpdate?.tagName != matched?.tagName || !updateAvailable {
                pendingUpdate = nil
            }
        } catch {
            if let urlError = error as? URLError, urlError.code == .notConnectedToInternet {
                lastError = "当前无法连接网络。"
            } else {
                lastError = error.localizedDescription
            }
        }
    }

    func dismissUpdate() {
        pendingUpdate = nil
    }

    func skipUpdate(_ release: GitHubRelease) {
        skippedVersions.insert(Self.versionString(for: release))
        pendingUpdate = nil
    }

    func unskipAndPrompt() {
        guard let latestRelease, hasNewerVersion else { return }
        skippedVersions.remove(Self.versionString(for: latestRelease))
        pendingUpdate = latestRelease
    }

    nonisolated static func selectLatestRelease(
        from releases: [GitHubRelease],
        for channel: Channel
    ) -> GitHubRelease? {
        releases
            .compactMap { release -> (release: GitHubRelease, version: SemanticVersion)? in
                guard !release.draft, let version = SemanticVersion(release.tagName) else {
                    return nil
                }
                return (release, version)
            }
            .filter { candidate in
                switch channel {
                case .stable:
                    return !candidate.release.prerelease && candidate.version.isStable
                case .beta:
                    if candidate.release.prerelease {
                        return candidate.version.isBetaOrReleaseCandidate
                    }
                    return candidate.version.isStable
                }
            }
            .max { $0.version < $1.version }?
            .release
    }

    nonisolated static func preferredDownloadURL(
        for release: GitHubRelease,
        architecture: String
    ) -> URL? {
        let dmgAssets = release.assets.filter { $0.name.lowercased().hasSuffix(".dmg") }
        let architectureSuffix = architecture == "arm64" ? "-arm64.dmg" : "-x86_64.dmg"

        if let architectureAsset = dmgAssets.first(where: {
            $0.name.lowercased().hasSuffix(architectureSuffix)
        }) {
            return architectureAsset.browserDownloadURL ?? release.htmlURL
        }

        if let universalAsset = dmgAssets.first(where: {
            let name = $0.name.lowercased()
            return !name.hasSuffix("-arm64.dmg") && !name.hasSuffix("-x86_64.dmg")
        }) {
            return universalAsset.browserDownloadURL ?? release.htmlURL
        }

        return dmgAssets.first?.browserDownloadURL ?? release.htmlURL
    }

    private func isSkipped(_ release: GitHubRelease) -> Bool {
        skippedVersions.contains(Self.versionString(for: release))
    }

    nonisolated private static func versionString(for release: GitHubRelease) -> String {
        SemanticVersion.normalizedString(release.tagName)
    }

    private func makeRequest() throws -> URLRequest {
        guard var url = URL(
            string: "https://api.github.com/repos/\(Self.repository)/releases"
        ) else {
            throw UpdateCheckError.invalidRepositoryURL
        }
        url.append(queryItems: [URLQueryItem(name: "per_page", value: "100")])

        var request = URLRequest(url: url)
        request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        request.setValue("2026-03-10", forHTTPHeaderField: "X-GitHub-Api-Version")
        request.setValue("Codex-Tweaks/\(currentVersion)", forHTTPHeaderField: "User-Agent")
        return request
    }

    nonisolated private static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let value = try container.decode(String.self)
            let formatter = ISO8601DateFormatter()
            if let date = formatter.date(from: value) {
                return date
            }
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = formatter.date(from: value) {
                return date
            }
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "无法解析日期：\(value)"
            )
        }
        return decoder
    }

    nonisolated private static var machineArchitecture: String {
        var info = utsname()
        uname(&info)
        return withUnsafeBytes(of: &info.machine) { rawBuffer in
            String(cString: rawBuffer.bindMemory(to: CChar.self).baseAddress!)
        }
    }
}
