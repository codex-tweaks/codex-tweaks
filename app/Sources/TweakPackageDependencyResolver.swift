import Foundation

struct SemanticVersionRequirement: Equatable {
    private enum Rule: Equatable {
        case any
        case exact(SemanticVersion)
        case range(
            lower: SemanticVersion?,
            includesLower: Bool,
            upper: SemanticVersion?,
            includesUpper: Bool
        )
    }

    let rawValue: String
    private let rule: Rule

    init?(_ rawValue: String) {
        let value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !value.isEmpty else { return nil }
        self.rawValue = value

        if value == "*" || value.lowercased() == "latest" {
            rule = .any
            return
        }

        for prefix in [">=", "<=", ">", "<"] {
            guard value.hasPrefix(prefix) else { continue }
            let versionText = String(value.dropFirst(prefix.count))
                .trimmingCharacters(in: .whitespacesAndNewlines)
            guard let version = SemanticVersion(versionText) else { return nil }
            switch prefix {
            case ">=":
                rule = .range(lower: version, includesLower: true, upper: nil, includesUpper: false)
            case ">":
                rule = .range(lower: version, includesLower: false, upper: nil, includesUpper: false)
            case "<=":
                rule = .range(lower: nil, includesLower: false, upper: version, includesUpper: true)
            default:
                rule = .range(lower: nil, includesLower: false, upper: version, includesUpper: false)
            }
            return
        }

        if value.hasPrefix("^") || value.hasPrefix("~") {
            let prefix = value.first!
            let versionText = String(value.dropFirst())
            guard let lower = SemanticVersion(versionText),
                  let components = Self.coreComponents(versionText) else {
                return nil
            }
            let upperText: String
            if prefix == "~" {
                upperText = "\(components.major).\(components.minor + 1).0"
            } else if components.major > 0 {
                upperText = "\(components.major + 1).0.0"
            } else if components.minor > 0 {
                upperText = "0.\(components.minor + 1).0"
            } else {
                upperText = "0.0.\(components.patch + 1)"
            }
            guard let upper = SemanticVersion(upperText) else { return nil }
            rule = .range(
                lower: lower,
                includesLower: true,
                upper: upper,
                includesUpper: false
            )
            return
        }

        if value.lowercased().contains("x") || value.contains("*") {
            guard let range = Self.wildcardRange(value) else { return nil }
            rule = range
            return
        }

        if let version = SemanticVersion(value) {
            rule = .exact(version)
            return
        }

        let shortParts = value.split(separator: ".", omittingEmptySubsequences: false)
        if shortParts.count == 1,
           let major = Int(shortParts[0]),
           let lower = SemanticVersion("\(major).0.0"),
           let upper = SemanticVersion("\(major + 1).0.0") {
            rule = .range(lower: lower, includesLower: true, upper: upper, includesUpper: false)
            return
        }
        if shortParts.count == 2,
           let major = Int(shortParts[0]),
           let minor = Int(shortParts[1]),
           let lower = SemanticVersion("\(major).\(minor).0"),
           let upper = SemanticVersion("\(major).\(minor + 1).0") {
            rule = .range(lower: lower, includesLower: true, upper: upper, includesUpper: false)
            return
        }
        return nil
    }

    func contains(_ rawVersion: String) -> Bool {
        guard let version = SemanticVersion(rawVersion) else { return false }
        switch rule {
        case .any:
            return true
        case let .exact(expected):
            return version == expected
        case let .range(lower, includesLower, upper, includesUpper):
            if let lower {
                if includesLower ? version < lower : version <= lower { return false }
            }
            if let upper {
                if includesUpper ? version > upper : version >= upper { return false }
            }
            return true
        }
    }

    private static func coreComponents(_ value: String) -> (major: Int, minor: Int, patch: Int)? {
        let normalized = SemanticVersion.normalizedString(value)
        let core = normalized.split(separator: "+", maxSplits: 1)[0]
            .split(separator: "-", maxSplits: 1)[0]
            .split(separator: ".", omittingEmptySubsequences: false)
        guard core.count == 3,
              let major = Int(core[0]),
              let minor = Int(core[1]),
              let patch = Int(core[2]) else {
            return nil
        }
        return (major, minor, patch)
    }

