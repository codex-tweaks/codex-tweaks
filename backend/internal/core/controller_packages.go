package core

import (
	"errors"
	"sort"
)

func (c *Controller) updatePackages() error {
	packages, err := c.store.LoadPackages()
	if err != nil {
		return err
	}
	discovered := map[string]bool{}
	for _, pkg := range packages {
		if pkg.ValidationError == nil && pkg.Manifest != nil {
			discovered[pkg.Manifest.Name] = true
		}
	}

	c.mu.Lock()
	known := map[string]bool{}
	hasKnownBaseline := c.config.KnownPackageIDs != nil
	if c.config.KnownPackageIDs != nil {
		known = stringSet(*c.config.KnownPackageIDs)
	}
	reconciliation := ReconcileEnablement(discovered, known, c.disabledPackageIDs, hasKnownBaseline)
	newIDs := sortedTrueKeys(reconciliation.NewlyDiscoveredPackageIDs)
	knownIDs := sortedTrueKeys(reconciliation.KnownPackageIDs)
	c.config.KnownPackageIDs = &knownIDs
	c.disabledPackageIDs = reconciliation.DisabledPackageIDs
	resolution := ResolveDependencies(packages, c.disabledPackageIDs)
	c.packageDependencyStatuses = resolution.DependenciesByPackageID
	c.packageDependencyIssues = resolution.IssuesByPackageID
	c.packagePriorityConstraints = resolution.PriorityConstraintsByPackageID
	c.packages = resolution.OrderedPackages
	currentRequestKeys := map[string]string{}
	for _, pkg := range resolution.OrderedPackages {
		if key, ok := pkg.BuildRequestKey(CompilerVersion); ok {
			currentRequestKeys[pkg.ID] = key
		}
	}
	for packageID := range c.packageBuildErrors {
		if c.packageBuildErrorRequestKeys[packageID] != currentRequestKeys[packageID] {
			delete(c.packageBuildErrors, packageID)
			delete(c.packageBuildErrorRequestKeys, packageID)
		}
	}
	err = c.persistConfigurationLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	for _, packageID := range newIDs {
		c.logger.Info("发现新功能包，默认保持停用：" + packageID)
	}
	return nil
}

func (c *Controller) packageByID(packageID string) (Package, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, pkg := range c.packages {
		if pkg.ID == packageID {
			return pkg, true
		}
	}
	return Package{}, false
}

func (c *Controller) SetEnabled(enabled bool) error {
	c.mu.Lock()
	if c.config.Enabled == enabled {
		c.mu.Unlock()
		return nil
	}
	c.config.Enabled = enabled
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if enabled {
		c.logger.Info("界面增强已启用")
	} else {
		c.logger.Info("界面增强已停用")
	}
	c.emit()
	go c.Refresh()
	return nil
}

func (c *Controller) SetDeveloperMode(enabled bool) error {
	c.mu.Lock()
	if c.config.DeveloperMode == enabled {
		c.mu.Unlock()
		return nil
	}
	c.config.DeveloperMode = enabled
	c.developerBuildAttemptKeys = map[string]string{}
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if enabled {
		c.logger.Info("开发者模式已启用")
	} else {
		c.logger.Info("开发者模式已停用")
	}
	c.emit()
	if enabled {
		c.scheduleDeveloperBuilds()
	}
	return nil
}

func (c *Controller) SetPackageEnabled(packageID string, enabled bool) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	c.mu.Lock()
	changed := false
	if enabled && c.disabledPackageIDs[packageID] {
		delete(c.disabledPackageIDs, packageID)
		changed = true
	} else if !enabled && !c.disabledPackageIDs[packageID] {
		c.disabledPackageIDs[packageID] = true
		changed = true
	}
	if !changed {
		c.mu.Unlock()
		return nil
	}
	delete(c.packageRuntimeErrors, packageID)
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if enabled {
		c.logger.Info("已启用功能包：" + pkg.DisplayName())
		c.scheduleDeveloperBuilds()
	} else {
		c.logger.Info("已停用功能包：" + pkg.DisplayName())
	}
	if err := c.updatePackages(); err != nil {
		return err
	}
	c.emit()
	go c.Refresh()
	return nil
}

func (c *Controller) SetPackagePriority(packageID string, priority *int) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	if priority != nil && *priority == pkg.DeclaredPriority() {
		priority = nil
	}
	if err := c.store.SetPriorityOverride(packageID, priority); err != nil {
		return err
	}
	if err := c.updatePackages(); err != nil {
		return err
	}
	c.mu.Lock()
	c.forceGeneration++
	c.mu.Unlock()
	if priority == nil {
		c.logger.Info("已恢复功能包 " + pkg.DisplayName() + " 的默认优先级")
	} else {
		c.logger.Info("已将功能包 " + pkg.DisplayName() + " 的用户优先级设为 " + intString(*priority))
	}
	c.emit()
	go c.Refresh()
	return nil
}

func (c *Controller) EnableDependencies(packageID string) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	c.mu.Lock()
	packageByID := map[string]Package{}
	for _, item := range c.packages {
		packageByID[item.ID] = item
	}
	pending := sortedStringKeys(pkg.RuntimePackageDependencies())
	enabledIDs := map[string]bool{}
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if enabledIDs[next] {
			continue
		}
		enabledIDs[next] = true
		if dependency, found := packageByID[next]; found {
			pending = append(pending, sortedStringKeys(dependency.RuntimePackageDependencies())...)
		}
	}
	for dependencyID := range enabledIDs {
		delete(c.disabledPackageIDs, dependencyID)
	}
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if err := c.updatePackages(); err != nil {
		return err
	}
	c.logger.Info("已启用 " + pkg.DisplayName() + " 的依赖链")
	c.emit()
	go c.Refresh()
	return nil
}

func (c *Controller) ReloadPackages() error {
	if err := c.updatePackages(); err != nil {
		message := "无法读取功能包：" + err.Error()
		c.mu.Lock()
		c.status = AppStatus{Kind: StatusError, Message: &message}
		c.mu.Unlock()
		c.logger.Error("读取功能包失败：" + err.Error())
		c.emit()
		return err
	}
	c.scheduleDeveloperBuilds()
	c.emit()
	return nil
}

func sortedDependencyIDs(values map[string]PackageDependency) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
