package core

import "sort"

type DependencyStateKind string

const (
	DependencySatisfied          DependencyStateKind = "satisfied"
	DependencyMissingLocal       DependencyStateKind = "missingLocal"
	DependencyMissingInstallable DependencyStateKind = "missingInstallable"
	DependencyDisabled           DependencyStateKind = "disabled"
	DependencyNotBuilt           DependencyStateKind = "notBuilt"
	DependencyVersionMismatch    DependencyStateKind = "versionMismatch"
	DependencySourceConflict     DependencyStateKind = "sourceConflict"
	DependencyCycle              DependencyStateKind = "cycle"
	DependencyBlocked            DependencyStateKind = "blocked"
	DependencyInvalidRequirement DependencyStateKind = "invalidRequirement"
	DependencySelfReference      DependencyStateKind = "selfReference"
)

type DependencyState struct {
	Kind          DependencyStateKind `json:"kind"`
	ActiveVersion *string             `json:"activeVersion,omitempty"`
	InstalledURL  *string             `json:"installedURL,omitempty"`
}

func (s DependencyState) Satisfied() bool { return s.Kind == DependencySatisfied }

type DependencyStatus struct {
	DependentPackageID string          `json:"dependentPackageID"`
	DependencyID       string          `json:"dependencyID"`
	Requirement        string          `json:"requirement"`
	DeclaredSource     *PackageSource  `json:"declaredSource,omitempty"`
	ResolvedOrigin     *PackageOrigin  `json:"resolvedOrigin,omitempty"`
	InstalledVersion   *string         `json:"installedVersion,omitempty"`
	ActiveVersion      *string         `json:"activeVersion,omitempty"`
	State              DependencyState `json:"state"`
}

func (s DependencyStatus) IssueDescription() string {
	switch s.State.Kind {
	case DependencySatisfied:
		return ""
	case DependencyMissingLocal:
		return "缺少依赖 " + s.DependencyID + "（要求 " + s.Requirement + "）；未声明 Git 来源，仅在本地查找。"
	case DependencyMissingInstallable:
		return "缺少依赖 " + s.DependencyID + "（要求 " + s.Requirement + "），可从声明的 Git 来源安装。"
	case DependencyDisabled:
		return "依赖 " + s.DependencyID + " 已停用。"
	case DependencyNotBuilt:
		return "依赖 " + s.DependencyID + " 尚未编译。"
	case DependencyVersionMismatch:
		version := ""
		if s.State.ActiveVersion != nil {
			version = *s.State.ActiveVersion
		}
		return "依赖 " + s.DependencyID + " 当前激活 v" + version + "，不满足 " + s.Requirement + "。"
	case DependencySourceConflict:
		return "依赖 " + s.DependencyID + " 声明的 Git 来源与本机已安装来源不一致。"
	case DependencyCycle:
		return "功能包依赖形成循环。"
	case DependencyBlocked:
		return "依赖 " + s.DependencyID + " 当前不可运行。"
	case DependencyInvalidRequirement:
		return "依赖 " + s.DependencyID + " 使用了不支持的版本范围：" + s.Requirement + "。"
	case DependencySelfReference:
		return "不能依赖自身。"
	default:
		return ""
	}
}

type PriorityConstraint struct {
	PackageID                string   `json:"packageID"`
	ActualLoadPosition       int      `json:"actualLoadPosition"`
	MustLoadAfterPackageIDs  []string `json:"mustLoadAfterPackageIDs"`
	MustLoadBeforePackageIDs []string `json:"mustLoadBeforePackageIDs"`
}

type DependencyResolution struct {
	OrderedPackages                []Package                     `json:"orderedPackages"`
	LoadablePackages               []Package                     `json:"loadablePackages"`
	DependenciesByPackageID        map[string][]DependencyStatus `json:"dependenciesByPackageID"`
	IssuesByPackageID              map[string][]string           `json:"issuesByPackageID"`
	PriorityConstraintsByPackageID map[string]PriorityConstraint `json:"priorityConstraintsByPackageID"`
	CyclePackageIDs                map[string]bool               `json:"cyclePackageIDs"`
}

