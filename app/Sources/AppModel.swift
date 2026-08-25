import AppKit
import Foundation
import UniformTypeIdentifiers

@MainActor
final class AppModel: ObservableObject {
    static let shared = AppModel()

    enum Status: Equatable {
        case starting
        case launchingCodex
        case codexNotRunning
        case waitingForCDP
        case restartRequired
        case waitingForPage
        case connected(targetCount: Int)
        case disabled
        case error(String)

        var symbol: String {
            switch self {
            case .connected: return "checkmark.circle.fill"
            case .disabled: return "pause.circle"
            case .codexNotRunning: return "circle"
            case .restartRequired: return "arrow.clockwise.circle"
            case .error: return "exclamationmark.triangle.fill"
            default: return "circle.dotted"
            }
        }

        var isCDPAvailable: Bool {
            switch self {
            case .connected, .waitingForPage: return true
            default: return false
            }
        }

    }

    @Published private(set) var status: Status = .starting
    @Published private(set) var presentation: BackendPresentationContract?
    @Published private(set) var logText = ""
    @Published private(set) var isAuthoringPromptCopied = false
    @Published private(set) var tweakPackages: [TweakPackage] = []
    @Published private(set) var disabledTweakPackageIDs: Set<String> = []
    @Published private(set) var buildingPackageIDs: Set<String> = []
    @Published private(set) var exportingPackageIDs: Set<String> = []
    @Published private(set) var packageBuildErrors: [String: String] = [:]
    @Published private(set) var packageRuntimeErrors: [String: String] = [:]
    @Published private(set) var packagePayloadErrors: [String: String] = [:]
    @Published private(set) var packageDependencyStatuses: [String: [TweakPackageDependencyStatus]] = [:]
    @Published private(set) var packageDependencyIssues: [String: [String]] = [:]
    @Published private(set) var packagePriorityConstraints: [String: TweakPackagePriorityConstraint] = [:]
    @Published private(set) var nodeEnvironment: NodeEnvironment?
    @Published private(set) var isCheckingNode = false
    @Published private(set) var gitEnvironment: GitEnvironment?
    @Published private(set) var isCheckingGit = false
    @Published private(set) var isCheckingRemoteUpdates = false
    @Published private(set) var remotePackageUpdates: [String: TweakPackageRemoteUpdate] = [:]
    @Published private(set) var remotePackageErrors: [String: String] = [:]
    @Published private(set) var installingPackageIDs: Set<String> = []
    @Published private(set) var isInstallingRemotePackage = false
    @Published private(set) var remoteOperationMessage: String?
    @Published private(set) var remoteOperationError: String?
    @Published private(set) var isInstallingLocalPackage = false
    @Published private(set) var localOperationMessage: String?
    @Published private(set) var localOperationError: String?

    @Published var isEnabled = true {
        didSet {
            guard !isApplyingSnapshot, oldValue != isEnabled else { return }
            command("setEnabled", BoolParameter(enabled: isEnabled))
        }
    }

    @Published var isDeveloperMode = false {
        didSet {
            guard !isApplyingSnapshot, oldValue != isDeveloperMode else { return }
            command("setDeveloperMode", BoolParameter(enabled: isDeveloperMode))
        }
    }

    @Published private(set) var isDeveloperAllowUnknownNode = false

    var menuBarSymbol: String {
        switch status {
        case .connected: return "wand.and.stars.inverse"
        case .error, .restartRequired: return "wand.and.stars"
        default: return "sparkles"
        }
    }

    @Published private(set) var tweaksDirectoryPath = ""
    @Published private(set) var packagesDirectoryPath = ""
    @Published private(set) var logFilePath = ""
    @Published private(set) var enabledTweakPackageCount = 0
    @Published private(set) var activeTweakPackageCount = 0

    private let backend = BackendClient.shared
    private var isApplyingSnapshot = false
    private var hasStarted = false
    private var promptCopyResetTask: Task<Void, Never>?

    private init() {}

    func text(_ key: PresentationTextKey, _ replacements: [String: String] = [:]) -> String {
        PresentationText.resolve(key, contract: presentation, replacements: replacements)
    }

    var tokens: BackendPresentationTokens {
        presentation?.tokens ?? GeneratedPresentationDefaults.contract.tokens
    }

