import AppKit
import Combine
import Foundation

/// 管理 macOS Agent 模式的本地偏好与应用激活策略
@MainActor
final class MacOSAgentModeController: ObservableObject {
    static let shared = MacOSAgentModeController()

    @Published private(set) var isEnabled: Bool
    @Published private(set) var hidesMenuBarIcon: Bool

    private let defaults: UserDefaults

    private init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        MacOSAgentModePreferences.registerDefaults(in: defaults)
        isEnabled = MacOSAgentModePreferences.isEnabled(in: defaults)
        hidesMenuBarIcon = MacOSAgentModePreferences.hidesMenuBarIcon(in: defaults)
    }

    var shouldShowMenuBarIcon: Bool {
        MacOSAgentModePreferences.shouldShowMenuBarIcon(
            isEnabled: isEnabled,
            hidesMenuBarIcon: hidesMenuBarIcon
        )
    }

    /// 将持久化的 Agent 模式应用到当前进程
    func applyCurrentActivationPolicy() {
        let policy: NSApplication.ActivationPolicy = isEnabled ? .accessory : .regular
        guard NSApplication.shared.activationPolicy() != policy else { return }
        _ = NSApplication.shared.setActivationPolicy(policy)
    }

    /// 更新 Agent 模式并立即刷新 Dock 与应用切换器状态
    func setEnabled(_ enabled: Bool) {
        guard enabled != isEnabled else { return }
        isEnabled = enabled
        MacOSAgentModePreferences.setEnabled(enabled, in: defaults)
        applyCurrentActivationPolicy()

        // 恢复普通模式时主动激活应用，确保 Dock 图标和主窗口立即可见
        if !enabled {
            NSApplication.shared.activate(ignoringOtherApps: true)
        }
    }

    /// 更新 Agent 模式下菜单栏状态项的可见性
    func setHidesMenuBarIcon(_ hidden: Bool) {
        guard hidden != hidesMenuBarIcon else { return }
        hidesMenuBarIcon = hidden
        MacOSAgentModePreferences.setHidesMenuBarIcon(hidden, in: defaults)
    }
}

/// 定义 Agent 模式偏好的键名、默认值与持久化规则
enum MacOSAgentModePreferences {
    static let enabledKey = "macOSAgentMode.enabled"
    static let hidesMenuBarIconKey = "macOSAgentMode.hidesMenuBarIcon"

    /// 注册新安装与升级场景使用的默认值
    static func registerDefaults(in defaults: UserDefaults) {
        defaults.register(defaults: [
            enabledKey: false,
            hidesMenuBarIconKey: true,
        ])
    }

    static func isEnabled(in defaults: UserDefaults) -> Bool {
        registerDefaults(in: defaults)
        return defaults.bool(forKey: enabledKey)
    }

    static func hidesMenuBarIcon(in defaults: UserDefaults) -> Bool {
        registerDefaults(in: defaults)
        return defaults.bool(forKey: hidesMenuBarIconKey)
    }

    static func setEnabled(_ enabled: Bool, in defaults: UserDefaults) {
        defaults.set(enabled, forKey: enabledKey)
    }

    static func setHidesMenuBarIcon(_ hidden: Bool, in defaults: UserDefaults) {
        defaults.set(hidden, forKey: hidesMenuBarIconKey)
    }

    /// 普通模式始终保留菜单栏入口，避免无恢复入口的配置
    static func shouldShowMenuBarIcon(
        isEnabled: Bool,
        hidesMenuBarIcon: Bool
    ) -> Bool {
        !isEnabled || !hidesMenuBarIcon
    }
}