func ResolveDependencies(packages []Package, disabledPackageIDs map[string]bool) DependencyResolution {
	validPackages := make([]Package, 0, len(packages))
	packageByID := map[string]Package{}
	for _, pkg := range packages {
		if pkg.ValidationError == nil && pkg.Manifest != nil {
			validPackages = append(validPackages, pkg)
			packageByID[pkg.ID] = pkg
		}
	}

	edges := map[string]map[string]bool{}
	for _, pkg := range validPackages {
		edges[pkg.ID] = map[string]bool{}
		for dependencyID := range pkg.RuntimePackageDependencies() {
			if _, exists := packageByID[dependencyID]; exists {
				edges[pkg.ID][dependencyID] = true
			}
		}
	}
	cycleIDs := cyclePackageIDs(edges)
	statuses := map[string][]DependencyStatus{}
	for _, pkg := range validPackages {
		statuses[pkg.ID] = []DependencyStatus{}
	}

	for _, pkg := range validPackages {
		requirements := pkg.RuntimePackageDependencies()
		for _, dependencyID := range sortedStringKeys(requirements) {
			requirementText := requirements[dependencyID]
			declaredDependency, hasDeclaration := pkg.PackageDependencies()[dependencyID]
			var declaredSource *PackageSource
			if hasDeclaration {
				declaredSource = declaredDependency.Source
			}
			dependency, installed := packageByID[dependencyID]
			status := DependencyStatus{
				DependentPackageID: pkg.ID,
				DependencyID:       dependencyID,
				Requirement:        requirementText,
				DeclaredSource:     declaredSource,
			}
			if installed {
				origin := dependency.Origin
				status.ResolvedOrigin = &origin
				if dependency.Manifest != nil {
					version := dependency.Manifest.Version
					status.InstalledVersion = &version
				}
				if dependency.ActiveBuild != nil {
					version := dependency.ActiveBuild.Record.PackageVersion
					status.ActiveVersion = &version
				}
			}

			switch {
			case dependencyID == pkg.ID:
				status.State = DependencyState{Kind: DependencySelfReference}
			case !installed:
				if declaredSource == nil {
					status.State = DependencyState{Kind: DependencyMissingLocal}
				} else {
					status.State = DependencyState{Kind: DependencyMissingInstallable}
				}
			case !validRequirement(requirementText):
				status.State = DependencyState{Kind: DependencyInvalidRequirement}
			case cycleIDs[pkg.ID] && cycleIDs[dependencyID]:
				status.State = DependencyState{Kind: DependencyCycle}
			case sourceConflicts(declaredSource, dependency.ManagedLock()):
				installedURL := dependency.ManagedLock().Source.URL
				status.State = DependencyState{Kind: DependencySourceConflict, InstalledURL: &installedURL}
			case disabledPackageIDs[dependencyID]:
				status.State = DependencyState{Kind: DependencyDisabled}
			case dependency.ActiveBuild == nil:
				status.State = DependencyState{Kind: DependencyNotBuilt}
			default:
				requirement, _ := ParseVersionRequirement(requirementText)
				activeVersion := dependency.ActiveBuild.Record.PackageVersion
				if !requirement.Contains(activeVersion) {
					status.State = DependencyState{Kind: DependencyVersionMismatch, ActiveVersion: &activeVersion}
				} else {
					status.State = DependencyState{Kind: DependencySatisfied}
				}
			}
			statuses[pkg.ID] = append(statuses[pkg.ID], status)
		}
	}

	blocked := map[string]bool{}
	for packageID, packageStatuses := range statuses {
		if containsUnsatisfied(packageStatuses) {
			blocked[packageID] = true
		}
	}
	for packageID := range cycleIDs {
		blocked[packageID] = true
	}
	for changed := true; changed; {
		changed = false
		for _, pkg := range validPackages {
			if disabledPackageIDs[pkg.ID] {
				continue
			}
			packageStatuses := statuses[pkg.ID]
			for index := range packageStatuses {
				if packageStatuses[index].State.Satisfied() && blocked[packageStatuses[index].DependencyID] {
					packageStatuses[index].State = DependencyState{Kind: DependencyBlocked}
					changed = true
				}
			}
			statuses[pkg.ID] = packageStatuses
			if containsUnsatisfied(packageStatuses) {
				blocked[pkg.ID] = true
			}
		}
	}

	issues := map[string][]string{}
	for packageID, packageStatuses := range statuses {
		seen := map[string]bool{}
		for _, status := range packageStatuses {
			message := status.IssueDescription()
			if message != "" && !seen[message] {
				seen[message] = true
				issues[packageID] = append(issues[packageID], message)
			}
		}
	}

	ordered := topologicallyOrdered(packages, validPackages, edges)
	constraints := priorityConstraints(validPackages, ordered, edges, cycleIDs)
	loadable := make([]Package, 0, len(ordered))
	for _, pkg := range ordered {
		if pkg.ValidationError == nil && pkg.ActiveBuild != nil && !disabledPackageIDs[pkg.ID] && len(issues[pkg.ID]) == 0 {
			loadable = append(loadable, pkg)
		}
	}
	return DependencyResolution{
		OrderedPackages:                ordered,
		LoadablePackages:               loadable,
		DependenciesByPackageID:        statuses,
		IssuesByPackageID:              issues,
		PriorityConstraintsByPackageID: constraints,
		CyclePackageIDs:                cycleIDs,
	}
}

func validRequirement(value string) bool {
	_, ok := ParseVersionRequirement(value)
	return ok
}

func sourceConflicts(source *PackageSource, lock *ManagedPackageLock) bool {
	return source != nil && lock != nil && !SourcesMatch(*source, lock.Source)
}

func containsUnsatisfied(statuses []DependencyStatus) bool {
	for _, status := range statuses {
		if !status.State.Satisfied() {
			return true
		}
	}
	return false
}

