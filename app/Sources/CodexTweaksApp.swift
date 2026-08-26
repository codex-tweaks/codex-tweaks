import AppKit
import SwiftUI

@main
struct CodexTweaksApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel.shared
    @StateObject private var updateChecker = UpdateChecker.shared
    // A test host must never register a second status item for the installed app.
    @State private var isMenuBarExtraInserted =
        ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil

    var body: some Scene {
        Window(model.text(.appName), id: "main") {
            MainWindowView(model: model, updateChecker: updateChecker)
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

        MenuBarExtra(isInserted: $isMenuBarExtraInserted) {
            MenuBarContent(model: model, updateChecker: updateChecker)
                .environment(\.locale, model.displayLocale)
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
            SparkleUpdateController.shared.start()
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