    var actions: BackendAvailableActions {
        presentation?.actions ?? GeneratedPresentationDefaults.contract.actions
    }

    var platform: BackendPlatformPresentation {
        presentation?.platform ?? GeneratedPresentationDefaults.contract.platform
    }

    var statusTitle: String {
        if case .error = status {
            return text(.statusErrorTitle)
        }
        let title = presentation?.status.title ?? GeneratedPresentationDefaults.contract.status.title
        return title.isEmpty ? text(.statusStartingTitle) : title
    }

    var statusDetail: String? {
        if case let .error(message) = status {
            return message
        }
        let detail = presentation?.status.detail ?? GeneratedPresentationDefaults.contract.status.detail
        return detail.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? nil : detail
    }

    var statusTone: String {
        if case .error = status {
            return "danger"
        }
        return presentation?.status.tone ?? GeneratedPresentationDefaults.contract.status.tone
    }

    func start() {
        guard !hasStarted else { return }
        hasStarted = true
        backend.stateHandler = { [weak self] snapshot in
            self?.apply(snapshot)
        }
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let snapshot = try await backend.start(params: Self.initializeParams())
                apply(snapshot)
            } catch {
                status = .error(error.localizedDescription)
            }
        }
    }

    func stop() {
        promptCopyResetTask?.cancel()
        promptCopyResetTask = nil
        Task { await backend.stop() }
    }

    func openCodex() { command("openCodex") }

    func confirmAndRestartCodex() {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = text(.dialogRestartTitle)
        alert.informativeText = text(.dialogRestartMessage)
        alert.addButton(withTitle: text(.overviewRestartAndConnect))
        alert.addButton(withTitle: text(.commonCancel))
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        command("restartCodex")
    }

    func confirmAndRestartCodexUI() {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = text(.dialogRestartCodexUITitle)
        alert.informativeText = text(.dialogRestartCodexUIMessage)
        let restartButton = alert.addButton(withTitle: text(.overviewRestartCodexUI))
        let cancelButton = alert.addButton(withTitle: text(.commonCancel))
        restartButton.keyEquivalent = ""
        cancelButton.keyEquivalent = "\r"
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        command("restartCodexUI")
    }

    func reinject() { command("reinject") }

    func isTweakPackageEnabled(_ package: TweakPackage) -> Bool {
        !disabledTweakPackageIDs.contains(package.id)
    }

    func setTweakPackage(_ package: TweakPackage, isEnabled: Bool) {
        if isEnabled,
           package.node?.explicitlyAuthorized == false,
           !isDeveloperAllowUnknownNode
        {
            authorizeNodePackage(package, enableAfterAuthorization: true)
            return
        }
        command(
            "setPackageEnabled",
            PackageEnabledParameter(packageID: package.id, enabled: isEnabled)
        )
    }

    func authorizeNodePackage(
        _ package: TweakPackage,
        enableAfterAuthorization: Bool = false
    ) {
        guard let node = package.node else { return }
        guard !node.authorizationID.isEmpty else {
            let alert = NSAlert()
            alert.alertStyle = .warning
            alert.messageText = package.presentation.statusTitle
            alert.informativeText = package.presentation.statusDetail
            alert.addButton(withTitle: text(.commonConfirm))
            alert.runModal()
            return
        }
        let alert = NSAlert()
        alert.alertStyle = .critical
        alert.messageText = text(.packagesNodeAuthorizationTitle)
        alert.informativeText = [
            text(.packagesNodeAuthorizationWarning),
            text(.packagesNodeAuthorizationReason) + "：\n" + node.reason,
        ].joined(separator: "\n\n")
        let allowButton = alert.addButton(withTitle: text(.packagesNodeAuthorizationAllow))
        let cancelButton = alert.addButton(withTitle: text(.packagesNodeAuthorizationCancel))
        allowButton.keyEquivalent = ""
        cancelButton.keyEquivalent = "\r"
        guard alert.runModal() == .alertFirstButtonReturn else { return }

        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                if enableAfterAuthorization {
                    try await backend.send(
                        method: "setPackageEnabled",
                        params: PackageEnabledParameter(packageID: package.id, enabled: true)
                    )
                }
                try await backend.send(
                    method: "authorizeNodePackage",
                    params: NodeAuthorizationParameter(
                        packageID: package.id,
                        authorizationID: node.authorizationID
                    )
                )
            } catch {
                status = .error(error.localizedDescription)
            }
        }
    }

    func requestDeveloperAllowUnknownNode(_ enabled: Bool) {
        if !enabled {
            command("setDeveloperAllowUnknownNode", BoolParameter(enabled: false))
            return
        }
        let alert = NSAlert()
        alert.alertStyle = .critical
        alert.messageText = text(.packagesNodeAutomaticWarningTitle)
        alert.informativeText = text(.packagesNodeAutomaticWarning)
        let allowButton = alert.addButton(withTitle: text(.packagesNodeAutomaticWarningAllow))
        let cancelButton = alert.addButton(withTitle: text(.commonCancel))
        allowButton.keyEquivalent = ""
        cancelButton.keyEquivalent = "\r"
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        command("setDeveloperAllowUnknownNode", BoolParameter(enabled: true))
    }

    func setTweakPackagePriority(_ package: TweakPackage, priority: Int) {
        command(
            "setPackagePriority",
            PackagePriorityParameter(packageID: package.id, priority: priority)
        )
    }

    func resetTweakPackagePriority(_ package: TweakPackage) {
        command(
            "setPackagePriority",
            PackagePriorityParameter(packageID: package.id, priority: nil)
        )
    }

    func dependencyIssues(for package: TweakPackage) -> [String] {
        packageDependencyIssues[package.id] ?? []
    }

    func dependencyStatuses(for package: TweakPackage) -> [TweakPackageDependencyStatus] {
        packageDependencyStatuses[package.id] ?? []
    }

    func priorityConstraint(for package: TweakPackage) -> TweakPackagePriorityConstraint? {
        packagePriorityConstraints[package.id]
    }

    func canInstallMissingDependencies(for package: TweakPackage) -> Bool {
        package.canInstallMissingDependencies
    }

    func canEnableDependencies(for package: TweakPackage) -> Bool {
        package.canEnableDependencies
    }

    func enableDependencies(for package: TweakPackage) {
        command("enableDependencies", PackageIDParameter(packageID: package.id))
    }

    func reloadTweakPackages() { command("reloadPackages") }
    func checkNodeEnvironment() { command("checkNodeEnvironment") }
    func checkGitEnvironment() { command("checkGitEnvironment") }

    func installRemotePackage(
        repositoryURL: String,
        selectorType: TweakPackageRemoteSelectorType,
        selectorValue: String
    ) {
        command(
            "installRemotePackage",
            RemoteInstallParameter(
                repositoryURL: repositoryURL,
                selectorType: selectorType,
                selectorValue: selectorValue
            )
        )
    }

    func clearRemoteOperationFeedback() { command("clearRemoteOperationFeedback") }

    func installLocalPackage(from sourceURL: URL) {
        command("installLocalPackage", LocalInstallParameter(sourcePath: sourceURL.path))
    }

    func clearLocalOperationFeedback() { command("clearLocalOperationFeedback") }

    func reportLocalPackageSelectionError(_ error: Error) {
        command(
            "reportLocalPackageSelectionError",
            MessageParameter(message: error.localizedDescription)
        )
    }

    func installMissingDependencies(for package: TweakPackage) {
        command("installMissingDependencies", PackageIDParameter(packageID: package.id))
    }

    func checkManagedPackageUpdates(automatic: Bool = false) {
        command(
            "checkManagedPackageUpdates",
            AutomaticParameter(automatic: automatic)
        )
    }

    func updateManagedPackage(_ package: TweakPackage) {
        command("updateManagedPackage", PackageIDParameter(packageID: package.id))
    }

    func buildPackage(_ package: TweakPackage) {
        command("buildPackage", PackageIDParameter(packageID: package.id))
    }

    func exportPackage(_ package: TweakPackage) {
        guard package.availableActions.export else { return }
        let panel = NSSavePanel()
        panel.title = text(.packagesExportZip)
        panel.message = text(.packagesExportZipHelp)
        panel.prompt = text(.packagesExportZip)
        panel.allowedContentTypes = [.zip]
        panel.canCreateDirectories = true
        panel.isExtensionHidden = false
        panel.nameFieldStringValue = package.exportFileName
        guard panel.runModal() == .OK, let destinationURL = panel.url else { return }
        command(
            "exportPackage",
            PackageExportParameter(
                packageID: package.id,
                destinationPath: destinationURL.path
            )
        )
    }

    func confirmDeletePackage(_ package: TweakPackage) {
        guard package.availableActions.delete else { return }
        let alert = NSAlert()
        alert.alertStyle = .critical
        alert.messageText = text(.packagesDeleteTitle, ["name": package.displayName])
        alert.informativeText = text(.packagesDeleteMessage)
        let deleteButton = alert.addButton(withTitle: text(.packagesDeleteConfirm))
        let cancelButton = alert.addButton(withTitle: text(.commonCancel))
        deleteButton.hasDestructiveAction = true
        deleteButton.keyEquivalent = ""
        cancelButton.keyEquivalent = "\r"
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        command("deletePackage", PackageIDParameter(packageID: package.id))
    }

    func openPackageDirectory(_ package: TweakPackage) {
        NSWorkspace.shared.open(package.directoryURL)
    }

    func openTweaksDirectory() {
        guard !packagesDirectoryPath.isEmpty else { return }
        NSWorkspace.shared.open(URL(fileURLWithPath: packagesDirectoryPath, isDirectory: true))
    }

    func refreshLog() { command("refreshLog") }

    @discardableResult
    func clearLog() -> String? {
        command("clearLog")
        return nil
    }

    func quit() {
        Task { @MainActor in
            await backend.stop()
            NSApplication.shared.terminate(nil)
        }
    }

    func copyAuthoringPrompt() {
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let contents: String = try await backend.request(
                    method: "readAuthoringPrompt",
                    params: NoParameter()
                )
                let pasteboard = NSPasteboard.general
                pasteboard.clearContents()
                guard pasteboard.setString(contents, forType: .string) else { return }
                promptCopyResetTask?.cancel()
                isAuthoringPromptCopied = true
                promptCopyResetTask = Task { @MainActor [weak self] in
                    try? await Task.sleep(nanoseconds: 2_000_000_000)
                    guard !Task.isCancelled else { return }
                    self?.isAuthoringPromptCopied = false
                }
            } catch {
                status = .error(error.localizedDescription)
            }
        }
    }

    func openLog() {
        guard !logFilePath.isEmpty else { return }
        NSWorkspace.shared.open(URL(fileURLWithPath: logFilePath))
    }

    func sendUpdateCommand<Params: Encodable & Sendable>(
        _ method: String,
        params: Params
    ) {
        command(method, params)
    }

    func sendUpdateCommand(_ method: String) { command(method) }

    private func apply(_ snapshot: BackendAppSnapshot) {
        guard snapshot.protocolVersion == 8 else {
            status = .error(text(.appProtocolMismatch))
            return
        }
        presentation = snapshot.presentation
        isApplyingSnapshot = true
        isEnabled = snapshot.enabled
        isDeveloperMode = snapshot.developerMode
        isDeveloperAllowUnknownNode = snapshot.developerAllowUnknownNode
        isApplyingSnapshot = false

        status = Status(snapshot.status)
        if logText != snapshot.logText {
            logText = snapshot.logText
        }
        tweakPackages = snapshot.packages
        disabledTweakPackageIDs = Set(snapshot.disabledPackageIDs)
        buildingPackageIDs = Set(snapshot.buildingPackageIDs)
        exportingPackageIDs = Set(snapshot.exportingPackageIDs)
        packageBuildErrors = snapshot.packageBuildErrors
        packageRuntimeErrors = snapshot.packageRuntimeErrors
        packagePayloadErrors = snapshot.packagePayloadErrors
        packageDependencyStatuses = snapshot.packageDependencyStatuses
        packageDependencyIssues = snapshot.packageDependencyIssues
        packagePriorityConstraints = snapshot.packagePriorityConstraints
        nodeEnvironment = snapshot.nodeEnvironment
        isCheckingNode = snapshot.checkingNode
        gitEnvironment = snapshot.gitEnvironment
        isCheckingGit = snapshot.checkingGit
        isCheckingRemoteUpdates = snapshot.checkingRemoteUpdates
        remotePackageUpdates = snapshot.remotePackageUpdates
        remotePackageErrors = snapshot.remotePackageErrors
        installingPackageIDs = Set(snapshot.installingPackageIDs)
        isInstallingRemotePackage = snapshot.installingRemotePackage
        remoteOperationMessage = snapshot.remoteOperationMessage
        remoteOperationError = snapshot.remoteOperationError
        isInstallingLocalPackage = snapshot.installingLocalPackage
        localOperationMessage = snapshot.localOperationMessage
        localOperationError = snapshot.localOperationError
        tweaksDirectoryPath = snapshot.tweaksDirectory
        packagesDirectoryPath = snapshot.packagesDirectory
        logFilePath = snapshot.logPath
        enabledTweakPackageCount = snapshot.enabledPackageCount
        activeTweakPackageCount = snapshot.activePackageCount
        UpdateChecker.shared.apply(snapshot.update)
    }

    private func command(_ method: String) {
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await backend.send(method: method)
            } catch {
                status = .error(error.localizedDescription)
            }
        }
    }

    private func command<Params: Encodable & Sendable>(_ method: String, _ params: Params) {
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await backend.send(method: method, params: params)
            } catch {
                status = .error(error.localizedDescription)
            }
        }
    }

    private static func initializeParams() -> BackendInitializeParams {
        let environment = ProcessInfo.processInfo.environment
        let currentVersion = Bundle.main.object(
            forInfoDictionaryKey: "CodexTweaksReleaseVersion"
        ) as? String ?? Bundle.main.object(
            forInfoDictionaryKey: "CFBundleShortVersionString"
        ) as? String ?? "-"
        let buildNumber = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "-"
        let packagePath = Bundle.main.url(
            forResource: "packages",
            withExtension: nil,
            subdirectory: "Tweaks"
        )?.path
        let skillPath = Bundle.main.url(
            forResource: "SKILL",
            withExtension: "md",
            subdirectory: "Skills/develop-codex-tweaks-package"
        )?.path

        return BackendInitializeParams(
            applicationSupportDirectory: environment["CODEX_TWEAKS_APPLICATION_SUPPORT"],
            cacheDirectory: environment["CODEX_TWEAKS_CACHE_DIRECTORY"],
            bundledPackagesDirectory: packagePath,
            skillPath: skillPath,
            currentVersion: currentVersion,
            buildNumber: buildNumber
        )
    }
}

