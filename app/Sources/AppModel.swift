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
            case .starting:
                return "正在启动"
            case .launchingCodex:
                return "正在启动 Codex"
            case .codexNotRunning:
                return "Codex 未运行"
            case .waitingForCDP:
                return "正在等待调试端口"
            case .restartRequired:
                return "Codex 需要重启"
            case .waitingForPage:
                return "正在等待 Codex 页面"
            case let .connected(targetCount):
                return targetCount == 1 ? "已连接 Codex" : "已连接 \(targetCount) 个窗口"
            case .disabled:
                return "界面增强已停用"
            case .error:
                return "连接异常"
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
            case .connected:
                return "checkmark.circle.fill"
            case .disabled:
                return "pause.circle"
            case .codexNotRunning:
                return "circle"
            case .restartRequired:
                return "arrow.clockwise.circle"
            case .error:
                return "exclamationmark.triangle.fill"
            default:
                return "circle.dotted"
            }
        }

        var isCDPAvailable: Bool {
            switch self {
            case .connected, .waitingForPage:
                return true
            default:
                return false
            }
        }

        var canRestartCodex: Bool {
            if case .restartRequired = self {
                return true
            }
            return false
        }
    }

    @Published private(set) var status: Status = .starting
    @Published private(set) var logText = ""
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

    var menuBarSymbol: String {
        switch status {
        case .connected:
            return "wand.and.stars.inverse"
        case .error, .restartRequired:
            return "wand.and.stars"
        default:
            return "sparkles"
        }
    }

    var tweaksDirectoryPath: String {
        resourceStore.tweaksDirectoryURL.path
    }

    var logFilePath: String {
        AppLogger.shared.logFileURL.path
    }

    private static let enabledDefaultsKey = "tweaksEnabled"
    private let launcher = CodexLauncher()
    private let resourceStore = TweakResourceStore()
    private let cdpService = CDPService()
    private var monitorTask: Task<Void, Never>?
    private var isRefreshing = false
    private var disabledCleanupCompleted = false
    private var hasAttemptedInitialLaunch = false
    private var forceGeneration = 0

    private init() {
        if UserDefaults.standard.object(forKey: Self.enabledDefaultsKey) == nil {
            isEnabled = true
        } else {
            isEnabled = UserDefaults.standard.bool(forKey: Self.enabledDefaultsKey)
        }
    }

    func start() {
        guard monitorTask == nil else { return }

        do {
            try resourceStore.prepareUserTweaks()
            refreshLog()
            AppLogger.shared.info("Codex Tweaks 已启动")
        } catch {
            status = .error("无法准备自定义文件：\(error.localizedDescription)")
            AppLogger.shared.error("准备自定义文件失败：\(error)")
        }

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
        Task { @MainActor [weak self] in
            await self?.refresh(force: true)
        }
    }

    func refreshLog() {
        do {
            let contents = try AppLogger.shared.readContents()
            let latestLog = LogTextFormatter.newestFirst(contents)
            if latestLog != logText {
                logText = latestLog
            }
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
        Task { @MainActor [weak self] in
            guard let self else {
                NSApplication.shared.terminate(nil)
                return
            }
            try? await self.cdpService.cleanupAllTargets()
            NSApplication.shared.terminate(nil)
        }
    }

    func openCustomCSS() {
        open(resourceStore.customCSSURL)
    }

    func openCustomJavaScript() {
        open(resourceStore.customJavaScriptURL)
    }

    func openTweaksDirectory() {
        open(resourceStore.tweaksDirectoryURL)
    }

    func openLog() {
        AppLogger.shared.ensureLogFileExists()
        open(AppLogger.shared.logFileURL)
    }

    private func open(_ url: URL) {
        do {
            try resourceStore.prepareUserTweaks()
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

        if !isEnabled {
            if !disabledCleanupCompleted {
                do {
                    try await cdpService.cleanupAllTargets()
                } catch {
                    // Codex 未开启 CDP 时没有需要清理的页面。
                }
                disabledCleanupCompleted = true
            }
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
            let payload = try resourceStore.loadPayload()
            let result = try await cdpService.inject(
                payload: payload,
                forceGeneration: forceGeneration
            )

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

}
