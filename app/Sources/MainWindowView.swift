import AppKit
import SwiftUI

struct MainWindowView: View {
    enum Section: String, CaseIterable, Identifiable {
        case overview
        case logs
        case updates

        var id: Self { self }

        var title: String {
            switch self {
            case .overview:
                return "概览"
            case .logs:
                return "运行日志"
            case .updates:
                return "关于与更新"
            }
        }

        var symbol: String {
            switch self {
            case .overview:
                return "rectangle.3.group"
            case .logs:
                return "doc.text.magnifyingglass"
            case .updates:
                return "arrow.triangle.2.circlepath"
            }
        }
    }

    @ObservedObject var model: AppModel
    @ObservedObject var updateChecker: UpdateChecker
    @State private var selection: Section? = .overview

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                ForEach(Section.allCases) { section in
                    Label(section.title, systemImage: section.symbol)
                        .tag(section)
                }
            }
            .navigationTitle("Codex Tweaks")
            .navigationSplitViewColumnWidth(min: 180, ideal: 200, max: 230)
        } detail: {
            switch selection ?? .overview {
            case .overview:
                OverviewView(
                    model: model,
                    showLogs: { selection = .logs }
                )
            case .logs:
                LogView(model: model)
            case .updates:
                UpdateView(updateChecker: updateChecker)
            }
        }
        .navigationSplitViewStyle(.balanced)
        .alert(
            "发现新版本",
            isPresented: Binding(
                get: { updateChecker.pendingUpdate != nil },
                set: { isPresented in
                    if !isPresented {
                        updateChecker.dismissUpdate()
                    }
                }
            ),
            presenting: updateChecker.pendingUpdate
        ) { release in
            Button("下载更新") {
                if let url = updateChecker.downloadURL(for: release) {
                    NSWorkspace.shared.open(url)
                }
                updateChecker.dismissUpdate()
            }
            Button("稍后", role: .cancel) {
                updateChecker.dismissUpdate()
            }
            Button("跳过此版本", role: .destructive) {
                updateChecker.skipUpdate(release)
            }
        } message: { release in
            Text(
                "当前版本为 \(updateChecker.currentVersion)，"
                    + "新版本 \(SemanticVersion.normalizedString(release.tagName)) 已可下载。"
            )
        }
    }
}

private struct OverviewView: View {
    @ObservedObject var model: AppModel
    let showLogs: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 28) {
                header
                statusSurface
                controls
                workflow
            }
            .frame(maxWidth: 760, alignment: .leading)
            .padding(32)
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .navigationTitle("概览")
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("管理 Codex 的本地界面增强")
                .font(.largeTitle.weight(.semibold))
            Text("连接状态、注入控制与常用入口集中在一个窗口中。")
                .font(.body)
                .foregroundStyle(.secondary)
        }
    }

    private var statusSurface: some View {
        HStack(spacing: 18) {
            ZStack {
                Circle()
                    .fill(statusTint.opacity(0.14))
                Image(systemName: model.status.symbol)
                    .font(.system(size: 23, weight: .semibold))
                    .foregroundStyle(statusTint)
            }
            .frame(width: 52, height: 52)
            .accessibilityHidden(true)

            VStack(alignment: .leading, spacing: 4) {
                Text(model.status.title)
                    .font(.title3.weight(.semibold))
                Text(model.status.detail ?? statusSummary)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Spacer(minLength: 16)
            primaryStatusAction
        }
        .padding(20)
        .background(Color(nsColor: .controlBackgroundColor))
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .stroke(Color(nsColor: .separatorColor).opacity(0.7), lineWidth: 1)
        }
    }

    private var controls: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("控制")
                .font(.title2.weight(.semibold))

            HStack(alignment: .center, spacing: 20) {
                VStack(alignment: .leading, spacing: 3) {
                    Text("启用界面增强")
                        .font(.body.weight(.medium))
                    Text("停用后会清理已注入的样式、组件和事件监听器。")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Toggle("启用界面增强", isOn: $model.isEnabled)
                    .labelsHidden()
                    .toggleStyle(.switch)
                    .accessibilityLabel("启用界面增强")
                    .accessibilityHint("停用后会清理已注入的样式、组件和事件监听器")
            }

            Divider()

            HStack(spacing: 10) {
                Button("重新注入", systemImage: "arrow.clockwise") {
                    model.reinject()
                }
                .disabled(!model.isEnabled || !model.status.isCDPAvailable)

                Button("编辑 CSS", systemImage: "paintbrush") {
                    model.openCustomCSS()
                }

                Button("编辑 JavaScript", systemImage: "curlybraces") {
                    model.openCustomJavaScript()
                }

                Spacer()

                Button("查看日志", systemImage: "doc.text") {
                    showLogs()
                }
            }
            .buttonStyle(.bordered)
        }
    }

    private var workflow: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("连接方式")
                .font(.title2.weight(.semibold))

            Grid(alignment: .leading, horizontalSpacing: 28, verticalSpacing: 12) {
                GridRow {
                    Text("CDP 端点").foregroundStyle(.secondary)
                    Text("127.0.0.1:9335")
                        .font(.system(.body, design: .monospaced))
                        .textSelection(.enabled)
                }
                GridRow {
                    Text("注入范围").foregroundStyle(.secondary)
                    Text("仅 app:// 页面")
                }
                GridRow {
                    Text("刷新策略").foregroundStyle(.secondary)
                    Text("每 2 秒检查文件与窗口")
                }
                GridRow {
                    Text("资源目录").foregroundStyle(.secondary)
                    Text(model.tweaksDirectoryPath)
                        .font(.system(.callout, design: .monospaced))
                        .lineLimit(2)
                        .textSelection(.enabled)
                }
            }

            Button("在 Finder 中显示 Tweaks 文件夹") {
                model.openTweaksDirectory()
            }
        }
    }

    @ViewBuilder
    private var primaryStatusAction: some View {
        if model.status.canRestartCodex {
            Button("重启并连接") {
                model.confirmAndRestartCodex()
            }
            .buttonStyle(.borderedProminent)
        } else if model.status.isCDPAvailable {
            Button("打开 Codex") {
                model.openCodex()
            }
            .buttonStyle(.bordered)
        } else {
            Button("打开 Codex") {
                model.openCodex()
            }
            .buttonStyle(.borderedProminent)
        }
    }

    private var statusSummary: String {
        switch model.status {
        case .connected:
            return "CSS 与 JavaScript 已自动应用，文件变化后会重新注入。"
        case .disabled:
            return "Codex 保持运行，但不会应用任何自定义内容。"
        case .starting, .launchingCodex, .waitingForCDP, .waitingForPage:
            return "Codex Tweaks 正在建立本地连接。"
        case .codexNotRunning:
            return "打开 Codex 后会自动建立连接。"
        case .restartRequired:
            return "需要重新启动 Codex 才能开启本地调试端口。"
        case .error:
            return "请查看运行日志了解详细原因。"
        }
    }

    private var statusTint: Color {
        switch model.status {
        case .connected:
            return .green
        case .restartRequired:
            return .orange
        case .error:
            return .red
        case .disabled, .codexNotRunning:
            return .secondary
        default:
            return .accentColor
        }
    }
}

