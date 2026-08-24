import AppKit
import SwiftUI

struct UpdateView: View {
    @ObservedObject var model: AppModel
    @ObservedObject var updateChecker: UpdateChecker

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: CGFloat(model.tokens.sectionSpacing)) {
                header
                applicationCard
                updateCard
            }
            .frame(maxWidth: CGFloat(model.tokens.contentMaxWidth), alignment: .leading)
            .padding(CGFloat(model.tokens.pagePadding))
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .navigationTitle(model.text(.navUpdates))
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
            Text(model.text(.updateTitle))
                .font(.largeTitle.weight(.semibold))
            Text(model.text(.updateSubtitle))
                .foregroundStyle(.secondary)
        }
    }

    private var applicationCard: some View {
        HStack(spacing: CGFloat(model.tokens.controlSpacing)) {
            Image(nsImage: NSApplication.shared.applicationIconImage)
                .resizable()
                .interpolation(.high)
                .frame(width: 64, height: 64)
                .clipShape(RoundedRectangle(
                    cornerRadius: CGFloat(model.tokens.cardCornerRadius),
                    style: .continuous
                ))

            VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
                Text(model.text(.appName))
                    .font(.title2.weight(.semibold))
                Text(model.text(
                    .updateVersionBuild,
                    [
                        "version": updateChecker.currentVersion,
                        "build": updateChecker.buildNumber,
                    ]
                ))
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }

            Spacer()

            Button(model.text(.updateRepository), systemImage: "arrow.up.right") {
                guard let repositoryURL = URL(string: model.platform.repositoryURL) else { return }
                NSWorkspace.shared.open(repositoryURL)
            }
            .disabled(
                !model.actions.openRepository
                    || URL(string: model.platform.repositoryURL) == nil
            )
        }
        .padding(CGFloat(model.tokens.cardPadding))
        .cardSurface(tokens: model.tokens)
    }

    private var updateCard: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.controlSpacing)) {
            HStack {
                Text(model.text(.updateSoftwareUpdate))
                    .font(.title2.weight(.semibold))
                Spacer()
                updateBadge
            }

            VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
                Picker(model.text(.updateChannel), selection: $updateChecker.channel) {
                    ForEach(UpdateChecker.Channel.allCases) { channel in
                        Text(model.text(channel.titleKey)).tag(channel)
                    }
                }
                .pickerStyle(.segmented)
                .onChange(of: updateChecker.channel) { _ in
                    Task { await updateChecker.check() }
                }

                Text(model.text(updateChecker.channel.detailKey))
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }

            Divider()

            Grid(alignment: .leading, horizontalSpacing: 30, verticalSpacing: 13) {
                GridRow {
                    Text(model.text(.updateCurrentVersion))
                        .foregroundStyle(.secondary)
                    Text(updateChecker.currentVersion)
                        .font(.system(.body, design: .monospaced))
                        .textSelection(.enabled)
                }
                GridRow {
                    Text(model.text(.updateLatestVersion))
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
                                    .foregroundStyle(model.tokens.accentColorValue)
                            }
                        }
                    }
                }
                GridRow {
                    Text(model.text(.updateLastCheck))
                        .foregroundStyle(.secondary)
                    Text(lastCheckText)
                }
            }

            Toggle(model.text(.updateAutoCheck), isOn: $updateChecker.autoCheck)
                .disabled(!model.actions.setUpdatePreferences)

            if let error = updateChecker.lastError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .font(.callout)
                    .foregroundStyle(model.tokens.dangerColorValue)
            } else if updateChecker.lastCheckDate != nil && updateChecker.latestRelease == nil {
                Label(model.text(.updateNoRelease), systemImage: "info.circle")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }

            HStack(spacing: 10) {
                Button(model.text(.updateCheck), systemImage: "arrow.clockwise") {
                    Task { await updateChecker.check(prompt: true) }
                }
                .buttonStyle(.borderedProminent)
                .disabled(!model.actions.checkAppUpdate)

                if updateChecker.hasNewerVersion {
                    Button(model.text(
                        .updateInstall,
                        ["version": updateChecker.latestVersionString]
                    ), systemImage: "arrow.triangle.2.circlepath") {
                        updateChecker.installUpdate()
                    }
                    .disabled(!model.actions.installAppUpdate)
                } else if let releaseURL = updateChecker.latestRelease?.htmlURL {
                    Button(model.text(.updateViewRelease), systemImage: "arrow.up.right") {
                        NSWorkspace.shared.open(releaseURL)
                    }
                }
            }
        }
        .padding(CGFloat(model.tokens.cardPadding))
        .cardSurface(tokens: model.tokens)
    }

    @ViewBuilder
    private var updateBadge: some View {
        if updateChecker.updateAvailable {
            Label(model.text(.updateAvailable), systemImage: "sparkles")
                .font(.callout.weight(.medium))
                .foregroundStyle(model.tokens.accentColorValue)
        } else if updateChecker.checking {
            Text(model.text(.updateChecking))
                .font(.callout)
                .foregroundStyle(.secondary)
        } else if updateChecker.lastCheckDate != nil && updateChecker.latestRelease != nil {
            Text(model.text(.updateCurrent))
                .font(.callout)
                .foregroundStyle(.secondary)
        }
    }

    private var lastCheckText: String {
        updateChecker.lastCheckDate?.formatted(date: .abbreviated, time: .shortened)
            ?? model.text(.updateNever)
    }

}

private extension View {
    func cardSurface(tokens: BackendPresentationTokens) -> some View {
        background(Color(nsColor: .controlBackgroundColor))
            .clipShape(RoundedRectangle(
                cornerRadius: CGFloat(tokens.cardCornerRadius),
                style: .continuous
            ))
            .overlay {
                RoundedRectangle(
                    cornerRadius: CGFloat(tokens.cardCornerRadius),
                    style: .continuous
                )
                    .stroke(Color(nsColor: .separatorColor).opacity(0.7), lineWidth: 1)
            }
    }
}
