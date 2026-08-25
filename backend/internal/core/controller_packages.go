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
	installedByID := map[string]Package{}
	for _, pkg := range resolution.OrderedPackages {
		installedByID[pkg.ID] = pkg
	}
	nodeAuthorizationsChanged := false
	for packageID, authorization := range c.nodeAuthorizations {
		if c.disabledPackageIDs[packageID] || NodeAuthorizationID(installedByID[packageID]) != authorization.AuthorizationID {
			delete(c.nodeAuthorizations, packageID)
			nodeAuthorizationsChanged = true
		}
	}
	c.clearUnauthorizedNodeRuntimeErrorsLocked()
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
	if err == nil && nodeAuthorizationsChanged {
		err = c.store.SaveNodeAuthorizations(c.nodeAuthorizations)
	}
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
	if !enabled && c.nodeRuntime != nil {
		c.nodeRuntime.StopAll()
	}
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
	if !enabled {
		c.developerAllowUnknownNode = false
	}
	c.clearUnauthorizedNodeRuntimeErrorsLocked()
	c.developerBuildAttemptKeys = map[string]string{}
	err := c.persistConfigurationLocked()
	packages := append([]Package(nil), c.packages...)
	disabled := cloneSet(c.disabledPackageIDs)
	trust := c.nodeTrustByPackageIDLocked()
	environment := c.enabledNodeEnvironmentLocked()
	c.mu.Unlock()
	if c.nodeRuntime != nil {
		c.nodeRuntime.Reconcile(c.ctx, packages, disabled, trust, environment)
	}
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
	go c.Refresh()
	return nil
}

func (c *Controller) SetDeveloperAllowUnknownNode(enabled bool) error {
	c.mu.Lock()
	if enabled && !c.config.DeveloperMode {
		c.mu.Unlock()
		return errors.New("必须先开启开发者模式。")
	}
	if c.developerAllowUnknownNode == enabled {
		c.mu.Unlock()
		return nil
	}
	c.developerAllowUnknownNode = enabled
	c.clearUnauthorizedNodeRuntimeErrorsLocked()
	packages := append([]Package(nil), c.packages...)
	disabled := cloneSet(c.disabledPackageIDs)
	trust := c.nodeTrustByPackageIDLocked()
	environment := c.enabledNodeEnvironmentLocked()
	c.mu.Unlock()
	if c.nodeRuntime != nil {
		c.nodeRuntime.Reconcile(c.ctx, packages, disabled, trust, environment)
	}
	if enabled {
		c.logger.Info("本次运行已允许开发者模式自动执行未知 Node 包")
	} else {
		c.logger.Info("已停止自动信任的 Node 包；它们已回到待授权状态")
	}
	c.emit()
	go c.Refresh()
	return nil
}

func (c *Controller) AuthorizeNodePackage(packageID, authorizationID string) error {
	pkg, exists := c.packageByID(packageID)
	if !exists || pkg.Manifest == nil || pkg.Manifest.CodexTweaks.Permissions.Node == nil {
		return errors.New("该功能包没有声明 Node 权限。")
	}
	currentAuthorizationID := NodeAuthorizationID(pkg)
	if currentAuthorizationID == "" {
		return errors.New("请先完成当前版本的功能包编译。")
	}
	if authorizationID == "" || authorizationID != currentAuthorizationID {
		return errors.New("功能包已发生变化，请查看最新风险说明后重新授权。")
	}
	c.mu.Lock()
	if c.disabledPackageIDs[packageID] {
		c.mu.Unlock()
		return errors.New("请先启用功能包，再授权当前 Node 版本。")
	}
	previousAuthorization, hadPreviousAuthorization := c.nodeAuthorizations[packageID]
	authorizeNodeRecord(c.nodeAuthorizations, packageID, currentAuthorizationID)
	err := c.store.SaveNodeAuthorizations(c.nodeAuthorizations)
	if err != nil {
		if hadPreviousAuthorization {
			c.nodeAuthorizations[packageID] = previousAuthorization
		} else {
			delete(c.nodeAuthorizations, packageID)
		}
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	c.logger.Info("用户已授权功能包执行 Node 代码：" + pkg.DisplayName())
	c.emit()
	go c.Refresh()
	return nil
}

func (c *Controller) SetPackageEnabled(packageID string, enabled bool) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	c.mu.Lock()
	if len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return errors.New("当前正在删除功能包。")
	}
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
	if !enabled {
		delete(c.nodeAuthorizations, packageID)
	}
	err := c.persistConfigurationLocked()
	if !enabled {
		if authorizationError := c.store.SaveNodeAuthorizations(c.nodeAuthorizations); err == nil {
			err = authorizationError
		}
	}
	c.mu.Unlock()
	if !enabled && c.nodeRuntime != nil {
		c.nodeRuntime.StopPackage(packageID)
	}
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
	c.mu.Lock()
	deletingPackage := len(c.deletingPackageIDs) > 0
	c.mu.Unlock()
	if deletingPackage {
		return errors.New("当前正在删除功能包。")
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
	if len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return errors.New("当前正在删除功能包。")
	}
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
