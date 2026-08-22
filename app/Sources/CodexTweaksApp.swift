import AppKit
import SwiftUI

@main
struct CodexTweaksApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel.shared
    @StateObject private var updateChecker = UpdateChecker.shared

    var body: some Scene {
        Window("Codex Tweaks", id: "main") {
            MainWindowView(model: model, updateChecker: updateChecker)
                .frame(minWidth: 820, minHeight: 560)
        }
        .defaultSize(width: 920, height: 640)

        MenuBarExtra {
            MenuBarContent(model: model, updateChecker: updateChecker)
        } label: {
            Image(systemName: model.menuBarSymbol)
                .accessibilityLabel(model.status.title)
        }
        .menuBarExtraStyle(.menu)
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationWillFinishLaunching(_ notification: Notification) {
        if ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] != nil {
            NSApplication.shared.setActivationPolicy(.prohibited)
        }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else {
            return
        }
        Task { @MainActor in
            AppModel.shared.start()
            await UpdateChecker.shared.autoCheckIfNeeded()
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else {
            return
        }
        Task { @MainActor in
            AppModel.shared.stop()
        }
    }
}
