import Foundation
import Sparkle

@MainActor
final class SparkleUpdateController: NSObject, SPUUpdaterDelegate {
    static let shared = SparkleUpdateController()

    private var channel: UpdateChecker.Channel = .stable
    private var hasStarted = false
    private lazy var updaterController = SPUStandardUpdaterController(
        startingUpdater: true,
        updaterDelegate: self,
        userDriverDelegate: nil
    )

    private override init() {
        super.init()
    }

    func start() {
        guard !hasStarted, !Self.isRunningTests else { return }
        hasStarted = true
        _ = updaterController
    }

    func synchronize(channel: UpdateChecker.Channel, automaticallyChecks: Bool) {
        let channelChanged = self.channel != channel
        self.channel = channel

        start()
        guard hasStarted else { return }

        let updater = updaterController.updater
        if updater.automaticallyChecksForUpdates != automaticallyChecks {
            updater.automaticallyChecksForUpdates = automaticallyChecks
        }
        if updater.automaticallyDownloadsUpdates != automaticallyChecks {
            updater.automaticallyDownloadsUpdates = automaticallyChecks
        }
        if channelChanged {
            updater.resetUpdateCycleAfterShortDelay()
        }
    }

    func checkForUpdates() {
        start()
        guard hasStarted, updaterController.updater.canCheckForUpdates else { return }
        updaterController.updater.checkForUpdates()
    }

    func allowedChannels(for updater: SPUUpdater) -> Set<String> {
        channel == .beta ? ["beta"] : []
    }

    private static var isRunningTests: Bool {
        ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil
    }
}
