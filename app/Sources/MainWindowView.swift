import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct MainWindowView: View {
    enum Section: String, CaseIterable, Identifiable {
        case overview
        case packages
        case logs
        case updates

        var id: Self { self }

        var titleKey: PresentationTextKey {
            switch self {
            case .overview:
                return .navOverview
            case .packages:
                return .navPackages
            case .logs:
                return .navLogs
            case .updates:
                return .navUpdates
            }
        }

        var symbol: String {
            switch self {
            case .overview:
                return "rectangle.3.group"
            case .packages:
                return "shippingbox"
            case .logs:
                return "doc.text.magnifyingglass"
            case .updates:
                return "arrow.triangle.2.circlepath"
            }
        }
    }

    @ObservedObject var model: AppModel
    @ObservedObject var updateChecker: UpdateChecker
    @ObservedObject var appVisibilityController: MacOSAppVisibilityController
    @State private var selection: Section? = .overview

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                ForEach(Section.allCases) { section in
                    Label(model.text(section.titleKey), systemImage: section.symbol)
                        .tag(section)
                }
            }
            .navigationTitle(model.text(.appName))
            .navigationSplitViewColumnWidth(
                min: CGFloat(model.tokens.navigationWidth - 24),
                ideal: CGFloat(model.tokens.navigationWidth),
                max: CGFloat(model.tokens.navigationWidth + 24)
            )
        } detail: {
            switch selection ?? .overview {
            case .overview:
                OverviewView(
                    model: model,
                    appVisibilityController: appVisibilityController,
                    showPackages: { selection = .packages },
                    showLogs: { selection = .logs }
                )
            case .packages:
                TweakPackagesView(model: model)
            case .logs:
                LogView(model: model)
            case .updates:
                UpdateView(model: model, updateChecker: updateChecker)
            }
        }
        .navigationSplitViewStyle(.balanced)
    }
}

private struct TweakPackagesView: View {
    private enum PackageFilter: String, CaseIterable, Identifiable {
        case all
        case enabled
        case disabled
        case pending
        case error

        var id: Self { self }

        var titleKey: PresentationTextKey {
            switch self {
            case .all: return .packagesFilterAll
            case .enabled: return .packagesFilterEnabled
            case .disabled: return .packagesFilterDisabled
            case .pending: return .packagesFilterPending
            case .error: return .packagesFilterError
            }
        }
    }

    @ObservedObject var model: AppModel
    @State private var searchText = ""
    @State private var selectedFilter: PackageFilter = .all
    @State private var isShowingLocalPackageImporter = false
    @State private var isShowingGitInstall = false
    @State private var expandedDependencyPackageIDs: Set<String> = []
    @State private var priorityHintPackageID: String?

    var body: some View {
        ScrollView {
            // Keep this eager: macOS accessibility snapshots can report transient
            // negative geometry that makes a lazy stack re-enter layout indefinitely.
            VStack(alignment: .leading, spacing: 0) {
                header
                    .padding(.bottom, CGFloat(model.tokens.pagePadding))

                Divider()

                packageListToolbar

                Divider()

                if model.tweakPackages.isEmpty {
                    emptyState
                } else if filteredPackages.isEmpty {
                    filteredEmptyState
                } else {
                    ForEach(filteredPackages) { package in
                        packageRow(package)
                        Divider()
                    }
                }
            }
            .frame(maxWidth: CGFloat(model.tokens.contentMaxWidth), alignment: .leading)
            .padding(CGFloat(model.tokens.pagePadding))
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .navigationTitle(model.text(.navPackages))
        .task {
            model.reloadTweakPackages()
        }
        .sheet(isPresented: $isShowingGitInstall) {
            GitPackageInstallView(model: model)
        }
        .fileImporter(
            isPresented: $isShowingLocalPackageImporter,
            allowedContentTypes: [.folder, .zip],
            allowsMultipleSelection: false,
            onCompletion: handleLocalPackageSelection
        )
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.controlSpacing)) {
            VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
                Text(model.text(.packagesTitle))
                    .font(.largeTitle.weight(.semibold))
                Text(model.text(.packagesSubtitle))
                    .foregroundStyle(.secondary)
            }

            HStack(spacing: 14) {
                Label(
                    model.text(
                        .packagesEnabledSummary,
                        [
                            "enabled": String(model.enabledTweakPackageCount),
                            "total": String(model.tweakPackages.count),
                            "active": String(model.activeTweakPackageCount),
                        ]
                    ),
                    systemImage: "shippingbox"
                )
                .font(.callout.weight(.medium))

                Spacer()

                HStack(spacing: CGFloat(model.tokens.compactSpacing)) {
                    Text(model.text(.packagesDeveloperMode))

                    Toggle("", isOn: $model.isDeveloperMode)
                        .labelsHidden()
                        .toggleStyle(.switch)
                }
                .disabled(!model.actions.setDeveloperMode)

                Button(model.text(.packagesRescan), systemImage: "arrow.clockwise") {
                    model.reloadTweakPackages()
                    model.checkNodeEnvironment()
                    model.checkGitEnvironment()
                }
                .buttonStyle(.bordered)
                .disabled(!model.actions.reloadPackages)
            }

            HStack(spacing: 8) {
                Image(systemName: model.nodeEnvironment == nil ? "exclamationmark.triangle" : "checkmark.circle")
                    .foregroundStyle(
                        model.nodeEnvironment == nil
                            ? model.tokens.warningColorValue
                            : model.tokens.successColorValue
                    )
                if model.isCheckingNode {
                    Text(model.text(.packagesNodeChecking))
                } else if let node = model.nodeEnvironment {
                    Text(model.text(.packagesNodeAvailable, ["version": node.version]))
                    Text(node.nodeURL.path)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                } else {
                    Text(model.text(.packagesNodeMissing))
                        .foregroundStyle(.secondary)
                }
            }
            .font(.callout)

