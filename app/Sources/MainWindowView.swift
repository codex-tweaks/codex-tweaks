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

        var title: String {
            switch self {
            case .overview:
                return "概览"
            case .packages:
                return "功能包"
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
                    showPackages: { selection = .packages },
                    showLogs: { selection = .logs }
                )
            case .packages:
                TweakPackagesView(model: model)
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

private struct TweakPackagesView: View {
    private enum PackageFilter: String, CaseIterable, Identifiable {
        case all
        case enabled
        case disabled
        case pending
        case error

        var id: Self { self }

        var title: String {
            switch self {
            case .all: return "全部"
            case .enabled: return "已启用"
            case .disabled: return "已停用"
            case .pending: return "待处理"
            case .error: return "异常"
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
        List {
            header
                .listRowInsets(EdgeInsets())
                .listRowBackground(Color.clear)

            packageListToolbar
                .listRowInsets(EdgeInsets())
                .listRowBackground(Color.clear)

            if model.tweakPackages.isEmpty {
                emptyState
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
            } else if filteredPackages.isEmpty {
                filteredEmptyState
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
            } else {
                ForEach(filteredPackages) { package in
                    packageRow(package)
                        .listRowInsets(
                            EdgeInsets(top: 0, leading: 28, bottom: 0, trailing: 28)
                        )
                }
            }
        }
        .listStyle(.inset)
        .scrollContentBackground(.hidden)
        .background(Color(nsColor: .windowBackgroundColor))
        .navigationTitle("功能包")
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
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 6) {
                Text("按包管理页面增强")
                    .font(.largeTitle.weight(.semibold))
                Text("每个目录是一个独立包。源码更新不会直接生效，手动编译成功后才会原子切换。")
                    .foregroundStyle(.secondary)
            }

            HStack(spacing: 14) {
                Label(
                    "已启用 \(model.enabledTweakPackageCount) / \(model.tweakPackages.count)，已激活 \(model.activeTweakPackageCount)",
                    systemImage: "shippingbox"
                )
                .font(.callout.weight(.medium))

                Spacer()

                Toggle("开发者模式", isOn: $model.isDeveloperMode)
                    .toggleStyle(.switch)

                Button("重新扫描", systemImage: "arrow.clockwise") {
                    model.reloadTweakPackages()
                    model.checkNodeEnvironment()
                    model.checkGitEnvironment()
                }
                .buttonStyle(.bordered)
            }

            HStack(spacing: 8) {
                Image(systemName: model.nodeEnvironment == nil ? "exclamationmark.triangle" : "checkmark.circle")
                    .foregroundStyle(model.nodeEnvironment == nil ? .orange : .green)
                if model.isCheckingNode {
                    Text("正在检测 Node.js…")
                } else if let node = model.nodeEnvironment {
                    Text("Node.js \(node.version) 可用")
                    Text(node.nodeURL.path)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                } else {
                    Text("未找到 Node.js、npm 和 npx；安装 Node.js 后重新扫描。")
                        .foregroundStyle(.secondary)
                }
            }
            .font(.callout)

            HStack(spacing: 8) {
                Image(systemName: model.gitEnvironment == nil ? "exclamationmark.triangle" : "checkmark.circle")
                    .foregroundStyle(model.gitEnvironment == nil ? .orange : .green)
                if model.isCheckingGit {
                    Text("正在检测 Git…")
                } else if let git = model.gitEnvironment {
                    Text("\(git.version) 可用")
                    Text(git.gitURL.path)
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                } else {
                    Text("未找到 Git；本地功能包仍可使用，但不能安装或检查远程包。")
                        .foregroundStyle(.secondary)
                }

                Spacer(minLength: 12)

                Button(
                    model.isInstallingLocalPackage ? "正在安装…" : "从本地安装",
                    systemImage: "folder.badge.plus"
                ) {
                    model.clearLocalOperationFeedback()
                    isShowingLocalPackageImporter = true
                }
                .buttonStyle(.borderedProminent)
                .disabled(
                    model.isInstallingLocalPackage || model.isInstallingRemotePackage
                )
                .help("选择功能包目录或 ZIP；校验通过后复制到本地 packages 目录")

                Button("从 Git 安装", systemImage: "square.and.arrow.down") {
                    isShowingGitInstall = true
                }
                .buttonStyle(.bordered)
                .disabled(
                    model.gitEnvironment == nil
                        || model.isInstallingLocalPackage
                        || model.isInstallingRemotePackage
                )

                Button(
                    model.isCheckingRemoteUpdates ? "检查中…" : "检查包更新",
                    systemImage: "arrow.triangle.2.circlepath"
                ) {
                    model.checkManagedPackageUpdates()
                }
                .buttonStyle(.bordered)
                .disabled(model.gitEnvironment == nil || model.isCheckingRemoteUpdates)
            }
            .font(.callout)

            localInstallFeedback

            if model.isDeveloperMode {
                Text("开发者模式会自动编译已启用包的源码变化，仅使用本地已有的依赖和编译器缓存；依赖或版本更新仍需手动确认。")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }

        }
        .padding(.horizontal, 28)
        .padding(.vertical, 24)
    }

    @ViewBuilder
    private var localInstallFeedback: some View {
        if model.isInstallingLocalPackage {
            HStack(spacing: 8) {
                ProgressView()
                    .controlSize(.small)
                Text("正在安全检查并安装本地功能包…")
            }
            .font(.callout)
            .foregroundStyle(.secondary)
        } else if let message = model.localOperationMessage {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Label(message, systemImage: "checkmark.circle.fill")
                    .foregroundStyle(.green)
                    .textSelection(.enabled)
                Spacer(minLength: 8)
                Button {
                    model.clearLocalOperationFeedback()
                } label: {
                    Image(systemName: "xmark")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .accessibilityLabel("清除本地安装提示")
            }
            .font(.callout)
        } else if let error = model.localOperationError {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.red)
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
                .accessibilityLabel("清除本地安装错误")
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
            Label("加载顺序", systemImage: "list.number")

            Text("依赖拓扑优先，其余按有效优先级")
                .font(.caption)
                .foregroundStyle(.secondary)

            Spacer(minLength: 12)

            if isFilteringPackages {
                Text("\(filteredPackages.count) / \(model.tweakPackages.count)")
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(.secondary)
            }

            TextField("搜索功能包", text: $searchText)
                .textFieldStyle(.roundedBorder)
                .frame(minWidth: 160, idealWidth: 210, maxWidth: 240)

            Picker("筛选功能包", selection: $selectedFilter) {
                ForEach(PackageFilter.allCases) { filter in
                    Text(filter.title).tag(filter)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .frame(width: 108)
        }
        .padding(.horizontal, 28)
        .padding(.vertical, 10)
    }

    private func packageRow(_ package: TweakPackage) -> some View {
        HStack(alignment: .top, spacing: 14) {
            Toggle(
                "启用 \(package.displayName)",
                isOn: Binding(
                    get: { model.isTweakPackageEnabled(package) },
                    set: { model.setTweakPackage(package, isEnabled: $0) }
                )
            )
            .labelsHidden()
            .toggleStyle(.switch)
            .padding(.top, 3)

            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 8) {
                    Text(package.displayName)
                        .font(.body.weight(.semibold))
                    Text("源 v\(package.version)")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.secondary)
                    if let active = package.activeBuild {
                        Text("已激活 v\(active.record.packageVersion)")
                            .font(.system(.caption, design: .monospaced))
                            .foregroundStyle(.secondary)
                    }
                }
                Text(package.detail)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
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
                    if model.canInstallMissingDependencies(for: package) {
                        Button("安装缺失依赖") {
                            model.installMissingDependencies(for: package)
                        }
                        .disabled(
                            model.gitEnvironment == nil
                                || model.installingPackageIDs.contains(package.id)
                        )
                    }

                    if model.canEnableDependencies(for: package) {
                        Button("启用依赖") {
                            model.enableDependencies(for: package)
                        }
                    }

                    if model.remotePackageUpdates[package.id]?.isInstallable == true {
                        Button("更新并编译") {
                            model.updateManagedPackage(package)
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(model.installingPackageIDs.contains(package.id))
                    }

                    Button {
                        model.openPackageDirectory(package)
                    } label: {
                        Image(systemName: "folder")
                    }
                    .help("在 Finder 中打开功能包")

                    Button(buildButtonTitle(package)) {
                        model.buildPackage(package)
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(
                        package.validationError != nil
                            || model.nodeEnvironment == nil
                            || model.buildingPackageIDs.contains(package.id)
                    )
                }
            }
        }
        .padding(.vertical, 8)
        .accessibilityElement(children: .contain)
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
                    expandedDependencyPackageIDs.contains(package.id) ? "已展开" : "已折叠"
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
                    Text("未声明 Git 来源，仅在本地查找")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }

                if case let .sourceConflict(installedURL) = status.state {
                    Text("本机已安装来源：\(installedURL)")
                        .font(.caption2)
                        .foregroundStyle(.red)
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
            return "依赖 \(statuses.count)/\(statuses.count) 正常"
        }
        if statuses.contains(where: { dependencyStateIsCritical($0.state) }) {
            return "依赖 \(statuses.count) · \(pendingCount) 个异常"
        }
        return "依赖 \(statuses.count) · \(pendingCount) 个需处理"
    }

    private func dependencySummaryColor(_ statuses: [TweakPackageDependencyStatus]) -> Color {
        if statuses.allSatisfy(\.state.isSatisfied) { return .green }
        return statuses.contains(where: { dependencyStateIsCritical($0.state) }) ? .red : .orange
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
            return status.activeVersion.map { "正常 · v\($0)" } ?? "正常"
        case .missingLocal:
            return "本地缺失"
        case .missingInstallable:
            return "可从 Git 安装"
        case .disabled:
            return status.activeVersion.map { "已停用 · v\($0)" } ?? "已停用"
        case .notBuilt:
            return status.installedVersion.map { "尚未编译 · v\($0)" } ?? "尚未编译"
        case let .versionMismatch(activeVersion):
            return "v\(activeVersion) 不匹配"
        case .sourceConflict:
            return "来源冲突"
        case .cycle:
            return "循环依赖"
        case .blocked:
            return "依赖链阻塞"
        case .invalidRequirement:
            return "版本范围无效"
        case .selfReference:
            return "依赖自身"
        }
    }

    private func dependencyStateColor(_ state: TweakPackageDependencyState) -> Color {
        if state.isSatisfied { return .green }
        return dependencyStateIsCritical(state) ? .red : .orange
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
                return "本地提供"
            case .managed:
                return "Git 管理"
            }
        }
        return status.declaredSource == nil ? "仅本地" : "Git 可安装"
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
        if let value = selector.value { return "\(selector.type.title)：\(value)" }
        return selector.type.title
    }

    private func priorityEditor(_ package: TweakPackage) -> some View {
        HStack(spacing: 4) {
            Text("优先级")
                .foregroundStyle(.tertiary)
            TextField(
                "优先级",
                value: Binding(
                    get: { package.priority },
                    set: { model.setTweakPackagePriority(package, priority: $0) }
                ),
                format: .number
            )
            .labelsHidden()
            .textFieldStyle(.roundedBorder)
            .frame(width: 58)
            .accessibilityLabel("\(package.displayName) 用户优先级")
            if package.priorityOverride != nil {
                Button {
                    model.resetTweakPackagePriority(package)
                } label: {
                    Image(systemName: "arrow.uturn.backward.circle")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.secondary)
                .help("恢复包默认优先级 \(package.declaredPriority)")
            }
            if let constraint = model.priorityConstraint(for: package) {
                Button {
                    priorityHintPackageID = package.id
                } label: {
                    Label("依赖约束", systemImage: "info.circle.fill")
                }
                .buttonStyle(.plain)
                .foregroundStyle(.blue)
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
            Label("依赖顺序优先", systemImage: "point.3.connected.trianglepath.dotted")
                .font(.headline)
            Text(priorityConstraintDescription(package, constraint: constraint))
                .fixedSize(horizontal: false, vertical: true)
            Text(
                "实际加载顺序 #\(constraint.actualLoadPosition)。用户优先级 \(package.priority) "
                    + "仍用于和没有依赖路径的功能包排序。"
            )
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
            messages.append(
                "\(package.displayName) 依赖于 "
                    + packageNames(constraint.mustLoadAfterPackageIDs)
                    + "，因此必须在这些包之后加载。"
            )
        }
        if !constraint.mustLoadBeforePackageIDs.isEmpty {
            messages.append(
                packageNames(constraint.mustLoadBeforePackageIDs)
                    + " 依赖于 \(package.displayName)，因此当前包必须先于这些包加载。"
            )
        }
        return messages.joined(separator: " ")
    }

    private func packageNames(_ packageIDs: [String]) -> String {
        let names = packageIDs.prefix(3).map { packageID in
            model.tweakPackages.first(where: { $0.id == packageID })?.displayName ?? packageID
        }
        let suffix = packageIDs.count > names.count ? " 等 \(packageIDs.count) 个包" : ""
        return names.joined(separator: "、") + suffix
    }

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "shippingbox")
                .font(.system(size: 36, weight: .light))
                .foregroundStyle(.secondary)
            Text("没有找到功能包")
                .font(.title3.weight(.medium))
            Text("在 packages 目录中创建包目录和 package.json 后重新扫描。")
                .foregroundStyle(.secondary)
            Button("打开 packages 目录") {
                model.openTweaksDirectory()
            }
        }
        .frame(maxWidth: .infinity, minHeight: 260)
        .padding(.horizontal, 32)
        .padding(.vertical, 48)
    }

    private var filteredEmptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "line.3.horizontal.decrease.circle")
                .font(.system(size: 32, weight: .light))
                .foregroundStyle(.secondary)
            Text("没有匹配的功能包")
                .font(.title3.weight(.medium))
            Text("尝试更换关键词或筛选条件。")
                .foregroundStyle(.secondary)
            Button("清除搜索与筛选") {
                searchText = ""
                selectedFilter = .all
            }
        }
        .frame(maxWidth: .infinity, minHeight: 220)
        .padding(.horizontal, 32)
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
            if let status = model.remotePackageUpdates[package.id]?.status,
               status != .current {
                return true
            }
            if !model.dependencyIssues(for: package).isEmpty {
                return true
            }
            switch package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion) {
            case .notBuilt, .versionUpdate, .dependencyUpdate, .sourceChanged, .compilerUpdate:
                return true
            case .invalid, .current:
                return false
            }
        case .error:
            return package.validationError != nil
                || model.packageBuildErrors[package.id] != nil
                || model.packagePayloadErrors[package.id] != nil
                || model.packageRuntimeErrors[package.id] != nil
                || model.remotePackageErrors[package.id] != nil
                || hasCriticalDependencyIssue(package)
        }
    }

    private func packageStatusTitle(_ package: TweakPackage) -> String {
        if model.installingPackageIDs.contains(package.id) { return "正在处理远程包" }
        if model.buildingPackageIDs.contains(package.id) { return "正在编译" }
        if package.validationError != nil { return "包配置无效" }
        if model.remotePackageErrors[package.id] != nil { return "远程操作失败" }
        if model.remotePackageUpdates[package.id]?.status == .pinnedReferenceChanged {
            return "固定引用已变化"
        }
        if model.remotePackageUpdates[package.id]?.status == .available {
            return "远程有更新"
        }
        if !model.dependencyIssues(for: package).isEmpty { return "依赖阻塞" }
        if model.packageBuildErrors[package.id] != nil { return "编译失败" }
        if model.packagePayloadErrors[package.id] != nil { return "产物读取失败" }
        if model.packageRuntimeErrors[package.id] != nil { return "运行失败" }
        switch package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion) {
        case .invalid: return "不可用"
        case .notBuilt: return "尚未编译"
        case .current: return "已激活"
        case .versionUpdate: return "发现新版本"
        case .dependencyUpdate: return "依赖或配置更新"
        case .sourceChanged: return "源码有更新"
        case .compilerUpdate: return "编译器有更新"
        }
    }

    private func packageStatusDetail(_ package: TweakPackage) -> String? {
        if let error = package.validationError { return error }
        if let error = model.remotePackageErrors[package.id] { return error }
        if let update = model.remotePackageUpdates[package.id] {
            switch update.status {
            case .available:
                return "\(update.candidateReference) 已更新到 \(update.candidateCommit.prefix(12))，点击后下载并编译。"
            case .pinnedReferenceChanged:
                return "远端固定 Tag/Release 指向了新的 commit；为避免静默替换，已阻止普通更新。"
            case .current:
                break
            }
        }
        let dependencyIssues = model.dependencyIssues(for: package)
        if !dependencyIssues.isEmpty {
            return "有 \(dependencyIssues.count) 个依赖问题，展开依赖详情可查看具体状态。"
        }
        if let error = model.packageBuildErrors[package.id] {
            if let active = package.activeBuild {
                return "当前仍运行 v\(active.record.packageVersion)。\(error)"
            }
            return error
        }
        if let error = model.packagePayloadErrors[package.id] { return error }
        if let error = model.packageRuntimeErrors[package.id] { return error }
        guard let active = package.activeBuild else { return "点击编译后才会加载到页面。" }
        switch package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion) {
        case .versionUpdate:
            return "当前仍运行 v\(active.record.packageVersion)，点击后更新到 v\(package.version)。"
        case .dependencyUpdate:
            return "当前编译产物仍在运行；需手动同步依赖或构建配置。"
        case .sourceChanged:
            return "当前编译产物仍在运行，新源码尚未激活。"
        case .compilerUpdate:
            return "当前产物由 esbuild \(active.record.compilerVersion) 生成。"
        default:
            return "上次编译：\(active.record.builtAt.formatted(date: .abbreviated, time: .shortened))"
        }
    }

    private func packageStatusColor(_ package: TweakPackage) -> Color {
        if package.validationError != nil
            || model.packageBuildErrors[package.id] != nil
            || model.packagePayloadErrors[package.id] != nil
            || model.packageRuntimeErrors[package.id] != nil {
            return .red
        }
        if model.remotePackageErrors[package.id] != nil { return .red }
        if model.remotePackageUpdates[package.id]?.status == .pinnedReferenceChanged {
            return .orange
        }
        if hasCriticalDependencyIssue(package) { return .red }
        if !model.dependencyIssues(for: package).isEmpty { return .orange }
        if model.installingPackageIDs.contains(package.id) { return .blue }
        if model.buildingPackageIDs.contains(package.id) { return .blue }
        return package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion) == .current
            ? .green
            : .orange
    }

    private func hasCriticalDependencyIssue(_ package: TweakPackage) -> Bool {
        model.dependencyStatuses(for: package).contains {
            dependencyStateIsCritical($0.state)
        }
    }

    private func packageStatusSymbol(_ package: TweakPackage) -> String {
        if model.installingPackageIDs.contains(package.id) { return "arrow.down.circle" }
        if model.buildingPackageIDs.contains(package.id) { return "arrow.triangle.2.circlepath" }
        if packageStatusColor(package) == .red { return "exclamationmark.triangle.fill" }
        return package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion) == .current
            ? "checkmark.circle.fill"
            : "clock.badge.exclamationmark"
    }

    private func buildButtonTitle(_ package: TweakPackage) -> String {
        if model.buildingPackageIDs.contains(package.id) { return "编译中…" }
        switch package.buildDisposition(compilerVersion: TweakPackageStore.compilerVersion) {
        case .notBuilt:
            return package.hasDependencies ? "安装并编译" : "编译"
        case .versionUpdate:
            return "更新到 v\(package.version)"
        case .dependencyUpdate:
            return "同步并编译"
        case .sourceChanged, .compilerUpdate:
            return "更新编译"
        case .current:
            return "重新编译"
        case .invalid:
            return "无法编译"
        }
    }
}

