import AppKit
import Combine
import Foundation

/// 管理 macOS 应用入口的本地偏好与激活策略。
@MainActor
final class MacOSAppVisibilityController: ObservableObject {
    static let shared = MacOSAppVisibilityController()

    @Published private(set) var hidesDockIcon: Bool
    @Published private(set) var hidesMenuBarIcon: Bool

    private let defaults: UserDefaults

    private init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        MacOSAppVisibilityPreferences.registerDefaults(in: defaults)
        hidesDockIcon = MacOSAppVisibilityPreferences.hidesDockIcon(in: defaults)
        hidesMenuBarIcon = MacOSAppVisibilityPreferences.hidesMenuBarIcon(in: defaults)
    }

    var shouldShowMenuBarIcon: Bool {
        !hidesMenuBarIcon
    }

    /// 将持久化的 Dock 图标偏好应用到当前进程。
    func applyCurrentActivationPolicy() {
        let policy: NSApplication.ActivationPolicy = hidesDockIcon ? .accessory : .regular
        guard NSApplication.shared.activationPolicy() != policy else { return }
        _ = NSApplication.shared.setActivationPolicy(policy)
    }

    /// 更新 Dock 与应用切换器中的可见性。
    func setHidesDockIcon(_ hidden: Bool) {
        guard hidden != hidesDockIcon else { return }
        hidesDockIcon = hidden
        MacOSAppVisibilityPreferences.setHidesDockIcon(hidden, in: defaults)
        applyCurrentActivationPolicy()

        // 恢复 Dock 图标时主动激活应用，确保主窗口立即可见。
        if !hidden {
            NSApplication.shared.activate(ignoringOtherApps: true)
        }
    }

    /// 更新菜单栏状态项的可见性。
    func setHidesMenuBarIcon(_ hidden: Bool) {
        guard hidden != hidesMenuBarIcon else { return }
        hidesMenuBarIcon = hidden
        MacOSAppVisibilityPreferences.setHidesMenuBarIcon(hidden, in: defaults)
    }
}

enum MacOSAppVisibilityPreferences {
    static let hidesDockIconKey = "macOSAppVisibility.hidesDockIcon"
    static let hidesMenuBarIconKey = "macOSAppVisibility.hidesMenuBarIcon"

    static func registerDefaults(in defaults: UserDefaults) {
        defaults.register(defaults: [
            hidesDockIconKey: false,
            hidesMenuBarIconKey: false,
        ])
    }

    static func hidesDockIcon(in defaults: UserDefaults) -> Bool {
        registerDefaults(in: defaults)
        return defaults.bool(forKey: hidesDockIconKey)
    }

    static func hidesMenuBarIcon(in defaults: UserDefaults) -> Bool {
        registerDefaults(in: defaults)
        return defaults.bool(forKey: hidesMenuBarIconKey)
    }

    static func setHidesDockIcon(_ hidden: Bool, in defaults: UserDefaults) {
        defaults.set(hidden, forKey: hidesDockIconKey)
    }

    static func setHidesMenuBarIcon(_ hidden: Bool, in defaults: UserDefaults) {
        defaults.set(hidden, forKey: hidesMenuBarIconKey)
    }
}
