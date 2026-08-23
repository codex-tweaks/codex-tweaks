import AppKit
import Foundation

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

        var title: String {
            switch self {
            case .starting: return "正在启动"
            case .launchingCodex: return "正在启动 Codex"
            case .codexNotRunning: return "Codex 未运行"
            case .waitingForCDP: return "正在等待调试端口"
            case .restartRequired: return "Codex 需要重启"
            case .waitingForPage: return "正在等待 Codex 页面"
            case let .connected(targetCount):
                return targetCount == 1 ? "已连接 Codex" : "已连接 \(targetCount) 个窗口"
            case .disabled: return "界面增强已停用"
            case .error: return "连接异常"
            }
        }

        var detail: String? {
            switch self {
            case .restartRequired:
                return "当前 Codex 未开启本地 CDP 端口"
            case .waitingForPage:
                return "调试端口可用，尚未发现 app:// 页面"
            case .codexNotRunning:
                return "可从菜单重新打开 Codex"
            case let .error(message):
                return message
            default:
                return nil
            }
        }

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

        var canRestartCodex: Bool {
            if case .restartRequired = self { return true }
            return false
        }
    }

    @Published private(set) var status: Status = .starting
    @Published private(set) var logText = ""
    @Published private(set) var isAuthoringPromptCopied = false
    @Published private(set) var tweakPackages: [TweakPackage] = []
    @Published private(set) var disabledTweakPackageIDs: Set<String> = []
    @Published private(set) var buildingPackageIDs: Set<String> = []
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

    @Published var isEnabled: Bool {
        didSet {
            guard oldValue != isEnabled else { return }
            UserDefaults.standard.set(isEnabled, forKey: Self.enabledDefaultsKey)
            AppLogger.shared.info(isEnabled ? "界面增强已启用" : "界面增强已停用")
            Task { @MainActor [weak self] in
                await self?.refresh(force: true)
            }
        }
    }

    @Published var isDeveloperMode: Bool {
        didSet {
            guard oldValue != isDeveloperMode else { return }
            UserDefaults.standard.set(isDeveloperMode, forKey: Self.developerModeDefaultsKey)
            developerBuildAttemptKeys.removeAll()
            AppLogger.shared.info(isDeveloperMode ? "开发者模式已启用" : "开发者模式已停用")
            if isDeveloperMode { scheduleDeveloperBuilds() }
        }
    }

    var menuBarSymbol: String {
        switch status {
        case .connected: return "wand.and.stars.inverse"
        case .error, .restartRequired: return "wand.and.stars"
        default: return "sparkles"
        }
    }

    var tweaksDirectoryPath: String { packageStore.tweaksDirectoryURL.path }
    var packagesDirectoryPath: String { packageStore.packagesDirectoryURL.path }
    var logFilePath: String { AppLogger.shared.logFileURL.path }

    var enabledTweakPackageCount: Int {
        tweakPackages.filter(isTweakPackageEnabled).count
    }

    var activeTweakPackageCount: Int {
        TweakPackageDependencyResolver.resolve(
            packages: tweakPackages,
            disabledPackageIDs: disabledTweakPackageIDs
        ).loadablePackages.count
    }

    private static let enabledDefaultsKey = "tweaksEnabled"
    private static let disabledPackagesDefaultsKey = "disabledTweakPackageIDs"
    private static let knownPackagesDefaultsKey = "knownTweakPackageIDs"
    private static let developerModeDefaultsKey = "developerMode"
    private let launcher = CodexLauncher()
    private let packageStore = TweakPackageStore()
    private lazy var packageBuilder = TweakPackageBuilder(store: packageStore)
    private lazy var remotePackageManager = TweakPackageRemoteManager(store: packageStore)
    private lazy var localPackageInstaller = TweakPackageLocalInstaller(store: packageStore)
    private let cdpService = CDPService()
    private var monitorTask: Task<Void, Never>?
    private var promptCopyResetTask: Task<Void, Never>?
    private var isRefreshing = false
    private var disabledCleanupCompleted = false
    private var hasAttemptedInitialLaunch = false
    private var forceGeneration = 0
    private var developerBuildAttemptKeys: [String: String] = [:]
    private var packageBuildErrorRequestKeys: [String: String] = [:]
    private var knownTweakPackageIDs: Set<String>?
    private var lastAutomaticRemoteCheckAt: Date?
    private static let automaticRemoteCheckInterval: TimeInterval = 6 * 60 * 60

    private init() {
        let defaults = UserDefaults.standard
        disabledTweakPackageIDs = Set(
            defaults.stringArray(forKey: Self.disabledPackagesDefaultsKey) ?? []
        )
        if let knownPackageIDs = defaults.stringArray(forKey: Self.knownPackagesDefaultsKey) {
            knownTweakPackageIDs = Set(knownPackageIDs)
        }
        isEnabled = defaults.object(forKey: Self.enabledDefaultsKey) == nil
            ? true
            : defaults.bool(forKey: Self.enabledDefaultsKey)
        isDeveloperMode = defaults.bool(forKey: Self.developerModeDefaultsKey)
    }

    func start() {
        guard monitorTask == nil else { return }
        do {
            try packageStore.prepareUserPackages()
            try updateTweakPackages()
            refreshLog()
            AppLogger.shared.info("Codex Tweaks 已启动")
        } catch {
            status = .error("无法准备功能包：\(error.localizedDescription)")
            AppLogger.shared.error("准备功能包失败：\(error)")
        }

        checkNodeEnvironment()
        checkGitEnvironment()
        monitorTask = Task { @MainActor [weak self] in
            guard let self else { return }
            while !Task.isCancelled {
                await self.refresh(force: false)
                do {
                    try await Task.sleep(nanoseconds: 2_000_000_000)
                } catch {
                    break
                }
            }
        }
    }

    func stop() {
        monitorTask?.cancel()
        monitorTask = nil
        promptCopyResetTask?.cancel()
        promptCopyResetTask = nil
        AppLogger.shared.info("Codex Tweaks 已退出")
    }

    func openCodex() {
        do {
            if launcher.isRunning {
                launcher.activate()
            } else {
                hasAttemptedInitialLaunch = true
                status = .launchingCodex
                try launcher.launchWithCDP()
                AppLogger.shared.info("已使用本地 CDP 参数启动 Codex")
            }
        } catch {
            status = .error(error.localizedDescription)
            AppLogger.shared.error("打开 Codex 失败：\(error)")
        }
    }

    func confirmAndRestartCodex() {
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = "重新启动 Codex？"
        alert.informativeText = "Codex 只有在启动时才能开启 CDP。重启后 Codex Tweaks 会自动恢复注入。"
        alert.addButton(withTitle: "重新启动")
        alert.addButton(withTitle: "取消")
        guard alert.runModal() == .alertFirstButtonReturn else { return }

        hasAttemptedInitialLaunch = true
        status = .launchingCodex
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await self.launcher.restartWithCDP()
                AppLogger.shared.info("Codex 已重启并开启本地 CDP")
                self.status = .waitingForCDP
            } catch {
                self.status = .error(error.localizedDescription)
                AppLogger.shared.error("重启 Codex 失败：\(error)")
            }
        }
    }

    func reinject() {
        forceGeneration &+= 1
        Task { @MainActor [weak self] in await self?.refresh(force: true) }
    }

    func isTweakPackageEnabled(_ package: TweakPackage) -> Bool {
        !disabledTweakPackageIDs.contains(package.id)
    }

    func setTweakPackage(_ package: TweakPackage, isEnabled: Bool) {
        var updated = disabledTweakPackageIDs
        if isEnabled { updated.remove(package.id) } else { updated.insert(package.id) }
        guard updated != disabledTweakPackageIDs else { return }

        disabledTweakPackageIDs = updated
        persistDisabledTweakPackages()
        packageRuntimeErrors.removeValue(forKey: package.id)
        AppLogger.shared.info("已\(isEnabled ? "启用" : "停用")功能包：\(package.displayName)")
        if isEnabled { scheduleDeveloperBuilds() }
        Task { @MainActor [weak self] in await self?.refresh(force: true) }
    }

    func setTweakPackagePriority(_ package: TweakPackage, priority: Int) {
        guard let currentPackage = tweakPackages.first(where: { $0.id == package.id }) else {
            return
        }
        let priorityOverride = priority == currentPackage.declaredPriority ? nil : priority
        guard priorityOverride != currentPackage.priorityOverride else { return }
        do {
            try packageStore.setPriorityOverride(priorityOverride, forPackageID: currentPackage.id)
            try updateTweakPackages()
            forceGeneration &+= 1
            if let priorityOverride {
                AppLogger.shared.info(
                    "已将功能包 \(currentPackage.displayName) 的用户优先级设为 \(priorityOverride)"
                )
            } else {
                AppLogger.shared.info(
                    "功能包 \(currentPackage.displayName) 的优先级与声明值一致，已移除用户覆盖"
                )
            }
            Task { @MainActor [weak self] in await self?.refresh(force: true) }
        } catch {
            AppLogger.shared.error("保存功能包优先级失败：\(error)")
        }
    }

    func resetTweakPackagePriority(_ package: TweakPackage) {
        do {
            try packageStore.setPriorityOverride(nil, forPackageID: package.id)
            try updateTweakPackages()
            forceGeneration &+= 1
            AppLogger.shared.info("已恢复功能包 \(package.displayName) 的默认优先级")
            Task { @MainActor [weak self] in await self?.refresh(force: true) }
        } catch {
            AppLogger.shared.error("恢复功能包默认优先级失败：\(error)")
        }
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
        let installedIDs = Set(tweakPackages.compactMap { $0.manifest?.name })
        return package.packageDependencies.contains { dependencyID, dependency in
            !installedIDs.contains(dependencyID) && dependency.source != nil
        }
    }

    func canEnableDependencies(for package: TweakPackage) -> Bool {
        package.runtimePackageDependencies.keys.contains {
            disabledTweakPackageIDs.contains($0)
        }
    }

    func enableDependencies(for package: TweakPackage) {
        var pending = Array(package.runtimePackageDependencies.keys)
        var enabledIDs = Set<String>()
        while let packageID = pending.popLast() {
            guard enabledIDs.insert(packageID).inserted else { continue }
            if let dependency = tweakPackages.first(where: { $0.id == packageID }) {
                pending.append(contentsOf: dependency.runtimePackageDependencies.keys)
            }
        }
        let updated = disabledTweakPackageIDs.subtracting(enabledIDs)
        guard updated != disabledTweakPackageIDs else { return }
        disabledTweakPackageIDs = updated
        persistDisabledTweakPackages()
        do { try updateTweakPackages() } catch {
            AppLogger.shared.error("启用依赖后重新读取功能包失败：\(error)")
        }
        AppLogger.shared.info("已启用 \(package.displayName) 的依赖链")
        Task { @MainActor [weak self] in await self?.refresh(force: true) }
    }

    func reloadTweakPackages() {
        do {
            try updateTweakPackages()
            scheduleDeveloperBuilds()
        } catch {
            status = .error("无法读取功能包：\(error.localizedDescription)")
            AppLogger.shared.error("读取功能包失败：\(error)")
        }
    }

    func checkNodeEnvironment() {
        guard !isCheckingNode else { return }
        isCheckingNode = true
        Task { @MainActor [weak self] in
            guard let self else { return }
            let environment = await self.packageBuilder.detectNodeEnvironment()
            self.nodeEnvironment = environment
            self.isCheckingNode = false
            if let environment {
                AppLogger.shared.info("已检测到 Node.js \(environment.version)")
                self.scheduleDeveloperBuilds()
            } else {
                AppLogger.shared.error("未找到可用的 Node.js、npm 和 npx")
            }
        }
    }

    func checkGitEnvironment() {
        guard !isCheckingGit else { return }
        isCheckingGit = true
        Task { @MainActor [weak self] in
            guard let self else { return }
            let environment = await self.remotePackageManager.detectGitEnvironment()
            self.gitEnvironment = environment
            self.isCheckingGit = false
            if let environment {
                AppLogger.shared.info("已检测到 \(environment.version)")
                self.checkManagedPackageUpdates(automatic: true)
            } else {
                AppLogger.shared.error("未找到可用的 Git")
            }
        }
    }

    func installRemotePackage(
        repositoryURL: String,
        selectorType: TweakPackageRemoteSelectorType,
        selectorValue: String
    ) {
        guard !isInstallingRemotePackage, !isInstallingLocalPackage else { return }
        isInstallingRemotePackage = true
        remoteOperationMessage = nil
        remoteOperationError = nil
        let source = TweakPackageSource(
            url: repositoryURL.trimmingCharacters(in: .whitespacesAndNewlines),
            selector: TweakPackageRemoteSelector(type: selectorType, value: selectorValue)
        )
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let result = try await self.remotePackageManager.install(source: source)
                self.isInstallingRemotePackage = false
                self.remoteOperationMessage = "已安装 \(result.packageID)，新包默认保持停用。"
                self.disabledTweakPackageIDs.insert(result.packageID)
                self.persistDisabledTweakPackages()
                try self.updateTweakPackages()
                AppLogger.shared.info(
                    "已从 Git 安装功能包 \(result.packageID) @ \(result.lock.resolvedCommit.prefix(12))"
                )
                if let package = self.tweakPackages.first(where: { $0.id == result.packageID }),
                   self.nodeEnvironment != nil {
                    self.startPackageBuild(
                        package,
                        installDependencies: true,
                        allowCompilerDownload: true,
                        isAutomatic: false
                    )
                }
            } catch {
                self.isInstallingRemotePackage = false
                self.remoteOperationError = error.localizedDescription
                AppLogger.shared.error("从 Git 安装功能包失败：\(error)")
            }
        }
    }

    func clearRemoteOperationFeedback() {
        remoteOperationMessage = nil
        remoteOperationError = nil
    }

    func installLocalPackage(from sourceURL: URL) {
        guard !isInstallingLocalPackage, !isInstallingRemotePackage else { return }
        isInstallingLocalPackage = true
        localOperationMessage = nil
        localOperationError = nil
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let result = try await self.localPackageInstaller.install(from: sourceURL)
                self.isInstallingLocalPackage = false
                self.disabledTweakPackageIDs.insert(result.packageID)
                self.persistDisabledTweakPackages()
                try self.updateTweakPackages()
                self.localOperationMessage = "已安装 \(result.packageID)，新包默认保持停用。"
                AppLogger.shared.info(
                    "已从本地安装功能包 \(result.packageID)：\(result.directoryURL.path)"
                )
                if let package = self.tweakPackages.first(where: { $0.id == result.packageID }),
                   self.nodeEnvironment != nil {
                    self.startPackageBuild(
                        package,
                        installDependencies: true,
                        allowCompilerDownload: true,
                        isAutomatic: false
                    )
                }
            } catch {
                self.isInstallingLocalPackage = false
                self.localOperationError = error.localizedDescription
                AppLogger.shared.error("从本地安装功能包失败：\(error)")
            }
        }
    }

    func clearLocalOperationFeedback() {
        localOperationMessage = nil
        localOperationError = nil
    }

    func reportLocalPackageSelectionError(_ error: Error) {
        localOperationMessage = nil
        localOperationError = error.localizedDescription
        AppLogger.shared.error("选择本地功能包失败：\(error)")
    }

    func installMissingDependencies(for package: TweakPackage) {
        guard !installingPackageIDs.contains(package.id) else { return }
        installingPackageIDs.insert(package.id)
        remotePackageErrors.removeValue(forKey: package.id)
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let results = try await self.remotePackageManager.installMissingDependencies(
                    for: package,
                    installedPackages: self.tweakPackages
                )
                self.installingPackageIDs.remove(package.id)
                try self.updateTweakPackages()
                for result in results {
                    if let installed = self.tweakPackages.first(where: { $0.id == result.packageID }),
                       self.nodeEnvironment != nil {
                        self.startPackageBuild(
                            installed,
                            installDependencies: true,
                            allowCompilerDownload: true,
                            isAutomatic: false
                        )
                    }
                }
                AppLogger.shared.info(
                    results.isEmpty
                        ? "功能包 \(package.displayName) 的依赖均已安装"
                        : "已为 \(package.displayName) 安装 \(results.count) 个依赖包"
                )
            } catch {
                self.installingPackageIDs.remove(package.id)
                self.remotePackageErrors[package.id] = error.localizedDescription
                AppLogger.shared.error("安装功能包依赖失败：\(error)")
            }
        }
    }

    func checkManagedPackageUpdates(automatic: Bool = false) {
        guard gitEnvironment != nil, !isCheckingRemoteUpdates else { return }
        if automatic,
           let lastAutomaticRemoteCheckAt,
           Date().timeIntervalSince(lastAutomaticRemoteCheckAt)
            < Self.automaticRemoteCheckInterval {
            return
        }
        isCheckingRemoteUpdates = true
        if automatic { lastAutomaticRemoteCheckAt = Date() }
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                let packageIDs = try await self.remotePackageManager.managedPackageIDs()
                for packageID in packageIDs {
                    do {
                        let update = try await self.remotePackageManager.checkForUpdate(
                            packageID: packageID
                        )
                        self.remotePackageUpdates[packageID] = update
                        self.remotePackageErrors.removeValue(forKey: packageID)
                    } catch {
                        self.remotePackageErrors[packageID] = error.localizedDescription
                    }
                }
                self.isCheckingRemoteUpdates = false
                if !automatic { AppLogger.shared.info("远程功能包更新检查完成") }
            } catch {
                self.isCheckingRemoteUpdates = false
                AppLogger.shared.error("读取远程功能包注册表失败：\(error)")
            }
        }
    }

    func updateManagedPackage(_ package: TweakPackage) {
        guard package.isManaged, !installingPackageIDs.contains(package.id) else { return }
        installingPackageIDs.insert(package.id)
        remotePackageErrors.removeValue(forKey: package.id)
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                guard let registration = try await self.remotePackageManager.registration(
                    for: package.id
                ) else {
                    throw TweakPackageRemoteError.packageNotManaged(package.id)
                }
                _ = try await self.remotePackageManager.install(
                    source: registration.source,
                    expectedPackageID: package.id
                )
                self.installingPackageIDs.remove(package.id)
                self.remotePackageUpdates.removeValue(forKey: package.id)
                try self.updateTweakPackages()
                if let updated = self.tweakPackages.first(where: { $0.id == package.id }) {
                    self.startPackageBuild(
                        updated,
                        installDependencies: true,
                        allowCompilerDownload: true,
                        isAutomatic: false
                    )
                }
                AppLogger.shared.info("已下载远程功能包更新：\(package.displayName)")
            } catch {
                self.installingPackageIDs.remove(package.id)
                self.remotePackageErrors[package.id] = error.localizedDescription
                AppLogger.shared.error("更新远程功能包失败：\(error)")
            }
        }
    }

    func buildPackage(_ package: TweakPackage) {
        startPackageBuild(
            package,
            installDependencies: true,
            allowCompilerDownload: true,
            isAutomatic: false
        )
    }

    func openPackageDirectory(_ package: TweakPackage) { open(package.directoryURL) }
    func openTweaksDirectory() { open(packageStore.packagesDirectoryURL) }

    func refreshLog() {
        do {
            let latestLog = LogTextFormatter.newestFirst(try AppLogger.shared.readContents())
            if latestLog != logText { logText = latestLog }
        } catch {
            logText = "日志读取失败：\(error.localizedDescription)"
        }
    }

    @discardableResult
    func clearLog() -> String? {
        do {
            try AppLogger.shared.clear()
            logText = ""
            return nil
        } catch {
            return error.localizedDescription
        }
    }

    func quit() {
        monitorTask?.cancel()
        monitorTask = nil
        promptCopyResetTask?.cancel()
        promptCopyResetTask = nil
        Task { @MainActor [weak self] in
            guard let self else {
                NSApplication.shared.terminate(nil)
                return
            }
            try? await self.cdpService.cleanupAllTargets()
            NSApplication.shared.terminate(nil)
        }
    }

    func copyAuthoringPrompt() {
        guard TweakAuthoringPromptBuilder.copyPrompt(
            tweaksDirectoryURL: packageStore.tweaksDirectoryURL
        ) else {
            AppLogger.shared.error("复制功能包开发 Skill 失败")
            return
        }

        promptCopyResetTask?.cancel()
        isAuthoringPromptCopied = true
        AppLogger.shared.info("已复制功能包开发 Skill")
        promptCopyResetTask = Task { @MainActor [weak self] in
            do {
                try await Task.sleep(nanoseconds: 2_000_000_000)
            } catch {
                return
            }
            self?.isAuthoringPromptCopied = false
        }
    }

    func openLog() {
        AppLogger.shared.ensureLogFileExists()
        open(AppLogger.shared.logFileURL)
    }

    private func open(_ url: URL) {
        do {
            try packageStore.prepareUserPackages()
            NSWorkspace.shared.open(url)
        } catch {
            status = .error(error.localizedDescription)
            AppLogger.shared.error("打开 \(url.path) 失败：\(error)")
        }
    }

    private func refresh(force _: Bool) async {
        guard !isRefreshing else { return }
        isRefreshing = true
        defer { isRefreshing = false }

        do {
            try updateTweakPackages()
            scheduleDeveloperBuilds()
            checkManagedPackageUpdates(automatic: true)
        } catch {
            status = .error("无法读取功能包：\(error.localizedDescription)")
            return
        }

        if !isEnabled {
            if !disabledCleanupCompleted {
                try? await cdpService.cleanupAllTargets()
                disabledCleanupCompleted = true
            }
            packageRuntimeErrors = [:]
            status = .disabled
            return
        }
        disabledCleanupCompleted = false

        guard launcher.isRunning else {
            guard !hasAttemptedInitialLaunch else {
                status = .codexNotRunning
                return
            }
            hasAttemptedInitialLaunch = true
            status = .launchingCodex
            do {
                try launcher.launchWithCDP()
                AppLogger.shared.info("Codex 未运行，已自动使用本地 CDP 参数启动")
                status = .waitingForCDP
            } catch {
                status = .error(error.localizedDescription)
                AppLogger.shared.error("自动启动 Codex 失败：\(error)")
            }
            return
        }
        hasAttemptedInitialLaunch = true

        do {
            let loadResult = packageStore.loadPayload(
                from: tweakPackages,
                disabledPackageIDs: disabledTweakPackageIDs
            )
            packagePayloadErrors = loadResult.packageErrors
            let result = try await cdpService.inject(
                payload: loadResult.payload,
                forceGeneration: forceGeneration
            )

            if result.packageErrors != packageRuntimeErrors {
                for (packageID, message) in result.packageErrors {
                    AppLogger.shared.error("功能包 \(packageID) 运行失败：\(message)")
                }
                packageRuntimeErrors = result.packageErrors
            }

            if result.targetCount == 0 {
                status = .waitingForPage
            } else if result.successCount > 0 {
                status = .connected(targetCount: result.successCount)
                if result.failureCount > 0 {
                    AppLogger.shared.error("部分 Codex 页面注入失败：\(result.failureCount)/\(result.targetCount)")
                }
            } else {
                status = .error("发现 Codex 页面，但注入没有成功")
            }
        } catch CDPServiceError.endpointUnavailable {
            status = .restartRequired
        } catch {
            status = .error(error.localizedDescription)
            AppLogger.shared.error("CDP 刷新失败：\(error)")
        }
    }

    private func updateTweakPackages() throws {
        let packages = try packageStore.loadPackages()
        let discoveredPackageIDs: Set<String> = Set(packages.compactMap { package -> String? in
            guard package.validationError == nil else { return nil }
            return package.manifest?.name
        })
        let reconciliation = TweakPackageEnablementReconciliation.reconcile(
            discoveredPackageIDs: discoveredPackageIDs,
            knownPackageIDs: knownTweakPackageIDs,
            disabledPackageIDs: disabledTweakPackageIDs
        )
        let knownPackagesChanged = knownTweakPackageIDs != reconciliation.knownPackageIDs
        let disabledPackagesChanged = disabledTweakPackageIDs != reconciliation.disabledPackageIDs
        knownTweakPackageIDs = reconciliation.knownPackageIDs
        disabledTweakPackageIDs = reconciliation.disabledPackageIDs
        if knownPackagesChanged { persistKnownTweakPackages() }
        if disabledPackagesChanged { persistDisabledTweakPackages() }
        for packageID in reconciliation.newlyDiscoveredPackageIDs.sorted() {
            AppLogger.shared.info("发现新功能包，默认保持停用：\(packageID)")
        }

        let dependencyResolution = TweakPackageDependencyResolver.resolve(
            packages: packages,
            disabledPackageIDs: disabledTweakPackageIDs
        )
        if dependencyResolution.dependenciesByPackageID != packageDependencyStatuses {
            packageDependencyStatuses = dependencyResolution.dependenciesByPackageID
        }
        if dependencyResolution.issuesByPackageID != packageDependencyIssues {
            packageDependencyIssues = dependencyResolution.issuesByPackageID
        }
        if dependencyResolution.priorityConstraintsByPackageID != packagePriorityConstraints {
            packagePriorityConstraints = dependencyResolution.priorityConstraintsByPackageID
        }
        if dependencyResolution.orderedPackages != tweakPackages {
            tweakPackages = dependencyResolution.orderedPackages
        }

        let currentRequestKeys = Dictionary(uniqueKeysWithValues: dependencyResolution.orderedPackages.compactMap { package in
            package.buildRequestKey(compilerVersion: TweakPackageStore.compilerVersion)
                .map { (package.id, $0) }
        })
        for packageID in Array(packageBuildErrors.keys) {
            guard packageBuildErrorRequestKeys[packageID] != currentRequestKeys[packageID] else {
                continue
            }
            packageBuildErrors.removeValue(forKey: packageID)
            packageBuildErrorRequestKeys.removeValue(forKey: packageID)
        }
    }

    private func scheduleDeveloperBuilds() {
        guard isDeveloperMode, nodeEnvironment != nil else { return }
        for package in tweakPackages where isTweakPackageEnabled(package) {
            let disposition = package.buildDisposition(
                compilerVersion: TweakPackageStore.compilerVersion
            )
            let mayBuildAutomatically = disposition == .sourceChanged
                || (disposition == .notBuilt && !package.hasDependencies)
            guard mayBuildAutomatically,
                  let requestKey = package.buildRequestKey(
                      compilerVersion: TweakPackageStore.compilerVersion
                  ),
                  developerBuildAttemptKeys[package.id] != requestKey,
                  !buildingPackageIDs.contains(package.id) else {
                continue
            }
            developerBuildAttemptKeys[package.id] = requestKey
            startPackageBuild(
                package,
                installDependencies: false,
                allowCompilerDownload: false,
                isAutomatic: true
            )
        }
    }

    private func startPackageBuild(
        _ package: TweakPackage,
        installDependencies: Bool,
        allowCompilerDownload: Bool,
        isAutomatic: Bool
    ) {
        guard package.validationError == nil,
              !buildingPackageIDs.contains(package.id) else { return }

        buildingPackageIDs.insert(package.id)
        packageBuildErrors.removeValue(forKey: package.id)
        packageBuildErrorRequestKeys.removeValue(forKey: package.id)
        let requestKey = package.buildRequestKey(
            compilerVersion: TweakPackageStore.compilerVersion
        )
        AppLogger.shared.info(
            "\(isAutomatic ? "自动编译" : "手动更新")功能包：\(package.displayName)"
        )

        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                _ = try await self.packageBuilder.build(
                    package: package,
                    installDependencies: installDependencies,
                    allowCompilerDownload: allowCompilerDownload
                )
                self.buildingPackageIDs.remove(package.id)
                try self.updateTweakPackages()
                self.forceGeneration &+= 1
                AppLogger.shared.info("功能包已编译并激活：\(package.displayName)")
                await self.refresh(force: true)
            } catch {
                self.buildingPackageIDs.remove(package.id)
                self.packageBuildErrors[package.id] = error.localizedDescription
                if let requestKey {
                    self.packageBuildErrorRequestKeys[package.id] = requestKey
                }
                AppLogger.shared.error("功能包 \(package.displayName) 编译失败：\(error)")
            }
        }
    }

    private func persistDisabledTweakPackages() {
        UserDefaults.standard.set(
            disabledTweakPackageIDs.sorted(),
            forKey: Self.disabledPackagesDefaultsKey
        )
    }

    private func persistKnownTweakPackages() {
        UserDefaults.standard.set(
            knownTweakPackageIDs?.sorted() ?? [],
            forKey: Self.knownPackagesDefaultsKey
        )
    }
}
