package core

import (
	"errors"
	"strings"
	"time"
)

const automaticRemoteCheckInterval = 6 * time.Hour

func (c *Controller) InstallRemotePackage(repositoryURL string, selectorType RemoteSelectorType, selectorValue string) {
	c.mu.Lock()
	if c.installingRemotePackage || c.installingLocalPackage || len(c.exportingPackageIDs) > 0 || len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return
	}
	c.installingRemotePackage = true
	c.remoteOperationMessage = nil
	c.remoteOperationError = nil
	c.mu.Unlock()
	c.emit()
	source := PackageSource{URL: strings.TrimSpace(repositoryURL), Selector: NewRemoteSelector(selectorType, selectorValue)}
	go func() {
		result, err := c.remote.Install(c.ctx, source, RemoteInstallOptions{})
		if err != nil {
			message := err.Error()
			c.mu.Lock()
			c.installingRemotePackage = false
			c.remoteOperationError = &message
			c.mu.Unlock()
			c.logger.Error("从 Git 安装功能包失败：" + message)
			c.emit()
			return
		}
		message := "已安装 " + result.PackageID + "，新包默认保持停用。"
		c.mu.Lock()
		c.installingRemotePackage = false
		c.remoteOperationMessage = &message
		c.disabledPackageIDs[result.PackageID] = true
		persistErr := c.persistConfigurationLocked()
		c.mu.Unlock()
		if persistErr != nil {
			c.logger.Error("保存功能包启用状态失败：" + persistErr.Error())
		}
		if err := c.updatePackages(); err != nil {
			c.logger.Error("安装后读取功能包失败：" + err.Error())
		}
		c.logger.Info("已从 Git 安装功能包 " + result.PackageID + " @ " + shortCommit(result.Lock.ResolvedCommit))
		c.emit()
		c.buildInstalledPackageIfPossible(result.PackageID)
	}()
}

func (c *Controller) InstallLocalPackage(sourcePath string) {
	c.mu.Lock()
	if c.installingLocalPackage || c.installingRemotePackage || len(c.exportingPackageIDs) > 0 || len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return
	}
	c.installingLocalPackage = true
	c.localOperationMessage = nil
	c.localOperationError = nil
	c.mu.Unlock()
	c.emit()
	go func() {
		result, err := c.installer.Install(sourcePath)
		if err != nil {
			message := err.Error()
			c.mu.Lock()
			c.installingLocalPackage = false
			c.localOperationError = &message
			c.mu.Unlock()
			c.logger.Error("从本地安装功能包失败：" + message)
			c.emit()
			return
		}
		message := "已安装 " + result.PackageID + "，新包默认保持停用。"
		c.mu.Lock()
		c.installingLocalPackage = false
		c.localOperationMessage = &message
		c.disabledPackageIDs[result.PackageID] = true
		persistErr := c.persistConfigurationLocked()
		c.mu.Unlock()
		if persistErr != nil {
			c.logger.Error("保存功能包启用状态失败：" + persistErr.Error())
		}
		if err := c.updatePackages(); err != nil {
			c.logger.Error("安装后读取功能包失败：" + err.Error())
		}
		c.logger.Info("已从本地安装功能包 " + result.PackageID + "：" + result.Directory)
		c.emit()
		c.buildInstalledPackageIfPossible(result.PackageID)
	}()
}

func (c *Controller) ReportLocalPackageSelectionError(message string) {
	c.mu.Lock()
	c.localOperationMessage = nil
	c.localOperationError = &message
	c.mu.Unlock()
	c.logger.Error("选择本地功能包失败：" + message)
	c.emit()
}