            HStack(spacing: 8) {
                Image(systemName: model.gitEnvironment == nil ? "exclamationmark.triangle" : "checkmark.circle")
                    .foregroundStyle(
                        model.gitEnvironment == nil
                            ? model.tokens.warningColorValue
                            : model.tokens.successColorValue
                    )
                if model.isCheckingGit {
                    Text(model.text(.packagesGitChecking))
                } else if let git = model.gitEnvironment {
                    Text(model.text(.packagesGitAvailable, ["version": git.version]))
                    Text(git.gitURL.path)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                } else {
                    Text(model.text(.packagesGitMissing))
                        .foregroundStyle(.secondary)
                }
            }
            .font(.callout)

            HStack(spacing: 8) {
                Button(
                    model.text(
                        model.isInstallingLocalPackage
                            ? .packagesInstalling
                            : .packagesInstallLocal
                    ),
                    systemImage: "folder.badge.plus"
                ) {
                    model.clearLocalOperationFeedback()
                    isShowingLocalPackageImporter = true
                }
                .buttonStyle(.borderedProminent)
                .disabled(!model.actions.installLocalPackage)
                .help(model.text(.packagesInstallLocalHelp))

                Button(model.text(.packagesInstallRemote), systemImage: "square.and.arrow.down") {
                    isShowingGitInstall = true
                }
                .buttonStyle(.bordered)
                .disabled(!model.actions.installRemotePackage)

                Button(
                    model.text(
                        model.isCheckingRemoteUpdates
                            ? .packagesCheckingRemote
                            : .packagesCheckRemoteUpdates
                    ),
                    systemImage: "arrow.triangle.2.circlepath"
                ) {
                    model.checkManagedPackageUpdates()
                }
                .buttonStyle(.bordered)
                .disabled(!model.actions.checkManagedPackageUpdates)
            }
            .font(.callout)

            localInstallFeedback

