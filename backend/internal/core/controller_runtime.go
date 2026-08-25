package core

import (
	"context"
	"errors"
	"strconv"
	"time"
)

func intString(value int) string { return strconv.Itoa(value) }

func (c *Controller) CheckNodeEnvironment() {
	c.mu.Lock()
	if c.checkingNode {
		c.mu.Unlock()
		return
	}
	c.checkingNode = true
	c.mu.Unlock()
	c.emit()
	go func() {
		environment := c.builder.DetectNodeEnvironment(c.ctx)
		c.mu.Lock()
		c.nodeEnvironment = environment
		c.checkingNode = false
		c.mu.Unlock()
		if environment != nil {
			c.logger.Info("已检测到 Node.js " + environment.Version)
			c.scheduleDeveloperBuilds()
		} else {
			c.logger.Error("未找到可用的 Node.js、npm 和 npx")
		}
		c.emit()
	}()
}

func (c *Controller) CheckGitEnvironment() {
	c.mu.Lock()
	if c.checkingGit {
		c.mu.Unlock()
		return
	}
	c.checkingGit = true
	c.mu.Unlock()
	c.emit()
	go func() {
		environment := c.remote.DetectGitEnvironment(c.ctx)
		c.mu.Lock()
		c.gitEnvironment = environment
		c.checkingGit = false
		c.mu.Unlock()
		if environment != nil {
			c.logger.Info("已检测到 " + environment.Version)
			c.CheckManagedPackageUpdates(true)
		} else {
			c.logger.Error("未找到可用的 Git")
		}
		c.emit()
	}()
}