    private static func wildcardRange(_ value: String) -> Rule? {
        let parts = value.lowercased().split(separator: ".", omittingEmptySubsequences: false)
        guard (1...3).contains(parts.count) else { return nil }
        let wildcard: (Substring) -> Bool = { $0 == "x" || $0 == "*" }
        if wildcard(parts[0]) { return .any }
        guard let major = Int(parts[0]) else { return nil }
        if parts.count == 1 || wildcard(parts[1]) {
            guard let lower = SemanticVersion("\(major).0.0"),
                  let upper = SemanticVersion("\(major + 1).0.0") else { return nil }
            return .range(lower: lower, includesLower: true, upper: upper, includesUpper: false)
        }
        guard let minor = Int(parts[1]) else { return nil }
        if parts.count == 2 || wildcard(parts[2]) {
            guard let lower = SemanticVersion("\(major).\(minor).0"),
                  let upper = SemanticVersion("\(major).\(minor + 1).0") else { return nil }
            return .range(lower: lower, includesLower: true, upper: upper, includesUpper: false)
        }
        return nil
    }
}

enum TweakPackageDependencyState: Equatable {
    case satisfied
    case missingLocal
    case missingInstallable
    case disabled
    case notBuilt
    case versionMismatch(activeVersion: String)
    case sourceConflict(installedURL: String)
    case cycle
    case blocked
    case invalidRequirement
    case selfReference

    var isSatisfied: Bool {
        self == .satisfied
    }
}

struct TweakPackageDependencyStatus: Identifiable, Equatable {
    let dependentPackageID: String
    let dependencyID: String
    let requirement: String
    let declaredSource: TweakPackageSource?
    let resolvedOrigin: TweakPackageOrigin?
    let installedVersion: String?
    let activeVersion: String?
    let state: TweakPackageDependencyState

    var id: String { dependencyID }

    func withState(_ state: TweakPackageDependencyState) -> Self {
        Self(
            dependentPackageID: dependentPackageID,
            dependencyID: dependencyID,
            requirement: requirement,
            declaredSource: declaredSource,
            resolvedOrigin: resolvedOrigin,
            installedVersion: installedVersion,
            activeVersion: activeVersion,
            state: state
        )
    }

    var issueDescription: String? {
        switch state {
        case .satisfied:
            return nil
        case .missingLocal:
            return "缺少依赖 \(dependencyID)（要求 \(requirement)）；未声明 Git 来源，仅在本地查找。"
        case .missingInstallable:
            return "缺少依赖 \(dependencyID)（要求 \(requirement)），可从声明的 Git 来源安装。"
        case .disabled:
            return "依赖 \(dependencyID) 已停用。"
        case .notBuilt:
            return "依赖 \(dependencyID) 尚未编译。"
        case let .versionMismatch(activeVersion):
            return "依赖 \(dependencyID) 当前激活 v\(activeVersion)，不满足 \(requirement)。"
        case .sourceConflict:
            return "依赖 \(dependencyID) 声明的 Git 来源与本机已安装来源不一致。"
        case .cycle:
            return "功能包依赖形成循环。"
        case .blocked:
            return "依赖 \(dependencyID) 当前不可运行。"
        case .invalidRequirement:
            return "依赖 \(dependencyID) 使用了不支持的版本范围：\(requirement)。"
        case .selfReference:
            return "不能依赖自身。"
        }
    }
}

struct TweakPackagePriorityConstraint: Equatable {
    let packageID: String
    let actualLoadPosition: Int
    let mustLoadAfterPackageIDs: [String]
    let mustLoadBeforePackageIDs: [String]
}

struct TweakPackageDependencyResolution: Equatable {
    let orderedPackages: [TweakPackage]
    let loadablePackages: [TweakPackage]
    let dependenciesByPackageID: [String: [TweakPackageDependencyStatus]]
    let issuesByPackageID: [String: [String]]
    let priorityConstraintsByPackageID: [String: TweakPackagePriorityConstraint]
    let cyclePackageIDs: Set<String>
}