            if model.isDeveloperMode {
                VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
                    Text(model.text(.packagesDeveloperModeDetail))
                        .font(.callout)
                        .foregroundStyle(.secondary)
                    Toggle(
                        model.text(.packagesDeveloperAllowUnknownNode),
                        isOn: Binding(
                            get: { model.isDeveloperAllowUnknownNode },
                            set: { model.requestDeveloperAllowUnknownNode($0) }
                        )
                    )
                    .toggleStyle(.switch)
                    Text(model.text(.packagesDeveloperAllowUnknownNodeDetail))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }

        }
    }

    @ViewBuilder
    private var localInstallFeedback: some View {
        if model.isInstallingLocalPackage {
            HStack(spacing: 8) {
                ProgressView()
                    .controlSize(.small)
                Text(model.text(.packagesInstallingLocal))
            }
            .font(.callout)
            .foregroundStyle(.secondary)
        } else if let package = model.tweakPackages.first(where: {
            model.exportingPackageIDs.contains($0.id)
        }) {
            HStack(spacing: 8) {
                ProgressView()
                    .controlSize(.small)
                Text(model.text(.packagesExporting, ["name": package.displayName]))
            }
            .font(.callout)
            .foregroundStyle(.secondary)
        } else if let message = model.localOperationMessage {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Label(message, systemImage: "checkmark.circle.fill")
                    .foregroundStyle(model.tokens.successColorValue)
                    .textSelection(.enabled)
                Spacer(minLength: 8)
                Button {
                    model.clearLocalOperationFeedback()
                } label: {
                    Image(systemName: "xmark")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .accessibilityLabel(model.text(.packagesClearMessage))
            }
            .font(.callout)
        } else if let error = model.localOperationError {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(model.tokens.dangerColorValue)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                Spacer(minLength: 8)
                Button {
                    model.clearLocalOperationFeedback()
                } label: {
                    Image(systemName: "xmark")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .accessibilityLabel(model.text(.packagesClearError))
            }
            .font(.callout)
        }
    }

    private func handleLocalPackageSelection(_ result: Result<[URL], Error>) {
        switch result {
        case let .success(urls):
            guard let sourceURL = urls.first else { return }
            model.installLocalPackage(from: sourceURL)
        case let .failure(error):
            model.reportLocalPackageSelectionError(error)
        }
    }

    private var packageListToolbar: some View {
        HStack(spacing: 12) {
            Label(model.text(.packagesLoadOrder), systemImage: "list.number")

            Text(model.text(.packagesLoadOrderDetail))
                .font(.caption)
                .foregroundStyle(.secondary)

            Spacer(minLength: 12)

            if isFilteringPackages {
                Text("\(filteredPackages.count) / \(model.tweakPackages.count)")
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(.secondary)
            }

            TextField(model.text(.packagesSearchPlaceholder), text: $searchText)
                .textFieldStyle(.roundedBorder)
                .frame(minWidth: 160, idealWidth: 210, maxWidth: 240)

            Picker(model.text(.packagesFilter), selection: $selectedFilter) {
                ForEach(PackageFilter.allCases) { filter in
                    Text(model.text(filter.titleKey)).tag(filter)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .frame(width: 108)
        }
        .padding(.vertical, 10)
    }

    private func packageRow(_ package: TweakPackage) -> some View {
        HStack(alignment: .top, spacing: 14) {
            Toggle(
                model.text(.packagesEnablePackage, ["name": package.displayName]),
                isOn: Binding(
                    get: { model.isTweakPackageEnabled(package) },
                    set: { model.setTweakPackage(package, isEnabled: $0) }
                )
            )
            .labelsHidden()
            .toggleStyle(.switch)
            .padding(.top, 3)
            .disabled(!package.availableActions.setEnabled)

            VStack(alignment: .leading, spacing: 6) {
                packageIdentity(package)
                if let node = package.node {
                    Label(node.reason, systemImage: "terminal")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let detail = packageStatusDetail(package) {
                    Text(detail)
                        .font(.caption)
                        .foregroundStyle(packageStatusColor(package))
                        .lineLimit(3)
                        .textSelection(.enabled)
                }
                if let lock = package.managedLock {
                    HStack(spacing: 6) {
                        Image(systemName: "point.3.connected.trianglepath.dotted")
                        Text(lock.source.url)
                            .lineLimit(1)
                            .truncationMode(.middle)
                        Text("\(lock.resolvedReference) · \(lock.resolvedCommit.prefix(12))")
                            .font(.system(.caption2, design: .monospaced))
                    }
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                    .textSelection(.enabled)
                }
                dependencySection(package)
            }

            Spacer(minLength: 12)

            VStack(alignment: .trailing, spacing: 10) {
                HStack(spacing: 10) {
                    priorityEditor(package)
                    Label(packageStatusTitle(package), systemImage: packageStatusSymbol(package))
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(packageStatusColor(package))
                }

                HStack(spacing: 8) {
                    if package.availableActions.authorizeNode {
                        Button(model.text(.packagesNodeAuthorizationAllow)) {
                            model.authorizeNodePackage(
                                package,
                                enableAfterAuthorization: !model.isTweakPackageEnabled(package)
                            )
                        }
                    }

                    if model.canInstallMissingDependencies(for: package) {
                        Button(model.text(.packagesInstallDependencies)) {
                            model.installMissingDependencies(for: package)
                        }
                        .disabled(!package.availableActions.installMissingDependencies)
                    }

                    if model.canEnableDependencies(for: package) {
                        Button(model.text(.packagesEnableDependencies)) {
                            model.enableDependencies(for: package)
                        }
                        .disabled(!package.availableActions.enableDependencies)
                    }

                    if model.remotePackageUpdates[package.id]?.isInstallable == true {
                        Button(model.text(.packagesUpdateAndBuild)) {
                            model.updateManagedPackage(package)
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(!package.availableActions.updateManagedPackage)
                    }

                    Button {
                        model.openPackageDirectory(package)
                    } label: {
                        Image(systemName: "folder")
                    }
                    .help(model.text(.packagesOpenDirectory))
                    .disabled(!package.availableActions.openDirectory)

                    Button {
                        model.exportPackage(package)
                    } label: {
                        if model.exportingPackageIDs.contains(package.id) {
                            ProgressView()
                                .controlSize(.small)
                        } else {
                            Image(systemName: "archivebox")
                        }
                    }
                    .accessibilityLabel(model.text(.packagesExportZip))
                    .help(model.text(.packagesExportZipHelp))
                    .disabled(!package.availableActions.export)

                    Button(role: .destructive) {
                        model.confirmDeletePackage(package)
                    } label: {
                        Image(systemName: "trash")
                    }
                    .accessibilityLabel(model.text(.packagesDelete))
                    .help(model.text(.packagesDeleteHelp))
                    .disabled(!package.availableActions.delete)

                    Button(buildButtonTitle(package)) {
                        model.buildPackage(package)
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(
                        !package.availableActions.build
                    )
                }
            }
        }
        .padding(.vertical, CGFloat(model.tokens.compactSpacing))
        .accessibilityElement(children: .contain)
    }

    @ViewBuilder
    private func packageIdentity(_ package: TweakPackage) -> some View {
        if let projectPageURL = package.projectPageURL {
            Link(destination: projectPageURL) {
                packageIdentityContent(package, isLinked: true)
            }
            .buttonStyle(.plain)
            .help(model.text(.packagesOpenProjectPage, ["name": package.displayName]))
            .accessibilityLabel(model.text(.packagesOpenProjectPage, ["name": package.displayName]))
        } else {
            packageIdentityContent(package, isLinked: false)
        }
    }

    private func packageIdentityContent(_ package: TweakPackage, isLinked: Bool) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                HStack(spacing: 4) {
                    Text(package.displayName)
                        .font(.body.weight(.semibold))
                    if isLinked {
                        Image(systemName: "arrow.up.right.square")
                            .font(.caption)
                    }
                }
                .foregroundStyle(isLinked ? Color.accentColor : Color.primary)

                Text(model.text(.packagesSourceVersion, ["version": package.version]))
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(.secondary)
                if let active = package.activeBuild {
                    Text(model.text(
                        .packagesActiveVersion,
                        ["version": active.record.packageVersion]
                    ))
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                }
            }
            Text(package.detail)
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .contentShape(Rectangle())
    }

    @ViewBuilder
    private func dependencySection(_ package: TweakPackage) -> some View {
        let statuses = model.dependencyStatuses(for: package)
        if !statuses.isEmpty {
            VStack(alignment: .leading, spacing: 8) {
                Button {
                    if expandedDependencyPackageIDs.contains(package.id) {
                        expandedDependencyPackageIDs.remove(package.id)
                    } else {
                        expandedDependencyPackageIDs.insert(package.id)
                    }
                } label: {
                    HStack(spacing: 6) {
                        Image(
                            systemName: expandedDependencyPackageIDs.contains(package.id)
                                ? "chevron.down"
                                : "chevron.right"
                        )
                        .font(.caption2.weight(.semibold))
                        .frame(width: 10)

                        Image(systemName: dependencySummarySymbol(statuses))
                            .foregroundStyle(dependencySummaryColor(statuses))

                        Text(dependencySummaryTitle(statuses))
                            .foregroundStyle(.secondary)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .font(.caption.weight(.medium))
                .accessibilityLabel(dependencySummaryTitle(statuses))
                .accessibilityValue(
                    model.text(
                        expandedDependencyPackageIDs.contains(package.id)
                            ? .packagesExpanded
                            : .packagesCollapsed
                    )
                )

                if expandedDependencyPackageIDs.contains(package.id) {
                    VStack(alignment: .leading, spacing: 10) {
                        ForEach(statuses) { status in
                            dependencyStatusRow(status)
                        }
                    }
                    .padding(.leading, 16)
                }
            }
        }
    }

    private func dependencyStatusRow(_ status: TweakPackageDependencyStatus) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: dependencyStateSymbol(status.state))
                .foregroundStyle(dependencyStateColor(status.state))
                .frame(width: 14)

            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 7) {
                    Text(status.dependencyID)
                        .font(.caption.weight(.semibold))
                        .lineLimit(1)
                        .truncationMode(.middle)
                    Text(status.requirement)
                        .font(.system(.caption2, design: .monospaced))
                        .foregroundStyle(.tertiary)
                    Text(dependencyOriginTitle(status))
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }

                if let source = dependencySourceForDisplay(status) {
                    HStack(spacing: 5) {
                        Image(systemName: "link")
                        Text(source.url)
                            .lineLimit(1)
                            .truncationMode(.middle)
                        Text(dependencySelectorTitle(source.selector))
                            .font(.system(.caption2, design: .monospaced))
                            .fixedSize(horizontal: true, vertical: false)
                    }
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
                    .textSelection(.enabled)
                } else {
                    Text(model.text(.packagesDependencyNoGitSource))
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }

                if case let .sourceConflict(installedURL) = status.state {
                    Text(model.text(
                        .packagesDependencyInstalledSource,
                        ["url": installedURL]
                    ))
                        .font(.caption2)
                        .foregroundStyle(model.tokens.dangerColorValue)
                        .lineLimit(1)
                        .truncationMode(.middle)
                        .textSelection(.enabled)
                }
            }
            .layoutPriority(1)

            Spacer(minLength: 8)

            Text(dependencyStateTitle(status))
                .font(.caption.weight(.medium))
                .foregroundStyle(dependencyStateColor(status.state))
                .fixedSize(horizontal: true, vertical: false)
        }
        .accessibilityElement(children: .combine)
    }

    private func dependencySummaryTitle(_ statuses: [TweakPackageDependencyStatus]) -> String {
        let pendingCount = statuses.filter { !$0.state.isSatisfied }.count
        if pendingCount == 0 {
            return model.text(
                .packagesDependencySummaryOK,
                ["total": String(statuses.count)]
            )
        }
        if statuses.contains(where: { dependencyStateIsCritical($0.state) }) {
            return model.text(
                .packagesDependencySummaryCritical,
                ["total": String(statuses.count), "pending": String(pendingCount)]
            )
        }
        return model.text(
            .packagesDependencySummaryPending,
            ["total": String(statuses.count), "pending": String(pendingCount)]
        )
    }

    private func dependencySummaryColor(_ statuses: [TweakPackageDependencyStatus]) -> Color {
        if statuses.allSatisfy(\.state.isSatisfied) { return model.tokens.successColorValue }
        return statuses.contains(where: { dependencyStateIsCritical($0.state) })
            ? model.tokens.dangerColorValue
            : model.tokens.warningColorValue
    }

    private func dependencySummarySymbol(_ statuses: [TweakPackageDependencyStatus]) -> String {
        if statuses.allSatisfy(\.state.isSatisfied) { return "checkmark.circle.fill" }
        return statuses.contains(where: { dependencyStateIsCritical($0.state) })
            ? "exclamationmark.triangle.fill"
            : "exclamationmark.circle.fill"
    }

    private func dependencyStateIsCritical(_ state: TweakPackageDependencyState) -> Bool {
        switch state {
        case .sourceConflict, .cycle, .invalidRequirement, .selfReference:
            return true
        default:
            return false
        }
    }

    private func dependencyStateTitle(_ status: TweakPackageDependencyStatus) -> String {
        switch status.state {
        case .satisfied:
            return status.activeVersion.map {
                model.text(.packagesDependencyStateSatisfiedVersion, ["version": $0])
            } ?? model.text(.packagesDependencyStateSatisfied)
        case .missingLocal:
            return model.text(.packagesDependencyStateMissingLocal)
        case .missingInstallable:
            return model.text(.packagesDependencyStateMissingInstallable)
        case .disabled:
            return status.activeVersion.map {
                model.text(.packagesDependencyStateDisabledVersion, ["version": $0])
            } ?? model.text(.packagesDependencyStateDisabled)
        case .notBuilt:
            return status.installedVersion.map {
                model.text(.packagesDependencyStateNotBuiltVersion, ["version": $0])
            } ?? model.text(.packagesDependencyStateNotBuilt)
        case let .versionMismatch(activeVersion):
            return model.text(
                .packagesDependencyStateVersionMismatch,
                ["version": activeVersion]
            )
        case .sourceConflict:
            return model.text(.packagesDependencyStateSourceConflict)
        case .cycle:
            return model.text(.packagesDependencyStateCycle)
        case .blocked:
            return model.text(.packagesDependencyStateBlocked)
        case .invalidRequirement:
            return model.text(.packagesDependencyStateInvalidRequirement)
        case .selfReference:
            return model.text(.packagesDependencyStateSelfReference)
        }
    }

    private func dependencyStateColor(_ state: TweakPackageDependencyState) -> Color {
        if state.isSatisfied { return model.tokens.successColorValue }
        return dependencyStateIsCritical(state)
            ? model.tokens.dangerColorValue
            : model.tokens.warningColorValue
    }

    private func dependencyStateSymbol(_ state: TweakPackageDependencyState) -> String {
        if state.isSatisfied { return "checkmark.circle.fill" }
        return dependencyStateIsCritical(state)
            ? "exclamationmark.triangle.fill"
            : "exclamationmark.circle.fill"
    }

    private func dependencyOriginTitle(_ status: TweakPackageDependencyStatus) -> String {
        if let origin = status.resolvedOrigin {
            switch origin {
            case .local:
                return model.text(.packagesDependencyOriginLocal)
            case .managed:
                return model.text(.packagesDependencyOriginManaged)
            }
        }
        return model.text(
            status.declaredSource == nil
                ? .packagesDependencyOriginLocalOnly
                : .packagesDependencyOriginInstallable
        )
    }

    private func dependencySourceForDisplay(
        _ status: TweakPackageDependencyStatus
    ) -> TweakPackageSource? {
        if let declaredSource = status.declaredSource { return declaredSource }
        if let origin = status.resolvedOrigin, case let .managed(lock) = origin {
            return lock.source
        }
        return nil
    }

    private func dependencySelectorTitle(_ selector: TweakPackageRemoteSelector) -> String {
        let title = selector.type.title(contract: model.presentation)
        if let value = selector.value {
            return model.text(
                .packagesDependencySelector,
                ["type": title, "value": value]
            )
        }
        return title
    }

    private func priorityEditor(_ package: TweakPackage) -> some View {
        HStack(spacing: 4) {
            Text(model.text(.packagesPriority))
                .foregroundStyle(.tertiary)
            TextField(
                model.text(.packagesPriority),
                value: Binding(
                    get: { package.priority },
                    set: { model.setTweakPackagePriority(package, priority: $0) }
                ),
                format: .number
            )
            .labelsHidden()
            .textFieldStyle(.roundedBorder)
            .frame(width: 58)
            .accessibilityLabel(model.text(
                .packagesUserPriority,
                ["name": package.displayName]
            ))
            .disabled(!package.availableActions.setPriority)
            if package.priorityOverride != nil {
                Button {
                    model.resetTweakPackagePriority(package)
                } label: {
                    Image(systemName: "arrow.uturn.backward.circle")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .help(model.text(
                    .packagesResetPriority,
                    ["priority": String(package.declaredPriority)]
                ))
                .disabled(!package.availableActions.setPriority)
            }
            if let constraint = model.priorityConstraint(for: package) {
                Button {
                    priorityHintPackageID = package.id
                } label: {
                    Label(model.text(.packagesDependencyConstraint), systemImage: "info.circle.fill")
                }
                .buttonStyle(.plain)
                .foregroundStyle(model.tokens.accentColorValue)
                .help(priorityConstraintDescription(package, constraint: constraint))
                .popover(
                    isPresented: priorityHintBinding(for: package.id),
                    arrowEdge: .top
                ) {
                    priorityConstraintPopover(package, constraint: constraint)
                }
            }
        }
        .font(.caption)
        .fixedSize(horizontal: true, vertical: false)
    }

    private func priorityHintBinding(for packageID: String) -> Binding<Bool> {
        Binding(
            get: { priorityHintPackageID == packageID },
            set: { isPresented in
                if isPresented {
                    priorityHintPackageID = packageID
                } else if priorityHintPackageID == packageID {
                    priorityHintPackageID = nil
                }
            }
        )
    }

    private func priorityConstraintPopover(
        _ package: TweakPackage,
        constraint: TweakPackagePriorityConstraint
    ) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(
                model.text(.packagesDependencyOrderFirst),
                systemImage: "point.3.connected.trianglepath.dotted"
            )
                .font(.headline)
            Text(priorityConstraintDescription(package, constraint: constraint))
                .fixedSize(horizontal: false, vertical: true)
            Text(model.text(
                .packagesPriorityActual,
                [
                    "position": String(constraint.actualLoadPosition),
                    "priority": String(package.priority),
                ]
            ))
            .foregroundStyle(.secondary)
            .fixedSize(horizontal: false, vertical: true)
        }
        .font(.callout)
        .padding(16)
        .frame(width: 340)
    }

    private func priorityConstraintDescription(
        _ package: TweakPackage,
        constraint: TweakPackagePriorityConstraint
    ) -> String {
        var messages: [String] = []
        if !constraint.mustLoadAfterPackageIDs.isEmpty {
            messages.append(model.text(
                .packagesPriorityAfter,
                [
                    "name": package.displayName,
                    "dependencies": packageNames(constraint.mustLoadAfterPackageIDs),
                ]
            ))
        }
        if !constraint.mustLoadBeforePackageIDs.isEmpty {
            messages.append(model.text(
                .packagesPriorityBefore,
                [
                    "name": package.displayName,
                    "dependencies": packageNames(constraint.mustLoadBeforePackageIDs),
                ]
            ))
        }
        return messages.joined(separator: " ")
    }

    private func packageNames(_ packageIDs: [String]) -> String {
        let names = packageIDs.prefix(3).map { packageID in
            model.tweakPackages.first(where: { $0.id == packageID })?.displayName ?? packageID
        }
        let suffix = packageIDs.count > names.count
            ? model.text(.packagesPackageCountSuffix, ["count": String(packageIDs.count)])
            : ""
        return names.joined(separator: "、") + suffix
    }

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "shippingbox")
                .font(.system(size: 36, weight: .light))
                .foregroundStyle(.secondary)
            Text(model.text(.packagesEmptyTitle))
                .font(.title3.weight(.medium))
            Text(model.text(.packagesEmptyDetail))
                .foregroundStyle(.secondary)
            Button(model.text(.overviewOpenPackagesDirectory)) {
                model.openTweaksDirectory()
            }
            .disabled(!model.actions.openPackagesDirectory)
        }
        .frame(maxWidth: .infinity, minHeight: 260)
        .padding(.horizontal, CGFloat(model.tokens.pagePadding))
        .padding(.vertical, 48)
    }

    private var filteredEmptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "line.3.horizontal.decrease.circle")
                .font(.system(size: 32, weight: .light))
                .foregroundStyle(.secondary)
            Text(model.text(.packagesNoMatchTitle))
                .font(.title3.weight(.medium))
            Text(model.text(.packagesNoMatchDetail))
                .foregroundStyle(.secondary)
            Button(model.text(.packagesClearSearch)) {
                searchText = ""
                selectedFilter = .all
            }
        }
        .frame(maxWidth: .infinity, minHeight: 220)
        .padding(.horizontal, CGFloat(model.tokens.pagePadding))
        .padding(.vertical, 40)
    }

    private var filteredPackages: [TweakPackage] {
        let query = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        return model.tweakPackages.filter { package in
            guard packageMatchesSelectedFilter(package) else { return false }
            guard !query.isEmpty else { return true }

            return [
                package.displayName,
                package.directoryName,
                package.detail,
                package.version,
                package.managedLock?.source.url ?? "",
                package.packageDependencies.keys.sorted().joined(separator: " "),
                package.packageDependencies.values.compactMap { $0.source?.url }
                    .joined(separator: " "),
                model.dependencyStatuses(for: package).compactMap {
                    dependencySourceForDisplay($0)?.url
                }.joined(separator: " "),
            ].contains { value in
                value.localizedCaseInsensitiveContains(query)
            }
        }
    }

    private var isFilteringPackages: Bool {
        !searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            || selectedFilter != .all
    }

    private func packageMatchesSelectedFilter(_ package: TweakPackage) -> Bool {
        switch selectedFilter {
        case .all:
            return true
        case .enabled:
            return model.isTweakPackageEnabled(package)
        case .disabled:
            return !model.isTweakPackageEnabled(package)
        case .pending:
            return package.presentation.isPending
        case .error:
            return package.presentation.isError
        }
    }

    private func packageStatusTitle(_ package: TweakPackage) -> String {
        package.presentation.statusTitle
    }

    private func packageStatusDetail(_ package: TweakPackage) -> String? {
        let detail = package.presentation.statusDetail
        return detail.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? nil : detail
    }

    private func packageStatusColor(_ package: TweakPackage) -> Color {
        switch package.presentation.statusTone {
        case "success": return model.tokens.successColorValue
        case "danger": return model.tokens.dangerColorValue
        case "accent": return model.tokens.accentColorValue
        case "neutral": return .secondary
        default: return model.tokens.warningColorValue
        }
    }

    private func packageHasError(_ package: TweakPackage) -> Bool {
        package.presentation.isError
    }

    private func hasCriticalDependencyIssue(_ package: TweakPackage) -> Bool {
        model.dependencyStatuses(for: package).contains {
            dependencyStateIsCritical($0.state)
        }
    }

    private func packageStatusSymbol(_ package: TweakPackage) -> String {
        if model.installingPackageIDs.contains(package.id) { return "arrow.down.circle" }
        if model.buildingPackageIDs.contains(package.id) { return "arrow.triangle.2.circlepath" }
        if packageHasError(package) { return "exclamationmark.triangle.fill" }
        return package.presentation.isPending
            ? "clock.badge.exclamationmark"
            : "checkmark.circle.fill"
    }

    private func buildButtonTitle(_ package: TweakPackage) -> String {
        if model.buildingPackageIDs.contains(package.id) {
            return model.text(.packagesBuilding)
        }
        switch package.buildDisposition {
        case .notBuilt:
            return model.text(
                package.hasDependencies ? .packagesInstallAndBuild : .packagesBuild
            )
        case .versionUpdate:
            return model.text(.packagesUpdateToVersion, ["version": package.version])
        case .dependencyUpdate:
            return model.text(.packagesSyncAndBuild)
        case .sourceChanged, .compilerUpdate:
            return model.text(.packagesUpdateBuild)
        case .current:
            return model.text(.packagesRebuild)
        case .invalid:
            return model.text(.packagesCannotBuild)
        }
    }
}

