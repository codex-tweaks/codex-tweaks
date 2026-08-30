import AppKit
import SwiftUI

enum AppAccessibilityIdentifier {
    static let hideDockIconToggle = "overview.hideDockIcon.toggle"
    static let hideMenuBarIconToggle = "overview.hideMenuBarIcon.toggle"
    static let interfaceEnhancementsToggle = "overview.interfaceEnhancements.toggle"
    static let menuBarExtra = "menuBarExtra"
}

@main
struct CodexTweaksApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel.shared
    @StateObject private var updateChecker = UpdateChecker.shared
    @StateObject private var appVisibilityController = MacOSAppVisibilityController.shared

    var body: some Scene {
        Window(model.text(.appName), id: "main") {
            MainWindowView(
                model: model,
                updateChecker: updateChecker,
                appVisibilityController: appVisibilityController
            )
                .environment(\.locale, model.displayLocale)
                .frame(
                    minWidth: CGFloat(model.tokens.windowMinWidth),
                    minHeight: CGFloat(model.tokens.windowMinHeight)
                )
        }
        .defaultSize(
            width: CGFloat(model.tokens.windowDefaultWidth),
            height: CGFloat(model.tokens.windowDefaultHeight)
        )

        MenuBarExtra(isInserted: menuBarExtraInsertion) {
            MenuBarContent(
                appDelegate: appDelegate,
                model: model,
                updateChecker: updateChecker
            )
                .environment(\.locale, model.displayLocale)
        } label: {
            Image(systemName: model.menuBarSymbol)
                .accessibilityLabel(model.statusTitle)
                .accessibilityIdentifier(AppAccessibilityIdentifier.menuBarExtra)
        }
        .menuBarExtraStyle(.menu)
    }

    private var menuBarExtraInsertion: Binding<Bool> {
        Binding(
            get: {
                ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil
                    && appVisibilityController.shouldShowMenuBarIcon
            },
            set: { inserted in
                guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil,
                      !inserted else { return }
                appVisibilityController.setHidesMenuBarIcon(true)
            }
        )
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate {
    // Keep the Scene-managed window alive when the close button hides it.
    private var mainWindow: NSWindow?

    func applicationWillFinishLaunching(_ notification: Notification) {
        if ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil {
            NSApplication.shared.setActivationPolicy(.prohibited)
            return
        }

        // 在窗口创建前应用激活策略，避免启动时短暂出现 Dock 图标。
        MacOSAppVisibilityController.shared.applyCurrentActivationPolicy()

        NotificationCenter.default.addObserver(
            self,
            selector: #selector(mainWindowDidBecomeKey(_:)),
            name: NSWindow.didBecomeKeyNotification,
            object: nil
        )
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else {
            return
        }
        Task { @MainActor in
            SparkleUpdateController.shared.start()
            AppModel.shared.start()
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else {
            return
        }
        NotificationCenter.default.removeObserver(self)
        Task { @MainActor in
            AppModel.shared.stop()
        }
    }

    func showMainWindow() -> Bool {
        guard let mainWindow else { return false }
        NSApplication.shared.activate(ignoringOtherApps: true)
        if mainWindow.isMiniaturized {
            mainWindow.deminiaturize(nil)
        }
        mainWindow.makeKeyAndOrderFront(nil)
        return true
    }

    // Closing the main window hides it while the menu bar and backend stay alive.
    func windowShouldClose(_ sender: NSWindow) -> Bool {
        sender.orderOut(nil)
        return false
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }

    func applicationShouldHandleReopen(
        _ sender: NSApplication,
        hasVisibleWindows _: Bool
    ) -> Bool {
        !showMainWindow()
    }

    @objc
    private func mainWindowDidBecomeKey(_ notification: Notification) {
        guard let window = notification.object as? NSWindow,
              !(window is NSPanel),
              !window.isSheet,
              window.parent == nil,
              window.styleMask.contains(.titled),
              mainWindow == nil || mainWindow === window else {
            return
        }
        mainWindow = window
        window.isReleasedWhenClosed = false
        window.isRestorable = false
        window.delegate = self
    }
}
