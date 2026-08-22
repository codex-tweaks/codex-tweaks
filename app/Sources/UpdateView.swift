import AppKit
import SwiftUI

struct UpdateView: View {
    @ObservedObject var updateChecker: UpdateChecker

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 28) {
                header
                applicationCard
                updateCard
            }
            .frame(maxWidth: 720, alignment: .leading)
            .padding(32)
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .navigationTitle("关于与更新")
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("关于与更新")
                .font(.largeTitle.weight(.semibold))
            Text("选择更新通道，并从 GitHub Releases 获取适合当前 Mac 的版本。")
                .foregroundStyle(.secondary)
        }
    }

    private var applicationCard: some View {
        HStack(spacing: 18) {
            Image(nsImage: NSApplication.shared.applicationIconImage)
                .resizable()
                .interpolation(.high)
                .frame(width: 64, height: 64)
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))

            VStack(alignment: .leading, spacing: 5) {
                Text("Codex Tweaks")
                    .font(.title2.weight(.semibold))
                Text("版本 \(updateChecker.currentVersion)（构建 \(updateChecker.buildNumber)）")
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }

            Spacer()

            Button("项目主页", systemImage: "arrow.up.right") {
                NSWorkspace.shared.open(UpdateChecker.repositoryURL)
            }
        }
        .padding(20)
        .cardSurface()
    }

    private var updateCard: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack {
                Text("软件更新")
                    .font(.title2.weight(.semibold))
                Spacer()
                updateBadge
            }

            VStack(alignment: .leading, spacing: 8) {
                Picker("更新通道", selection: $updateChecker.channel) {
                    ForEach(UpdateChecker.Channel.allCases) { channel in
                        Text(channel.rawValue).tag(channel)
                    }
                }
                .pickerStyle(.segmented)
                .onChange(of: updateChecker.channel) { _ in
                    Task { await updateChecker.check() }
                }

                Text(updateChecker.channel.detail)
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }

            Divider()

            Grid(alignment: .leading, horizontalSpacing: 30, verticalSpacing: 13) {
                GridRow {
                    Text("当前版本")
                        .foregroundStyle(.secondary)
                    Text(updateChecker.currentVersion)
                        .font(.system(.body, design: .monospaced))
                        .textSelection(.enabled)
                }
                GridRow {
                    Text("通道最新版本")
                        .foregroundStyle(.secondary)
                    HStack(spacing: 7) {
                        if updateChecker.checking {
                            ProgressView()
                                .controlSize(.small)
                        } else {
                            Text(updateChecker.latestVersionString)
                                .font(.system(.body, design: .monospaced))
                            if updateChecker.hasNewerVersion {
                                Image(systemName: "arrow.up.circle.fill")
                                    .foregroundStyle(.blue)
                            }
                        }
                    }
                }
                GridRow {
                    Text("上次检查")
                        .foregroundStyle(.secondary)
                    Text(lastCheckText)
                }
            }

            Toggle("启动 Codex Tweaks 时自动检查更新", isOn: $updateChecker.autoCheck)

            if let error = updateChecker.lastError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .font(.callout)
                    .foregroundStyle(.red)
            } else if updateChecker.latestVersionIsSkipped {
                Label(
                    "已跳过版本 \(updateChecker.latestVersionString)，仍可手动下载或恢复提醒。",
                    systemImage: "forward.end"
                )
                .font(.callout)
                .foregroundStyle(.secondary)
            } else if updateChecker.lastCheckDate != nil && updateChecker.latestRelease == nil {
                Label("当前通道还没有可用的 GitHub Release。", systemImage: "info.circle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }

            HStack(spacing: 10) {
                Button("检查更新", systemImage: "arrow.clockwise") {
                    Task { await updateChecker.check(prompt: true) }
                }
                .buttonStyle(.borderedProminent)
                .disabled(updateChecker.checking)

                if updateChecker.hasNewerVersion {
                    Button("下载 \(updateChecker.latestVersionString)", systemImage: "arrow.down.circle") {
                        openLatestDownload()
                    }

                    if updateChecker.latestVersionIsSkipped {
                        Button("恢复提醒") {
                            updateChecker.unskipAndPrompt()
                        }
                    }
                } else if let releaseURL = updateChecker.latestRelease?.htmlURL {
                    Button("查看 Release", systemImage: "arrow.up.right") {
                        NSWorkspace.shared.open(releaseURL)
                    }
                }
            }
        }
        .padding(20)
        .cardSurface()
    }

    @ViewBuilder
    private var updateBadge: some View {
        if updateChecker.updateAvailable {
            Label("有新版本", systemImage: "sparkles")
                .font(.callout.weight(.medium))
                .foregroundStyle(.blue)
        } else if updateChecker.checking {
            Text("正在检查…")
                .font(.callout)
                .foregroundStyle(.secondary)
        } else if updateChecker.lastCheckDate != nil && updateChecker.latestRelease != nil {
            Text(updateChecker.hasNewerVersion ? "已跳过" : "已是最新")
                .font(.callout)
                .foregroundStyle(.secondary)
        }
    }

    private var lastCheckText: String {
        updateChecker.lastCheckDate?.formatted(date: .abbreviated, time: .shortened) ?? "从未"
    }

    private func openLatestDownload() {
        guard let url = updateChecker.downloadURL else { return }
        NSWorkspace.shared.open(url)
    }
}

private extension View {
    func cardSurface() -> some View {
        background(Color(nsColor: .controlBackgroundColor))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay {
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(Color(nsColor: .separatorColor).opacity(0.7), lineWidth: 1)
            }
    }
}