private struct GitPackageInstallView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: AppModel
    @State private var repositoryURL = ""
    @State private var selectorType: TweakPackageRemoteSelectorType = .defaultBranch
    @State private var selectorValue = ""

    var body: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.cardPadding)) {
            VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
                Text(model.text(.remoteTitle))
                    .font(.title2.weight(.semibold))
                Text(model.text(.remoteSubtitle))
                    .foregroundStyle(.secondary)
            }

            Form {
                TextField(
                    model.text(.remoteRepository),
                    text: $repositoryURL,
                    prompt: Text(model.text(.remoteRepositoryPlaceholder))
                )

                Picker(model.text(.remoteSelector), selection: $selectorType) {
                    ForEach(TweakPackageRemoteSelectorType.allCases) { type in
                        Text(type.title(contract: model.presentation)).tag(type)
                    }
                }

                if let valueLabel = selectorType.valueLabel(contract: model.presentation) {
                    TextField(valueLabel, text: $selectorValue)
                }
            }
            .formStyle(.grouped)

            Text(model.text(.remoteValidationDetail))
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

            if let message = model.remoteOperationMessage {
                Label(message, systemImage: "checkmark.circle.fill")
                    .foregroundStyle(model.tokens.successColorValue)
                    .textSelection(.enabled)
            }
            if let error = model.remoteOperationError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(model.tokens.dangerColorValue)
                    .textSelection(.enabled)
            }

            HStack {
                Spacer()
                Button(model.text(.remoteClose)) { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button(model.text(
                    model.isInstallingRemotePackage
                        ? .packagesInstalling
                        : .remoteInstall
                )) {
                    model.installRemotePackage(
                        repositoryURL: repositoryURL,
                        selectorType: selectorType,
                        selectorValue: selectorValue
                    )
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.defaultAction)
                .disabled(!canInstall || !model.actions.installRemotePackage)
            }
        }
        .padding(CGFloat(model.tokens.pagePadding))
        .frame(width: 620)
        .frame(minHeight: 420)
        .onAppear {
            model.clearRemoteOperationFeedback()
        }
        .onChange(of: selectorType) { type in
            selectorValue = type == .branch ? model.text(.selectorBranchDefault) : ""
        }
    }

    private var canInstall: Bool {
        !repositoryURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && (selectorType.valueLabelKey == nil
                || !selectorValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            && model.gitEnvironment != nil
    }
}

