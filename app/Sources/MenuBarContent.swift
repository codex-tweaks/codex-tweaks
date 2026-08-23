import AppKit
import SwiftUI

struct MenuBarContent: View {
    @Environment(\.openWindow) private var openWindow
    @ObservedObject var model: AppModel
    @ObservedObject var updateChecker: UpdateChecker

    var body: some View {
        Label(model.statusTitle, systemImage: model.status.symbol)

        if let detail = model.statusDetail {
            Text(detail)
                .font(.caption)
        }

        Divider()

        Button(model.text(.menuShow)) {
            NSApplication.shared.activate(ignoringOtherApps: true)
            openWindow(id: "main")
        }
        .keyboardShortcut("1")

        Button(model.text(.overviewOpenCodex)) {
            model.openCodex()
        }
        .disabled(!model.actions.openCodex)

        if model.actions.restartCodex {
            Button(model.text(.overviewRestartAndConnect)) {
                model.confirmAndRestartCodex()
            }
        }

        Toggle(model.text(.overviewEnable), isOn: $model.isEnabled)
            .disabled(!model.actions.setEnabled)

        Button(model.text(.overviewReinject)) {
            model.reinject()
        }
        .disabled(!model.actions.reinject)

        Divider()

        Button(model.text(.overviewManagePackages)) {
            model.openTweaksDirectory()
        }
        .disabled(!model.actions.openPackagesDirectory)

        Button(model.text(model.isAuthoringPromptCopied ? .overviewCopied : .overviewCopy)) {
            model.copyAuthoringPrompt()
        }
        .disabled(!model.actions.readAuthoringPrompt)

        Button(model.text(.overviewViewLogs)) {
            model.openLog()
        }
        .disabled(!model.actions.openLogFile)

        Divider()

        if updateChecker.updateAvailable {
            Button(model.text(
                .updateDownload,
                ["version": updateChecker.latestVersionString]
            )) {
                if let url = updateChecker.downloadURL {
                    NSWorkspace.shared.open(url)
                }
            }
        }

        Button(model.text(updateChecker.checking ? .updateChecking : .updateCheck)) {
            Task {
                await updateChecker.check(prompt: true)
                if updateChecker.pendingUpdate != nil {
                    NSApplication.shared.activate(ignoringOtherApps: true)
                    openWindow(id: "main")
                }
            }
        }
        .disabled(!model.actions.checkAppUpdate)

        Divider()

        Button(model.text(.menuQuit)) {
            model.quit()
        }
        .keyboardShortcut("q")
    }
}