func (c *Controller) Refresh() {
	if !c.refreshMu.TryLock() {
		return
	}
	defer c.refreshMu.Unlock()
	if err := c.updatePackages(); err != nil {
		message := "无法读取功能包：" + err.Error()
		c.mu.Lock()
		c.status = AppStatus{Kind: StatusError, Message: &message}
		c.mu.Unlock()
		c.emit()
		return
	}
	c.scheduleDeveloperBuilds()
	c.CheckManagedPackageUpdates(true)

	c.mu.Lock()
	enabled := c.config.Enabled
	cleanupCompleted := c.disabledCleanupCompleted
	c.mu.Unlock()
	if !enabled {
		if c.nodeRuntime != nil {
			c.nodeRuntime.StopAll()
		}
		if !cleanupCompleted {
			ctx, cancel := context.WithTimeout(c.ctx, 6*time.Second)
			_ = c.cdp.CleanupAllTargets(ctx)
			cancel()
		}
		c.mu.Lock()
		c.disabledCleanupCompleted = true
		c.packageRuntimeErrors = map[string]string{}
		c.status = AppStatus{Kind: StatusDisabled}
		c.mu.Unlock()
		c.emit()
		return
	}
	c.mu.Lock()
	c.disabledCleanupCompleted = false
	packages := append([]Package(nil), c.packages...)
	disabled := cloneSet(c.disabledPackageIDs)
	trust := c.nodeTrustByPackageIDLocked()
	nodeEnvironment := cloneNodeEnvironment(c.nodeEnvironment)
	c.mu.Unlock()
	if c.nodeRuntime != nil {
		c.nodeRuntime.Reconcile(c.ctx, packages, disabled, trust, nodeEnvironment)
		nodeErrors := c.nodeRuntime.RuntimeErrors()
		c.mu.Lock()
		for packageID, message := range nodeErrors {
			c.packageRuntimeErrors[packageID] = message
		}
		c.mu.Unlock()
	}

	running, err := c.platform.IsCodexRunning(c.ctx)
	if err != nil {
		message := err.Error()
		c.setStatus(AppStatus{Kind: StatusError, Message: &message})
		return
	}
	if !running {
		c.mu.Lock()
		attempted := c.hasAttemptedInitialLaunch
		if !attempted {
			c.hasAttemptedInitialLaunch = true
			c.status = AppStatus{Kind: StatusLaunchingCodex}
		}
		c.mu.Unlock()
		if attempted {
			c.setStatus(AppStatus{Kind: StatusCodexNotRunning})
			return
		}
		if err := c.platform.LaunchCodex(c.ctx); err != nil {
			message := err.Error()
			c.logger.Error("自动启动 Codex 失败：" + message)
			c.setStatus(AppStatus{Kind: StatusError, Message: &message})
			return
		}
		c.logger.Info("Codex 未运行，已自动使用本地 CDP 参数启动")
		c.setStatus(AppStatus{Kind: StatusWaitingForCDP})
		return
	}
	c.mu.Lock()
	c.hasAttemptedInitialLaunch = true
	forceGeneration := c.forceGeneration
	c.mu.Unlock()
	nodeRunnable := map[string]bool{}
	if c.nodeRuntime != nil {
		runningNodePackages := c.nodeRuntime.RunningPackageIDs()
		for packageID, mode := range trust {
			if mode != "" && runningNodePackages[packageID] {
				nodeRunnable[packageID] = true
			}
		}
	}
	loadResult := c.store.LoadPayload(packages, disabled, nodeRunnable)
	c.mu.Lock()
	c.packagePayloadErrors = loadResult.PackageErrors
	c.mu.Unlock()
	result, err := c.cdp.Inject(c.ctx, loadResult.Payload, forceGeneration)
	if errors.Is(err, ErrCDPEndpointUnavailable) {
		c.setStatus(AppStatus{Kind: StatusRestartRequired})
		return
	}
	if err != nil {
		message := err.Error()
		c.logger.Error("CDP 刷新失败：" + message)
		c.setStatus(AppStatus{Kind: StatusError, Message: &message})
		return
	}
	c.mu.Lock()
	combinedRuntimeErrors := map[string]string{}
	if c.nodeRuntime != nil {
		combinedRuntimeErrors = c.nodeRuntime.RuntimeErrors()
	}
	for packageID, message := range result.PackageErrors {
		combinedRuntimeErrors[packageID] = message
	}
	if !stringMapsEqual(combinedRuntimeErrors, c.packageRuntimeErrors) {
		for packageID, message := range result.PackageErrors {
			c.logger.Error("功能包 " + packageID + " 运行失败：" + message)
		}
		c.packageRuntimeErrors = combinedRuntimeErrors
	}
	switch {
	case result.TargetCount == 0:
		c.status = AppStatus{Kind: StatusWaitingForPage}
	case result.SuccessCount > 0:
		c.status = AppStatus{Kind: StatusConnected, TargetCount: result.SuccessCount}
		if result.FailureCount() > 0 {
			c.logger.Error("部分 Codex 页面注入失败：" + intString(result.FailureCount()) + "/" + intString(result.TargetCount))
		}
	default:
		message := "发现 Codex 页面，但注入没有成功"
		c.status = AppStatus{Kind: StatusError, Message: &message}
	}
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) setStatus(status AppStatus) {
	c.mu.Lock()
	c.status = status
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) OpenCodex() error {
	running, err := c.platform.IsCodexRunning(c.ctx)
	if err != nil {
		return err
	}
	if running {
		return c.platform.ActivateCodex(c.ctx)
	}
	c.mu.Lock()
	c.hasAttemptedInitialLaunch = true
	c.status = AppStatus{Kind: StatusLaunchingCodex}
	c.mu.Unlock()
	c.emit()
	if err := c.platform.LaunchCodex(c.ctx); err != nil {
		message := err.Error()
		c.setStatus(AppStatus{Kind: StatusError, Message: &message})
		return err
	}
	c.logger.Info("已使用本地 CDP 参数启动 Codex")
	c.setStatus(AppStatus{Kind: StatusWaitingForCDP})
	return nil
}

func (c *Controller) RestartCodex() {
	c.mu.Lock()
	c.hasAttemptedInitialLaunch = true
	c.status = AppStatus{Kind: StatusLaunchingCodex}
	c.mu.Unlock()
	c.emit()
	go func() {
		if err := c.platform.RestartCodex(c.ctx); err != nil {
			message := err.Error()
			c.logger.Error("重启 Codex 失败：" + message)
			c.setStatus(AppStatus{Kind: StatusError, Message: &message})
			return
		}
		c.logger.Info("Codex 已重启并开启本地 CDP")
		c.setStatus(AppStatus{Kind: StatusWaitingForCDP})
	}()
}