private struct OverviewView: View {
    @ObservedObject var model: AppModel
    @ObservedObject var appVisibilityController: MacOSAppVisibilityController
    let showPackages: () -> Void
    let showLogs: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: CGFloat(model.tokens.sectionSpacing)) {
                header
                statusSurface
                controls
                aiAuthoring
                workflow
            }
            .frame(maxWidth: CGFloat(model.tokens.contentMaxWidth), alignment: .leading)
            .padding(CGFloat(model.tokens.pagePadding))
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .navigationTitle(model.text(.navOverview))
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
            Text(model.text(.overviewTitle))
                .font(.largeTitle.weight(.semibold))
            Text(model.text(.overviewSubtitle))
                .font(.body)
                .foregroundStyle(.secondary)
        }
    }

    private var statusSurface: some View {
        HStack(spacing: CGFloat(model.tokens.controlSpacing)) {
            ZStack {
                Circle()
                    .fill(statusTint.opacity(0.14))
                Image(systemName: model.status.symbol)
                    .font(.system(size: 23, weight: .semibold))
                    .foregroundStyle(statusTint)
            }
            .frame(
                width: CGFloat(model.tokens.statusIconSize + 16),
                height: CGFloat(model.tokens.statusIconSize + 16)
            )
            .accessibilityHidden(true)

            VStack(alignment: .leading, spacing: 4) {
                Text(model.statusTitle)
                    .font(.title3.weight(.semibold))
                Text(model.statusDetail ?? model.text(.overviewConnectingDetail))
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Spacer(minLength: 16)
            primaryStatusAction
        }
        .padding(CGFloat(model.tokens.cardPadding))
        .background(Color(nsColor: .controlBackgroundColor))
        .clipShape(RoundedRectangle(
            cornerRadius: CGFloat(model.tokens.cardCornerRadius),
            style: .continuous
        ))
        .overlay {
            RoundedRectangle(
                cornerRadius: CGFloat(model.tokens.cardCornerRadius),
                style: .continuous
            )
                .stroke(Color(nsColor: .separatorColor).opacity(0.7), lineWidth: 1)
        }
    }

    private var controls: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.controlSpacing)) {
            Text(model.text(.overviewControl))
                .font(.title2.weight(.semibold))

            OverviewToggleRow(
                title: model.text(.overviewEnable),
                detail: model.text(.overviewEnableDetail),
                accessibilityIdentifier: AppAccessibilityIdentifier.interfaceEnhancementsToggle,
                isOn: $model.isEnabled
            )
            .disabled(!model.actions.setEnabled)

            Divider()

            OverviewToggleRow(
                title: model.text(.overviewDisableGPUAcceleration),
                detail: model.text(.overviewDisableGPUAccelerationDetail),
                accessibilityIdentifier: AppAccessibilityIdentifier.disableGPUAccelerationToggle,
                isOn: $model.isGPUAccelerationDisabled
            )
            .disabled(!model.actions.setDisableGPUAcceleration)

            Divider()

            OverviewToggleRow(
                title: model.text(.overviewHideDockIcon),
                detail: model.text(.overviewHideDockIconDetail),
                accessibilityIdentifier: AppAccessibilityIdentifier.hideDockIconToggle,
                isOn: Binding(
                    get: { appVisibilityController.hidesDockIcon },
                    set: { appVisibilityController.setHidesDockIcon($0) }
                )
            )

            Divider()

            OverviewToggleRow(
                title: model.text(.overviewHideMenuBarIcon),
                detail: model.text(.overviewHideMenuBarIconDetail),
                accessibilityIdentifier: AppAccessibilityIdentifier.hideMenuBarIconToggle,
                isOn: Binding(
                    get: { appVisibilityController.hidesMenuBarIcon },
                    set: { appVisibilityController.setHidesMenuBarIcon($0) }
                )
            )

            Divider()

            HStack(alignment: .center, spacing: 20) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(model.text(.overviewRestartCodexUI))
                        .font(.body.weight(.medium))
                    Text(model.text(.overviewRestartCodexUIDetail))
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }

                Spacer(minLength: 16)

                Button(model.text(.overviewRestartCodexUI), systemImage: "arrow.clockwise.circle") {
                    model.confirmAndRestartCodexUI()
                }
                .buttonStyle(.bordered)
                .accessibilityHint(model.text(.overviewRestartCodexUIDetail))
                .disabled(!model.actions.restartCodexUI)
            }

            Divider()

            HStack(spacing: 10) {
                Button(model.text(.overviewReinject), systemImage: "arrow.clockwise") {
                    model.reinject()
                }
                .disabled(!model.actions.reinject)

                Button(model.text(.overviewManagePackages), systemImage: "shippingbox") {
                    showPackages()
                }

                Spacer()

                Button(model.text(.overviewViewLogs), systemImage: "doc.text") {
                    showLogs()
                }
            }
            .buttonStyle(.bordered)
        }
    }

    private var aiAuthoring: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.controlSpacing)) {
            Text(model.text(.overviewAiAuthoring))
                .font(.title2.weight(.semibold))

            HStack(alignment: .center, spacing: 20) {
                VStack(alignment: .leading, spacing: 5) {
                    Text(model.text(.overviewCopySkill))
                        .font(.body.weight(.medium))
                    Text(model.text(.overviewCopySkillDetail))
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }

                Spacer(minLength: 16)

                Button {
                    model.copyAuthoringPrompt()
                } label: {
                    Label(
                        model.text(
                            model.isAuthoringPromptCopied ? .overviewCopied : .overviewCopy
                        ),
                        systemImage: model.isAuthoringPromptCopied
                            ? "checkmark"
                            : "doc.on.doc"
                    )
                }
                .buttonStyle(.borderedProminent)
                .accessibilityHint(model.text(.overviewCopyHint))
                .disabled(!model.actions.readAuthoringPrompt)
            }
            .padding(CGFloat(model.tokens.cardPadding))
            .background(Color(nsColor: .controlBackgroundColor))
            .clipShape(RoundedRectangle(
                cornerRadius: CGFloat(model.tokens.cardCornerRadius),
                style: .continuous
            ))
            .overlay {
                RoundedRectangle(
                    cornerRadius: CGFloat(model.tokens.cardCornerRadius),
                    style: .continuous
                )
                    .stroke(Color(nsColor: .separatorColor).opacity(0.7), lineWidth: 1)
            }
        }
    }

    private var workflow: some View {
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.controlSpacing)) {
            Text(model.text(.overviewConnection))
                .font(.title2.weight(.semibold))

            Grid(alignment: .leading, horizontalSpacing: 28, verticalSpacing: 12) {
                GridRow {
                    Text(model.text(.overviewCdpEndpoint)).foregroundStyle(.secondary)
                    Text(model.platform.cdpEndpoint)
                        .font(.system(.body, design: .monospaced))
                        .textSelection(.enabled)
                }
                GridRow {
                    Text(model.text(.overviewInjectionScope)).foregroundStyle(.secondary)
                    Text(model.text(.overviewAppPagesOnly))
                }
                GridRow {
                    Text(model.text(.overviewRefreshPolicy)).foregroundStyle(.secondary)
                    Text(model.text(.overviewRefreshEveryTwoSeconds))
                }
                GridRow {
                    Text(model.text(.overviewLoadOrder)).foregroundStyle(.secondary)
                    Text(model.text(.overviewLoadOrderDetail))
                        .font(.system(.callout, design: .monospaced))
                }
                GridRow {
                    Text(model.text(.overviewResources)).foregroundStyle(.secondary)
                    Text(model.tweaksDirectoryPath)
                        .font(.system(.callout, design: .monospaced))
                        .lineLimit(2)
                        .textSelection(.enabled)
                }
            }

            Button(model.text(.overviewOpenPackagesDirectory)) {
                model.openTweaksDirectory()
            }
            .disabled(!model.actions.openPackagesDirectory)
        }
    }

    @ViewBuilder
    private var primaryStatusAction: some View {
        if model.actions.restartCodex {
            Button(model.text(.overviewRestartAndConnect)) {
                model.confirmAndRestartCodex()
            }
            .buttonStyle(.borderedProminent)
        } else if model.status.isCDPAvailable {
            Button(model.text(.overviewOpenCodex)) {
                model.openCodex()
            }
            .buttonStyle(.bordered)
            .disabled(!model.actions.openCodex)
        } else {
            Button(model.text(.overviewOpenCodex)) {
                model.openCodex()
            }
            .buttonStyle(.borderedProminent)
            .disabled(!model.actions.openCodex)
        }
    }

    private var statusTint: Color {
        switch model.statusTone {
        case "success":
            return model.tokens.successColorValue
        case "warning":
            return model.tokens.warningColorValue
        case "danger":
            return model.tokens.dangerColorValue
        case "neutral":
            return .secondary
        default:
            return model.tokens.accentColorValue
        }
    }
}

