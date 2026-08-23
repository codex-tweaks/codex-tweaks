import AppKit
import SwiftUI

struct MenuBarContent: View {
    @Environment(\.openWindow) private var openWindow
    @ObservedObject var model: AppModel
    @ObservedObject var updateChecker: UpdateChecker

    var body: some View {
        Label(model.status.title, systemImage: model.status.symbol)

        if let detail = model.status.detail {
            Text(detail)
                .font(.caption)
        }

        Divider()

        Button("显示 Codex Tweaks") {
            NSApplication.shared.activate(ignoringOtherApps: true)
            openWindow(id: "main")
        }
        .keyboardShortcut("1")

        Button("打开 Codex") {
            model.openCodex()
        }

        if model.status.canRestartCodex {
            Button("重启 Codex 并开启调试…") {
                model.confirmAndRestartCodex()
            }
        }

        Toggle("启用界面增强", isOn: $model.isEnabled)

        Button("重新注入") {
            model.reinject()
        }
        .disabled(!model.isEnabled || !model.status.isCDPAvailable)

        Divider()

        Button("管理功能包") {
            model.openTweaksDirectory()
        }

        Button(model.isAuthoringPromptCopied ? "功能包开发 Skill 已复制" : "复制功能包开发 Skill") {
            model.copyAuthoringPrompt()
        }

        Button("查看日志") {
            model.openLog()
        }

        Divider()

        if updateChecker.updateAvailable {
            Button("下载 Codex Tweaks \(updateChecker.latestVersionString)") {
                if let url = updateChecker.downloadURL {
                    NSWorkspace.shared.open(url)
                }
            }
        }

        Button(updateChecker.checking ? "正在检查更新…" : "检查更新…") {
            Task {
                await updateChecker.check(prompt: true)
                if updateChecker.pendingUpdate != nil {
                    NSApplication.shared.activate(ignoringOtherApps: true)
                    openWindow(id: "main")
                }
            }
        }
        .disabled(updateChecker.checking)

        Divider()

        Button("退出 Codex Tweaks") {
            model.quit()
        }
        .keyboardShortcut("q")
    }
}