func (c *Controller) ClearRemoteOperationFeedback() {
	c.mu.Lock()
	c.remoteOperationMessage = nil
	c.remoteOperationError = nil
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) ClearLocalOperationFeedback() {
	c.mu.Lock()
	c.localOperationMessage = nil
	c.localOperationError = nil
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) InstallMissingDependencies(packageID string) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	c.mu.Lock()
	if c.installingPackageIDs[packageID] || len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return nil
	}
	c.installingPackageIDs[packageID] = true
	delete(c.remotePackageErrors, packageID)
	installed := append([]Package(nil), c.packages...)
	c.mu.Unlock()
	c.emit()
	go func() {
		results, err := c.remote.InstallMissingDependencies(c.ctx, pkg, installed)
		c.mu.Lock()
		delete(c.installingPackageIDs, packageID)
		if err != nil {
			c.remotePackageErrors[packageID] = err.Error()
		}
		c.mu.Unlock()
		if err != nil {
			c.logger.Error("安装功能包依赖失败：" + err.Error())
			c.emit()
			return
		}
		if err := c.updatePackages(); err != nil {
			c.logger.Error("安装依赖后读取功能包失败：" + err.Error())
		}
		for _, result := range results {
			c.buildInstalledPackageIfPossible(result.PackageID)
		}
		if len(results) == 0 {
			c.logger.Info("功能包 " + pkg.DisplayName() + " 的依赖均已安装")
		} else {
			c.logger.Info("已为 " + pkg.DisplayName() + " 安装 " + intString(len(results)) + " 个依赖包")
		}
		c.emit()
	}()
	return nil
}

func (c *Controller) CheckManagedPackageUpdates(automatic bool) {
	c.mu.Lock()
	if c.gitEnvironment == nil || c.checkingRemoteUpdates || len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return
	}
	if automatic && !c.lastAutomaticRemoteCheckAt.IsZero() && time.Since(c.lastAutomaticRemoteCheckAt) < automaticRemoteCheckInterval {
		c.mu.Unlock()
		return
	}
	c.checkingRemoteUpdates = true
	if automatic {
		c.lastAutomaticRemoteCheckAt = time.Now()
	}
	c.mu.Unlock()
	c.emit()
	go func() {
		packageIDs, err := c.remote.ManagedPackageIDs()
		if err == nil {
			for _, packageID := range packageIDs {
				update, updateErr := c.remote.CheckForUpdate(c.ctx, packageID)
				c.mu.Lock()
				if updateErr != nil {
					c.remotePackageErrors[packageID] = updateErr.Error()
				} else {
					c.remotePackageUpdates[packageID] = update
					delete(c.remotePackageErrors, packageID)
				}
				c.mu.Unlock()
			}
		}
		c.mu.Lock()
		c.checkingRemoteUpdates = false
		c.mu.Unlock()
		if err != nil {
			c.logger.Error("读取远程功能包注册表失败：" + err.Error())
		} else if !automatic {
			c.logger.Info("远程功能包更新检查完成")
		}
		c.emit()
	}()
}

func (c *Controller) UpdateManagedPackage(packageID string) error {
	pkg, exists := c.packageByID(packageID)
	if !exists || pkg.Origin.Kind != OriginManaged {
		return errors.New("功能包 " + packageID + " 不是由 Git 管理的包。")
	}
	c.mu.Lock()
	if c.installingPackageIDs[packageID] || len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return nil
	}
	c.installingPackageIDs[packageID] = true
	delete(c.remotePackageErrors, packageID)
	c.mu.Unlock()
	c.emit()
	go func() {
		registration, err := c.remote.Registration(packageID)
		if err == nil && registration == nil {
			err = errors.New("功能包 " + packageID + " 不是由 Git 管理的包。")
		}
		if err == nil {
			_, err = c.remote.Install(c.ctx, registration.Source, RemoteInstallOptions{ExpectedPackageID: packageID})
		}
		c.mu.Lock()
		delete(c.installingPackageIDs, packageID)
		if err != nil {
			c.remotePackageErrors[packageID] = err.Error()
		} else {
			delete(c.remotePackageUpdates, packageID)
		}
		c.mu.Unlock()
		if err != nil {
			c.logger.Error("更新远程功能包失败：" + err.Error())
			c.emit()
			return
		}
		if err := c.updatePackages(); err != nil {
			c.logger.Error("更新后读取功能包失败：" + err.Error())
		}
		c.logger.Info("已下载远程功能包更新：" + pkg.DisplayName())
		c.emit()
		c.buildInstalledPackageIfPossible(packageID)
	}()
	return nil
}

func (c *Controller) buildInstalledPackageIfPossible(packageID string) {
	c.mu.Lock()
	nodeAvailable := c.nodeEnvironment != nil
	c.mu.Unlock()
	if !nodeAvailable {
		return
	}
	if pkg, exists := c.packageByID(packageID); exists {
		c.startPackageBuild(pkg, true, true, false)
	}
}

func shortCommit(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
