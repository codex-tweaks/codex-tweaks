import Foundation
import Sparkle

@MainActor
private final class ConfirmedSparkleUserDriver: SPUStandardUserDriver {
    // Sparkle exposes this callback through its Objective-C protocol instead of
    // the concrete class header, so the exact selector is declared explicitly.
    @objc(showReadyToInstallAndRelaunch:)
    func showReadyToInstallAndRelaunch(
        _ reply: @escaping (SPUUserUpdateChoice) -> Void
    ) {
        reply(.install)
    }
}

@MainActor
final class SparkleUpdateController: NSObject, SPUUpdaterDelegate {
    static let shared = SparkleUpdateController()

    private var channel: UpdateChecker.Channel = .stable
    private var hasStarted = false
    private lazy var userDriver = ConfirmedSparkleUserDriver(
        hostBundle: .main,
        delegate: nil
    )
    private lazy var updater = SPUUpdater(
        hostBundle: .main,
        applicationBundle: .main,
        userDriver: userDriver,
        delegate: self
    )

    private override init() {
        super.init()
    }

    func start() {
        guard !hasStarted, !Self.isRunningTests else { return }
        do {
            try updater.start()
            hasStarted = true
        } catch {
            NSLog("Sparkle updater failed to start: %@", error.localizedDescription)
        }
    }

    func synchronize(channel: UpdateChecker.Channel, automaticallyChecks: Bool) {
        let channelChanged = self.channel != channel
        self.channel = channel

        start()
        guard hasStarted else { return }

        if updater.automaticallyChecksForUpdates != automaticallyChecks {
            updater.automaticallyChecksForUpdates = automaticallyChecks
        }
        if updater.automaticallyDownloadsUpdates {
            updater.automaticallyDownloadsUpdates = false
        }
        if channelChanged {
            updater.resetUpdateCycleAfterShortDelay()
        }
    }

    func checkForUpdates() {
        start()
        guard hasStarted, updater.canCheckForUpdates else { return }
        updater.checkForUpdates()
    }

    func allowedChannels(for updater: SPUUpdater) -> Set<String> {
        channel == .beta ? ["beta"] : []
    }

    private static var isRunningTests: Bool {
        ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil
    }
}