private struct GitPackageInstallView: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: AppModel
    @State private var repositoryURL = ""
    @State private var selectorType: TweakPackageRemoteSelectorType = .branch
    @State private var selectorValue = TweakPackageRemoteSelectorType.branch.defaultValue

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            VStack(alignment: .leading, spacing: 6) {
                Text("从 Git 安装功能包")
                    .font(.title2.weight(.semibold))
                Text("程序会解析远程引用、锁定精确 commit，并在临时目录校验包格式后保存不可变源码。")
                    .foregroundStyle(.secondary)
            }

            Form {
                TextField("仓库地址", text: $repositoryURL, prompt: Text("https://github.com/owner/package.git"))

                Picker("版本选择", selection: $selectorType) {
                    ForEach(TweakPackageRemoteSelectorType.allCases) { type in
                        Text(type.title).tag(type)
                    }
                }

                if let valueLabel = selectorType.valueLabel {
                    TextField(valueLabel, text: $selectorValue)
                }
            }
            .formStyle(.grouped)

            Text("安装会校验 package.json、API 版本、SemVer、入口、包依赖与 npm 锁文件。通过后新包默认保持停用；有 Node.js 时会继续下载锁定依赖并编译，但不会自动启用。")
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

            if let message = model.remoteOperationMessage {
                Label(message, systemImage: "checkmark.circle.fill")
                    .foregroundStyle(.green)
                    .textSelection(.enabled)
            }
            if let error = model.remoteOperationError {
                Label(error, systemImage: "exclamationmark.triangle.fill")
                    .foregroundStyle(.red)
                    .textSelection(.enabled)
            }

            HStack {
                Spacer()
                Button("关闭") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button(model.isInstallingRemotePackage ? "正在安装…" : "安装") {
                    model.installRemotePackage(
                        repositoryURL: repositoryURL,
                        selectorType: selectorType,
                        selectorValue: selectorValue
                    )
                }
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.defaultAction)
                .disabled(!canInstall || model.isInstallingRemotePackage)
            }
        }
        .padding(24)
        .frame(width: 620)
        .frame(minHeight: 420)
        .onAppear { model.clearRemoteOperationFeedback() }
        .onChange(of: selectorType) { type in
            selectorValue = type.defaultValue
        }
    }

    private var canInstall: Bool {
        !repositoryURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && (selectorType.valueLabel == nil
                || !selectorValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            && model.gitEnvironment != nil
    }
}