func topologicallyOrdered(packages, validPackages []Package, edges map[string]map[string]bool) []Package {
	byID := map[string]Package{}
	indegree := map[string]int{}
	dependents := map[string][]string{}
	for _, pkg := range validPackages {
		byID[pkg.ID] = pkg
		indegree[pkg.ID] = len(edges[pkg.ID])
	}
	for packageID, dependencies := range edges {
		for dependencyID := range dependencies {
			dependents[dependencyID] = append(dependents[dependencyID], packageID)
		}
	}
	ready := []Package{}
	for _, pkg := range validPackages {
		if indegree[pkg.ID] == 0 {
			ready = append(ready, pkg)
		}
	}
	ordered := []Package{}
	for len(ready) > 0 {
		sortPackages(ready)
		pkg := ready[0]
		ready = ready[1:]
		ordered = append(ordered, pkg)
		for _, dependentID := range dependents[pkg.ID] {
			indegree[dependentID]--
			if indegree[dependentID] == 0 {
				ready = append(ready, byID[dependentID])
			}
		}
	}
	orderedIDs := map[string]bool{}
	for _, pkg := range ordered {
		orderedIDs[pkg.ID] = true
	}
	remaining := []Package{}
	for _, pkg := range validPackages {
		if !orderedIDs[pkg.ID] {
			remaining = append(remaining, pkg)
		}
	}
	sortPackages(remaining)
	ordered = append(ordered, remaining...)
	invalid := []Package{}
	for _, pkg := range packages {
		if pkg.ValidationError != nil {
			invalid = append(invalid, pkg)
		}
	}
	sortPackages(invalid)
	return append(ordered, invalid...)
}

func cyclePackageIDs(edges map[string]map[string]bool) map[string]bool {
	const (
		visiting = 1
		visited  = 2
	)
	states := map[string]int{}
	stack := []string{}
	cycles := map[string]bool{}
	var visit func(string)
	visit = func(packageID string) {
		if states[packageID] == visited {
			return
		}
		if states[packageID] == visiting {
			for index, stackedID := range stack {
				if stackedID == packageID {
					for _, cycleID := range stack[index:] {
						cycles[cycleID] = true
					}
					break
				}
			}
			return
		}
		states[packageID] = visiting
		stack = append(stack, packageID)
		for _, dependencyID := range sortedSetKeys(edges[packageID]) {
			visit(dependencyID)
		}
		stack = stack[:len(stack)-1]
		states[packageID] = visited
	}
	for _, packageID := range sortedNestedMapKeys(edges) {
		visit(packageID)
	}
	return cycles
}

func priorityConstraints(packages, ordered []Package, edges map[string]map[string]bool, cycleIDs map[string]bool) map[string]PriorityConstraint {
	byID := map[string]Package{}
	positions := map[string]int{}
	dependents := map[string]map[string]bool{}
	for _, pkg := range packages {
		byID[pkg.ID] = pkg
	}
	for index, pkg := range ordered {
		if positions[pkg.ID] == 0 {
			positions[pkg.ID] = index + 1
		}
	}
	for packageID, dependencies := range edges {
		for dependencyID := range dependencies {
			if dependents[dependencyID] == nil {
				dependents[dependencyID] = map[string]bool{}
			}
			dependents[dependencyID][packageID] = true
		}
	}
	result := map[string]PriorityConstraint{}
	for _, pkg := range packages {
		if pkg.PriorityOverride == nil || cycleIDs[pkg.ID] || positions[pkg.ID] == 0 {
			continue
		}
		after, before := []string{}, []string{}
		for dependencyID := range reachablePackageIDs(pkg.ID, edges) {
			if dependency, exists := byID[dependencyID]; exists && pkg.Priority() < dependency.Priority() {
				after = append(after, dependencyID)
			}
		}
		for dependentID := range reachablePackageIDs(pkg.ID, dependents) {
			if dependent, exists := byID[dependentID]; exists && pkg.Priority() > dependent.Priority() {
				before = append(before, dependentID)
			}
		}
		if len(after) == 0 && len(before) == 0 {
			continue
		}
		sortByPosition(after, positions)
		sortByPosition(before, positions)
		result[pkg.ID] = PriorityConstraint{
			PackageID:                pkg.ID,
			ActualLoadPosition:       positions[pkg.ID],
			MustLoadAfterPackageIDs:  after,
			MustLoadBeforePackageIDs: before,
		}
	}
	return result
}

func reachablePackageIDs(packageID string, edges map[string]map[string]bool) map[string]bool {
	pending := sortedSetKeys(edges[packageID])
	visited := map[string]bool{}
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if next == packageID || visited[next] {
			continue
		}
		visited[next] = true
		pending = append(pending, sortedSetKeys(edges[next])...)
	}
	return visited
}

func sortPackages(packages []Package) {
	sort.SliceStable(packages, func(left, right int) bool {
		if packages[left].Priority() != packages[right].Priority() {
			return packages[left].Priority() < packages[right].Priority()
		}
		return packages[left].ID < packages[right].ID
	})
}

func sortByPosition(values []string, positions map[string]int) {
	sort.SliceStable(values, func(left, right int) bool {
		leftPosition, rightPosition := positions[values[left]], positions[values[right]]
		if leftPosition != rightPosition {
			return leftPosition < rightPosition
		}
		return values[left] < values[right]
	})
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSetKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, included := range values {
		if included {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func sortedNestedMapKeys(values map[string]map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
