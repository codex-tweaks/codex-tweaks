package core

import "errors"

func (c *Controller) DeletePackage(packageID string) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	c.mu.Lock()
	if c.installingLocalPackage || c.installingRemotePackage || len(c.exportingPackageIDs) > 0 ||
		len(c.deletingPackageIDs) > 0 || len(c.installingPackageIDs) > 0 ||
		len(c.buildingPackageIDs) > 0 || c.checkingRemoteUpdates {
		c.mu.Unlock()
		return errors.New("当前有另一个功能包操作正在进行。")
	}
	c.deletingPackageIDs[packageID] = true
	c.localOperationMessage = nil
	c.localOperationError = nil
	c.mu.Unlock()
	c.emit()

	go c.deletePackage(pkg)
	return nil
}

func (c *Controller) deletePackage(pkg Package) {
	if c.nodeRuntime != nil {
		c.nodeRuntime.StopPackage(pkg.ID)
	}

	var err error
	if pkg.Origin.Kind == OriginManaged {
		err = c.remote.Remove(pkg.ID)
	} else {
		err = c.store.DeleteLocalPackage(pkg)
	}
	if err == nil {
		err = c.store.DeletePackageArtifacts(pkg.ID)
	}
	if err == nil {
		c.mu.Lock()
		c.disabledPackageIDs[pkg.ID] = true
		delete(c.nodeAuthorizations, pkg.ID)
		delete(c.packageBuildErrors, pkg.ID)
		delete(c.packageBuildErrorRequestKeys, pkg.ID)
		delete(c.packageRuntimeErrors, pkg.ID)
		delete(c.packagePayloadErrors, pkg.ID)
		delete(c.remotePackageUpdates, pkg.ID)
		delete(c.remotePackageErrors, pkg.ID)
		configurationErr := c.persistConfigurationLocked()
		authorizationErr := c.store.SaveNodeAuthorizations(c.nodeAuthorizations)
		c.mu.Unlock()
		err = errors.Join(configurationErr, authorizationErr)
	}
	if err == nil {
		err = c.updatePackages()
	}

	presentationText := c.presentationText()
	c.mu.Lock()
	delete(c.deletingPackageIDs, pkg.ID)
	if err != nil {
		message := resolvePresentationText(presentationText, "packages.deleteFailed", map[string]string{
			"name": pkg.DisplayName(), "message": err.Error(),
		})
		c.localOperationError = &message
		c.mu.Unlock()
		c.logger.Error(message)
		c.Refresh()
		c.emit()
		return
	}
	c.forceGeneration++
	c.mu.Unlock()
	c.Refresh()

	message := resolvePresentationText(presentationText, "packages.deleteSuccess", map[string]string{
		"name": pkg.DisplayName(),
	})
	c.mu.Lock()
	c.localOperationMessage = &message
	c.mu.Unlock()
	c.logger.Info(message)
	c.emit()
}
