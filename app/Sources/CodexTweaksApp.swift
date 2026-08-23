import AppKit
import SwiftUI

@main
struct CodexTweaksApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel.shared
    @StateObject private var updateChecker = UpdateChecker.shared

    var body: some Scene {
        Window(model.text(.appName), id: "main") {
            MainWindowView(model: model, updateChecker: updateChecker)
                .frame(
                    minWidth: CGFloat(model.tokens.windowMinWidth),
                    minHeight: CGFloat(model.tokens.windowMinHeight)
                )
        }
        .defaultSize(
            width: CGFloat(model.tokens.windowMinWidth),
            height: CGFloat(model.tokens.windowMinHeight)
        )

        MenuBarExtra {
            MenuBarContent(model: model, updateChecker: updateChecker)
        } label: {
            Image(systemName: model.menuBarSymbol)
                .accessibilityLabel(model.statusTitle)
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