private struct OverviewToggleRow: View {
    let title: String
    let detail: String
    let accessibilityIdentifier: String
    @Binding var isOn: Bool

    var body: some View {
        HStack(alignment: .center, spacing: 20) {
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.body.weight(.medium))
                Text(detail)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer()
            Toggle(title, isOn: $isOn)
                .labelsHidden()
                .toggleStyle(.switch)
                .accessibilityIdentifier(accessibilityIdentifier)
                .accessibilityLabel(title)
                .accessibilityHint(detail)
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
        VStack(alignment: .leading, spacing: CGFloat(model.tokens.sectionSpacing)) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: CGFloat(model.tokens.compactSpacing)) {
                    Text(model.text(.logsTitle))
                        .font(.largeTitle.weight(.semibold))
                    Text(model.text(.logsSubtitle))
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button(model.text(.logsRefresh), systemImage: "arrow.clockwise") {
                    model.refreshLog()
                }
                .disabled(!model.actions.refreshLog)
                Button(model.text(.logsOpenFile), systemImage: "arrow.up.forward.app") {
                    model.openLog()
                }
                .disabled(!model.actions.openLogFile)
                Button(role: .destructive) {
                    presentedAlert = .confirmClear
                } label: {
                    Label(model.text(.logsClear), systemImage: "trash")
                }
                .disabled(!model.actions.clearLog)
            }

