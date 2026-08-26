package core

import (
	"errors"
	"path/filepath"
)

func (c *Controller) ExportPackage(packageID, destinationPath string) error {
	pkg, exists := c.packageByID(packageID)
	if !exists {
		return errors.New("没有找到功能包：" + packageID)
	}
	c.mu.Lock()
	if c.installingLocalPackage || c.installingRemotePackage || len(c.exportingPackageIDs) > 0 || len(c.deletingPackageIDs) > 0 {
		c.mu.Unlock()
		return errors.New("当前有另一个功能包本地操作正在进行。")
	}
	c.exportingPackageIDs[packageID] = true
	c.localOperationMessage = nil
	c.localOperationError = nil
	c.mu.Unlock()
	c.emit()

	go func() {
		err := c.exporter.Export(c.ctx, pkg, destinationPath)
		text := c.presentationText()
		c.mu.Lock()
		delete(c.exportingPackageIDs, packageID)
		if err != nil {
			message := resolvePresentationText(text, "packages.exportFailed", map[string]string{
				"name": pkg.DisplayName(), "message": err.Error(),
			})
			c.localOperationError = &message
			c.mu.Unlock()
			c.logger.Error(message)
			c.emit()
			return
		}
		message := resolvePresentationText(text, "packages.exportSuccess", map[string]string{
			"name": pkg.DisplayName(), "file": filepath.Base(destinationPath),
		})
		c.localOperationMessage = &message
		c.mu.Unlock()
		c.logger.Info(message + "（" + destinationPath + "）")
		c.emit()
	}()
	return nil
}