enum TweakPackageDependencyResolver {
    static func resolve(
        packages: [TweakPackage],
        disabledPackageIDs: Set<String>
    ) -> TweakPackageDependencyResolution {
        let validPackages = packages.filter { $0.validationError == nil && $0.manifest != nil }
        let packageByID = Dictionary(uniqueKeysWithValues: validPackages.map { ($0.id, $0) })
        let runtimeEdges = Dictionary(uniqueKeysWithValues: validPackages.map { package in
            (
                package.id,
                Set(package.runtimePackageDependencies.keys.filter { packageByID[$0] != nil })
            )
        })
        let cycleIDs = cyclePackageIDs(edges: runtimeEdges)
        var statuses: [String: [TweakPackageDependencyStatus]] = [:]

        for package in validPackages {
            let requirements = package.runtimePackageDependencies
            var packageStatuses: [TweakPackageDependencyStatus] = []
            for dependencyID in requirements.keys.sorted() {
                let rawRequirement = requirements[dependencyID] ?? ""
                let declaredSource = package.packageDependencies[dependencyID]?.source
                let dependency = packageByID[dependencyID]
                let dependencyOrigin = dependency?.origin
                let installedVersion = dependency?.manifest?.version
                let activeVersion = dependency?.activeBuild?.record.packageVersion
                let state: TweakPackageDependencyState

                if dependencyID == package.id {
                    state = .selfReference
                } else if dependency == nil {
                    state = declaredSource == nil ? .missingLocal : .missingInstallable
                } else if SemanticVersionRequirement(rawRequirement) == nil {
                    state = .invalidRequirement
                } else if cycleIDs.contains(package.id), cycleIDs.contains(dependencyID) {
                    state = .cycle
                } else if let declaredSource,
                          let lock = dependency?.managedLock,
                          !TweakPackageRemoteManager.sourcesMatch(declaredSource, lock.source) {
                    state = .sourceConflict(installedURL: lock.source.url)
                } else if disabledPackageIDs.contains(dependencyID) {
                    state = .disabled
                } else if let dependency, dependency.activeBuild == nil {
                    state = .notBuilt
                } else if let dependency,
                          let activeBuild = dependency.activeBuild,
                          let requirement = SemanticVersionRequirement(rawRequirement),
                          !requirement.contains(activeBuild.record.packageVersion) {
                    state = .versionMismatch(activeVersion: activeBuild.record.packageVersion)
                } else {
                    state = .satisfied
                }

                packageStatuses.append(
                    TweakPackageDependencyStatus(
                        dependentPackageID: package.id,
                        dependencyID: dependencyID,
                        requirement: rawRequirement,
                        declaredSource: declaredSource,
                        resolvedOrigin: dependencyOrigin,
                        installedVersion: installedVersion,
                        activeVersion: activeVersion,
                        state: state
                    )
                )
            }
            if !packageStatuses.isEmpty { statuses[package.id] = packageStatuses }
        }

        var blockedPackageIDs = Set(statuses.compactMap { packageID, packageStatuses in
            packageStatuses.contains { !$0.state.isSatisfied } ? packageID : nil
        })
        blockedPackageIDs.formUnion(cycleIDs)
        var changed = true
        while changed {
            changed = false
            for package in validPackages where !disabledPackageIDs.contains(package.id) {
                guard var packageStatuses = statuses[package.id] else { continue }
                for index in packageStatuses.indices where packageStatuses[index].state.isSatisfied {
                    if blockedPackageIDs.contains(packageStatuses[index].dependencyID) {
                        packageStatuses[index] = packageStatuses[index].withState(.blocked)
                        changed = true
                    }
                }
                statuses[package.id] = packageStatuses
                if packageStatuses.contains(where: { !$0.state.isSatisfied }) {
                    blockedPackageIDs.insert(package.id)
                }
            }
        }

        let issues = statuses.reduce(into: [String: [String]]()) { result, item in
            var seen = Set<String>()
            let messages = item.value.compactMap(\.issueDescription).filter {
                seen.insert($0).inserted
            }
            if !messages.isEmpty { result[item.key] = messages }
        }

        let orderedPackages = topologicallyOrdered(
            packages: packages,
            validPackages: validPackages,
            edges: runtimeEdges
        )
        let priorityConstraints = priorityConstraints(
            packages: validPackages,
            orderedPackages: orderedPackages,
            edges: runtimeEdges,
            cyclePackageIDs: cycleIDs
        )
        let loadablePackages = orderedPackages.filter { package in
            package.validationError == nil
                && package.activeBuild != nil
                && !disabledPackageIDs.contains(package.id)
                && issues[package.id]?.isEmpty != false
        }
        return TweakPackageDependencyResolution(
            orderedPackages: orderedPackages,
            loadablePackages: loadablePackages,
            dependenciesByPackageID: statuses,
            issuesByPackageID: issues,
            priorityConstraintsByPackageID: priorityConstraints,
            cyclePackageIDs: cycleIDs
        )
    }