            VStack(spacing: 0) {
                Divider()

                LogTextView(
                    text: model.logText.isEmpty ? model.text(.logsEmpty) : model.logText,
                    contentInset: CGFloat(model.tokens.cardPadding),
                    accessibilityLabel: model.text(.logsTitle)
                )
                .frame(maxWidth: .infinity, maxHeight: .infinity)

                Divider()

                Text(model.logFilePath)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, CGFloat(model.tokens.cardPadding))
                    .padding(.vertical, CGFloat(model.tokens.controlSpacing))
            }
        }
        .frame(
            maxWidth: CGFloat(model.tokens.contentMaxWidth),
            maxHeight: .infinity,
            alignment: .topLeading
        )
        .padding(CGFloat(model.tokens.pagePadding))
        .background(Color(nsColor: .windowBackgroundColor))
        .navigationTitle(model.text(.navLogs))
        .alert(item: $presentedAlert) { alert in
            switch alert {
            case .confirmClear:
                return Alert(
                    title: Text(model.text(.logsClearTitle)),
                    message: Text(model.text(.logsClearMessage)),
                    primaryButton: .destructive(Text(model.text(.logsClear))) {
                        if let message = model.clearLog() {
                            DispatchQueue.main.async {
                                presentedAlert = .clearFailed(message)
                            }
                        }
                    },
                    secondaryButton: .cancel(Text(model.text(.commonCancel)))
                )
            case let .clearFailed(message):
                return Alert(
                    title: Text(model.text(.logsClearFailed)),
                    message: Text(message),
                    dismissButton: .default(Text(model.text(.commonOk)))
                )
            }
        }
        .task { model.refreshLog() }
    }
}

