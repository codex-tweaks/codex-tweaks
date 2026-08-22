import AppKit
import SwiftUI

struct MenuBarContent: View {
    @Environment(\.openWindow) private var openWindow
    @ObservedObject var model: AppModel

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

        Button("打开自定义 CSS") {
            model.openCustomCSS()
        }

        Button("打开自定义 JS") {
            model.openCustomJavaScript()
        }

        Button("打开 Tweaks 文件夹") {
            model.openTweaksDirectory()
        }

        Button("查看日志") {
            model.openLog()
        }

        Divider()

        Button("退出 Codex Tweaks") {
            model.quit()
        }
        .keyboardShortcut("q")
    }
}