    private static func priorityConstraints(
        packages: [TweakPackage],
        orderedPackages: [TweakPackage],
        edges: [String: Set<String>],
        cyclePackageIDs: Set<String>
    ) -> [String: TweakPackagePriorityConstraint] {
        let packageByID = Dictionary(uniqueKeysWithValues: packages.map { ($0.id, $0) })
        var positions: [String: Int] = [:]
        for (index, package) in orderedPackages.enumerated() where positions[package.id] == nil {
            positions[package.id] = index + 1
        }
        var dependents: [String: Set<String>] = [:]
        for (packageID, dependencyIDs) in edges {
            for dependencyID in dependencyIDs {
                dependents[dependencyID, default: []].insert(packageID)
            }
        }

        var result: [String: TweakPackagePriorityConstraint] = [:]
        for package in packages where package.priorityOverride != nil {
            guard !cyclePackageIDs.contains(package.id),
                  let actualLoadPosition = positions[package.id] else {
                continue
            }
            let prerequisites = reachablePackageIDs(from: package.id, edges: edges)
            let downstream = reachablePackageIDs(from: package.id, edges: dependents)
            let mustLoadAfter = prerequisites.filter { dependencyID in
                guard let dependency = packageByID[dependencyID] else { return false }
                return package.priority < dependency.priority
            }
            let mustLoadBefore = downstream.filter { dependentID in
                guard let dependent = packageByID[dependentID] else { return false }
                return package.priority > dependent.priority
            }
            guard !mustLoadAfter.isEmpty || !mustLoadBefore.isEmpty else { continue }

            let orderedByPosition: (String, String) -> Bool = {
                (positions[$0] ?? .max) < (positions[$1] ?? .max)
            }
            result[package.id] = TweakPackagePriorityConstraint(
                packageID: package.id,
                actualLoadPosition: actualLoadPosition,
                mustLoadAfterPackageIDs: mustLoadAfter.sorted(by: orderedByPosition),
                mustLoadBeforePackageIDs: mustLoadBefore.sorted(by: orderedByPosition)
            )
        }
        return result
    }

    private static func reachablePackageIDs(
        from packageID: String,
        edges: [String: Set<String>]
    ) -> Set<String> {
        var pending = Array(edges[packageID, default: []])
        var visited = Set<String>()
        while let next = pending.popLast() {
            guard next != packageID, visited.insert(next).inserted else { continue }
            pending.append(contentsOf: edges[next, default: []])
        }
        return visited
    }

    private static func topologicallyOrdered(
        packages: [TweakPackage],
        validPackages: [TweakPackage],
        edges: [String: Set<String>]
    ) -> [TweakPackage] {
        let byID = Dictionary(uniqueKeysWithValues: validPackages.map { ($0.id, $0) })
        var indegree = Dictionary(uniqueKeysWithValues: validPackages.map {
            ($0.id, edges[$0.id]?.count ?? 0)
        })
        var dependents: [String: [String]] = [:]
        for (packageID, dependencyIDs) in edges {
            for dependencyID in dependencyIDs {
                dependents[dependencyID, default: []].append(packageID)
            }
        }

        var ready = validPackages.filter { indegree[$0.id] == 0 }
        var ordered: [TweakPackage] = []
        while !ready.isEmpty {
            ready.sort(by: packageComesFirst)
            let package = ready.removeFirst()
            ordered.append(package)
            for dependentID in dependents[package.id, default: []] {
                indegree[dependentID, default: 0] -= 1
                if indegree[dependentID] == 0, let dependent = byID[dependentID] {
                    ready.append(dependent)
                }
            }
        }

        let orderedIDs = Set(ordered.map(\.id))
        ordered.append(contentsOf: validPackages.filter { !orderedIDs.contains($0.id) }.sorted(by: packageComesFirst))
        ordered.append(contentsOf: packages.filter { $0.validationError != nil }.sorted(by: packageComesFirst))
        return ordered
    }

    private static func cyclePackageIDs(edges: [String: Set<String>]) -> Set<String> {
        enum VisitState: Equatable { case visiting, visited }
        var states: [String: VisitState] = [:]
        var stack: [String] = []
        var cycleIDs = Set<String>()

        func visit(_ packageID: String) {
            if states[packageID] == .visited { return }
            if states[packageID] == .visiting {
                if let start = stack.firstIndex(of: packageID) {
                    cycleIDs.formUnion(stack[start...])
                }
                return
            }
            states[packageID] = .visiting
            stack.append(packageID)
            for dependencyID in edges[packageID, default: []].sorted() {
                visit(dependencyID)
            }
            _ = stack.popLast()
            states[packageID] = .visited
        }

        for packageID in edges.keys.sorted() { visit(packageID) }
        return cycleIDs
    }

    private static func packageComesFirst(_ lhs: TweakPackage, _ rhs: TweakPackage) -> Bool {
        if lhs.priority != rhs.priority { return lhs.priority < rhs.priority }
        return lhs.id.localizedStandardCompare(rhs.id) == .orderedAscending
    }
}