private struct LogTextView: NSViewRepresentable {
    let text: String
    let contentInset: CGFloat
    let accessibilityLabel: String

    func makeNSView(context: Context) -> NSScrollView {
        let scrollView = NSTextView.scrollablePlainDocumentContentTextView()
        scrollView.borderType = .noBorder
        scrollView.drawsBackground = true
        scrollView.backgroundColor = .textBackgroundColor
        scrollView.autohidesScrollers = true

        guard let textView = scrollView.documentView as? NSTextView else {
            return scrollView
        }

        textView.isEditable = false
        textView.isSelectable = true
        textView.drawsBackground = false
        textView.font = .monospacedSystemFont(ofSize: NSFont.systemFontSize, weight: .regular)
        textView.textColor = .textColor
        textView.textContainerInset = NSSize(width: contentInset, height: contentInset)
        textView.usesFindBar = true
        textView.isIncrementalSearchingEnabled = true
        textView.isContinuousSpellCheckingEnabled = false
        textView.isGrammarCheckingEnabled = false
        textView.isAutomaticLinkDetectionEnabled = false
        textView.isAutomaticDataDetectionEnabled = false
        textView.isAutomaticSpellingCorrectionEnabled = false
        textView.isAutomaticTextCompletionEnabled = false
        textView.isAutomaticTextReplacementEnabled = false

        return scrollView
    }

    func updateNSView(_ scrollView: NSScrollView, context: Context) {
        guard let textView = scrollView.documentView as? NSTextView else {
            return
        }

        textView.textContainerInset = NSSize(width: contentInset, height: contentInset)
        textView.setAccessibilityLabel(accessibilityLabel)

        guard textView.string != text else {
            return
        }

        textView.string = text
        textView.scrollRangeToVisible(NSRange(location: 0, length: 0))
    }
}