private struct OverviewView: View {
    @ObservedObject var model: AppModel
    let showPackages: () -> Void
    let showLogs: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 28) {
                header
                statusSurface
                controls
                aiAuthoring
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

                Button("管理功能包", systemImage: "shippingbox") {
                    showPackages()
                }

                Spacer()

                Button("查看日志", systemImage: "doc.text") {
                    showLogs()
                }
            }
            .buttonStyle(.bordered)
        }
    }

    private var aiAuthoring: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("交给 AI 编写")
                .font(.title2.weight(.semibold))

            HStack(alignment: .center, spacing: 20) {
                VStack(alignment: .leading, spacing: 5) {
                    Text("复制 Codex Tweaks 功能包开发 Skill")
                        .font(.body.weight(.medium))
                    Text("复制内容直接来自项目内的 SKILL.md，与功能包协议、依赖和验证约束始终使用同一份来源。粘贴后再追加你的具体需求。")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }

                Spacer(minLength: 16)

                Button {
                    model.copyAuthoringPrompt()
                } label: {
                    Label(
                        model.isAuthoringPromptCopied ? "已复制" : "复制 Skill",
                        systemImage: model.isAuthoringPromptCopied
                            ? "checkmark"
                            : "doc.on.doc"
                    )
                }
                .buttonStyle(.borderedProminent)
                .accessibilityHint("复制后可粘贴给 Codex 或其他智能体")
            }
            .padding(20)
            .background(Color(nsColor: .controlBackgroundColor))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay {
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(Color(nsColor: .separatorColor).opacity(0.7), lineWidth: 1)
            }
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
                    Text("每 2 秒检查功能包与窗口")
                }
                GridRow {
                    Text("包加载顺序").foregroundStyle(.secondary)
                    Text("依赖拓扑优先，再按用户有效优先级从小到大")
                        .font(.system(.callout, design: .monospaced))
                }
                GridRow {
                    Text("资源目录").foregroundStyle(.secondary)
                    Text(model.tweaksDirectoryPath)
                        .font(.system(.callout, design: .monospaced))
                        .lineLimit(2)
                        .textSelection(.enabled)
                }
            }

            Button("在 Finder 中管理功能包") {
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
            return "已编译且启用的功能包已应用到 Codex。"
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