private struct LogView: View {
    private enum PresentedAlert: Identifiable {
        case confirmClear
        case clearFailed(String)

        var id: String {
            switch self {
            case .confirmClear:
                return "confirm-clear"
            case .clearFailed:
                return "clear-failed"
            }
        }
    }

    @ObservedObject var model: AppModel
    @State private var presentedAlert: PresentedAlert?

    var body: some View {
        VStack(spacing: 0) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 5) {
                    Text("连接与注入日志")
                        .font(.largeTitle.weight(.semibold))
                    Text("查看启动、连接和注入过程。")
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("刷新", systemImage: "arrow.clockwise") {
                    model.refreshLog()
                }
                Button("打开日志文件", systemImage: "arrow.up.forward.app") {
                    model.openLog()
                }
                Button(role: .destructive) {
                    presentedAlert = .confirmClear
                } label: {
                    Label("清除日志", systemImage: "trash")
                }
                .disabled(model.logText.isEmpty)
            }
            .padding(.horizontal, 28)
            .padding(.vertical, 26)

            Divider()

            ScrollView(.vertical) {
                Text(model.logText.isEmpty ? "暂无日志" : model.logText)
                    .font(.system(.callout, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .topLeading)
                    .padding(18)
            }
            .background(Color(nsColor: .textBackgroundColor))

            Divider()

            Text(model.logFilePath)
                .font(.system(.caption, design: .monospaced))
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 18)
                .padding(.vertical, 12)
        }
        .navigationTitle("运行日志")
        .alert(item: $presentedAlert) { alert in
            switch alert {
            case .confirmClear:
                return Alert(
                    title: Text("清除所有日志？"),
                    message: Text("日志文件中的现有记录将被永久删除，此操作无法撤销。"),
                    primaryButton: .destructive(Text("清除")) {
                        if let message = model.clearLog() {
                            DispatchQueue.main.async {
                                presentedAlert = .clearFailed(message)
                            }
                        }
                    },
                    secondaryButton: .cancel(Text("取消"))
                )
            case let .clearFailed(message):
                return Alert(
                    title: Text("无法清除日志"),
                    message: Text(message),
                    dismissButton: .default(Text("好"))
                )
            }
        }
        .task {
            while !Task.isCancelled {
                model.refreshLog()
                do {
                    try await Task.sleep(nanoseconds: 2_000_000_000)
                } catch {
                    return
                }
            }
        }
    }
}