func (c *Controller) RestartCodexUI() error {
	c.mu.Lock()
	if c.restartingCodexUI {
		c.mu.Unlock()
		return errors.New("Codex 界面正在重启，请稍候")
	}
	c.restartingCodexUI = true
	c.mu.Unlock()
	c.emit()
	defer func() {
		c.mu.Lock()
		c.restartingCodexUI = false
		c.mu.Unlock()
		c.emit()
	}()

	ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
	defer cancel()
	result, err := c.cdp.ReloadAllTargets(ctx)
	if err != nil {
		c.logger.Error("重启 Codex 界面失败：" + err.Error())
		return err
	}
	c.mu.Lock()
	c.forceGeneration++
	c.mu.Unlock()
	c.logger.Info("已重启 " + intString(result.SuccessCount) + " 个 Codex 界面，等待重新注入")
	return nil
}

func (c *Controller) Reinject() {
	c.mu.Lock()
	c.forceGeneration++
	c.mu.Unlock()
	go c.Refresh()
}

func (c *Controller) scheduleDeveloperBuilds() {
	c.mu.Lock()
	if !c.config.DeveloperMode || c.nodeEnvironment == nil {
		c.mu.Unlock()
		return
	}
	candidates := []Package{}
	for _, pkg := range c.packages {
		if c.disabledPackageIDs[pkg.ID] || c.buildingPackageIDs[pkg.ID] {
			continue
		}
		disposition := pkg.BuildDisposition(CompilerVersion)
		mayBuild := disposition == BuildSourceChanged || disposition == BuildNotBuilt && (pkg.Manifest == nil || len(pkg.Manifest.Dependencies) == 0)
		key, hasKey := pkg.BuildRequestKey(CompilerVersion)
		if mayBuild && hasKey && c.developerBuildAttemptKeys[pkg.ID] != key {
			c.developerBuildAttemptKeys[pkg.ID] = key
			candidates = append(candidates, pkg)
		}
	}
	c.mu.Unlock()
	for _, pkg := range candidates {
		c.startPackageBuild(pkg, false, false, true)
	}
}

func (c *Controller) BuildPackage(packageID string) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	c.startPackageBuild(pkg, true, true, false)
	return nil
}

func (c *Controller) startPackageBuild(pkg Package, installDependencies, allowCompilerDownload, automatic bool) {
	if pkg.ValidationError != nil {
		return
	}
	c.mu.Lock()
	if c.buildingPackageIDs[pkg.ID] || len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return
	}
	c.buildingPackageIDs[pkg.ID] = true
	delete(c.packageBuildErrors, pkg.ID)
	delete(c.packageBuildErrorRequestKeys, pkg.ID)
	requestKey, hasRequestKey := pkg.BuildRequestKey(CompilerVersion)
	c.mu.Unlock()
	if automatic {
		c.logger.Info("自动编译功能包：" + pkg.DisplayName())
	} else {
		c.logger.Info("手动更新功能包：" + pkg.DisplayName())
	}
	c.emit()
	go func() {
		_, err := c.builder.Build(c.ctx, pkg, installDependencies, allowCompilerDownload)
		c.mu.Lock()
		delete(c.buildingPackageIDs, pkg.ID)
		if err != nil {
			c.packageBuildErrors[pkg.ID] = err.Error()
			if hasRequestKey {
				c.packageBuildErrorRequestKeys[pkg.ID] = requestKey
			}
		}
		c.mu.Unlock()
		if err != nil {
			c.logger.Error("功能包 " + pkg.DisplayName() + " 编译失败：" + err.Error())
			c.emit()
			return
		}
		if err := c.updatePackages(); err != nil {
			c.logger.Error("编译后重新读取功能包失败：" + err.Error())
		}
		c.mu.Lock()
		c.forceGeneration++
		c.mu.Unlock()
		c.logger.Info("功能包已编译并激活：" + pkg.DisplayName())
		c.emit()
		c.Refresh()
	}()
}

func stringMapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