private extension AppModel.Status {
    init(_ status: BackendAppStatus) {
        switch status.kind {
        case .starting: self = .starting
        case .launchingCodex: self = .launchingCodex
        case .codexNotRunning: self = .codexNotRunning
        case .waitingForCDP: self = .waitingForCDP
        case .restartRequired: self = .restartRequired
        case .waitingForPage: self = .waitingForPage
        case .connected: self = .connected(targetCount: status.targetCount ?? 0)
        case .disabled: self = .disabled
        case .error: self = .error(status.message ?? PresentationText.resolve(.statusErrorTitle))
        }
    }
}

private struct NoParameter: Encodable, Sendable {}
private struct BoolParameter: Encodable, Sendable { let enabled: Bool }
private struct PackageIDParameter: Encodable, Sendable { let packageID: String }
private struct PackageEnabledParameter: Encodable, Sendable {
    let packageID: String
    let enabled: Bool
}
private struct NodeAuthorizationParameter: Encodable, Sendable {
    let packageID: String
    let authorizationID: String
}
private struct PackagePriorityParameter: Encodable, Sendable {
    let packageID: String
    let priority: Int?
}
private struct RemoteInstallParameter: Encodable, Sendable {
    let repositoryURL: String
    let selectorType: TweakPackageRemoteSelectorType
    let selectorValue: String
}
private struct LocalInstallParameter: Encodable, Sendable { let sourcePath: String }
private struct PackageExportParameter: Encodable, Sendable {
    let packageID: String
    let destinationPath: String
}
private struct MessageParameter: Encodable, Sendable { let message: String }
private struct AutomaticParameter: Encodable, Sendable { let automatic: Bool }
