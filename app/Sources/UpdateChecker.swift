import Foundation

@MainActor
final class UpdateChecker: ObservableObject {
    static let shared = UpdateChecker()

    enum Channel: String, CaseIterable, Identifiable {
        case stable
        case beta

        var id: String { rawValue }

        var titleKey: PresentationTextKey {
            switch self {
            case .stable: return .updateChannelStable
            case .beta: return .updateChannelBeta
            }
        }

        var detailKey: PresentationTextKey {
            switch self {
            case .stable: return .updateChannelStableDetail
            case .beta: return .updateChannelBetaDetail
            }
        }

        var backendValue: BackendUpdateChannel {
            self == .beta ? .beta : .stable
        }
    }

    @Published var channel: Channel = .stable {
        didSet {
            guard !isApplyingSnapshot, oldValue != channel else { return }
            AppModel.shared.sendUpdateCommand(
                "setUpdateChannel",
                params: UpdateChannelParameter(channel: channel.backendValue)
            )
        }
    }

    @Published var autoCheck = true {
        didSet {
            guard !isApplyingSnapshot, oldValue != autoCheck else { return }
            AppModel.shared.sendUpdateCommand(
                "setUpdateAutoCheck",
                params: UpdateAutoCheckParameter(enabled: autoCheck)
            )
        }
    }

    @Published private(set) var checking = false
    @Published private(set) var latestRelease: GitHubRelease?
    @Published private(set) var lastError: String?
    @Published private(set) var lastCheckDate: Date?
    @Published private(set) var pendingUpdate: GitHubRelease?
    @Published private(set) var currentVersion = "-"
    @Published private(set) var buildNumber = "-"
    @Published private(set) var hasNewerVersion = false
    @Published private(set) var updateAvailable = false
    @Published private(set) var latestVersionString = "-"
    @Published private(set) var latestVersionIsSkipped = false
    @Published private(set) var downloadURL: URL?

    private var isApplyingSnapshot = false

    private init() {}

    func check(prompt: Bool = false) async {
        AppModel.shared.sendUpdateCommand(
            "checkAppUpdate",
            params: UpdateCheckParameter(prompt: prompt)
        )
    }

    func dismissUpdate() {
        AppModel.shared.sendUpdateCommand("dismissUpdate")
    }

    func skipUpdate(_ release: GitHubRelease) {
        AppModel.shared.sendUpdateCommand(
            "skipUpdate",
            params: SkipUpdateParameter(tagName: release.tagName)
        )
    }

    func unskipAndPrompt() {
        AppModel.shared.sendUpdateCommand("unskipAndPromptUpdate")
    }

    func downloadURL(for _: GitHubRelease) -> URL? { downloadURL }

    func apply(_ snapshot: BackendUpdateSnapshot) {
        isApplyingSnapshot = true
        channel = snapshot.channel == .beta ? .beta : .stable
        autoCheck = snapshot.autoCheck
        isApplyingSnapshot = false
        checking = snapshot.checking
        latestRelease = snapshot.latestRelease
        lastError = snapshot.lastError
        lastCheckDate = snapshot.lastCheckAt
        pendingUpdate = snapshot.pendingUpdate
        currentVersion = snapshot.currentVersion
        buildNumber = snapshot.buildNumber
        hasNewerVersion = snapshot.hasNewerVersion
        updateAvailable = snapshot.updateAvailable
        latestVersionString = snapshot.latestVersionString
        latestVersionIsSkipped = snapshot.latestVersionIsSkipped
        downloadURL = snapshot.downloadURL
    }
}

private struct UpdateChannelParameter: Encodable, Sendable {
    let channel: BackendUpdateChannel
}

private struct UpdateAutoCheckParameter: Encodable, Sendable {
    let enabled: Bool
}

private struct UpdateCheckParameter: Encodable, Sendable {
    let prompt: Bool
}

private struct SkipUpdateParameter: Encodable, Sendable {
    let tagName: String
}
