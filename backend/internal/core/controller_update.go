package core

import (
	"strings"
	"time"
)

func (c *Controller) CheckAppUpdate(prompt bool) {
	c.mu.Lock()
	if c.updateChecking {
		c.mu.Unlock()
		return
	}
	c.updateChecking = true
	c.updateLastError = nil
	channel := c.config.UpdateChannel
	currentVersion := c.currentVersion
	c.mu.Unlock()
	c.emit()
	go func() {
		release, err := c.updates.Check(c.ctx, channel, currentVersion)
		c.mu.Lock()
		c.updateChecking = false
		if err != nil {
			message := err.Error()
			if strings.Contains(strings.ToLower(message), "network is unreachable") || strings.Contains(strings.ToLower(message), "no route to host") {
				message = "当前无法连接网络。"
			}
			c.updateLastError = &message
			c.mu.Unlock()
			c.emit()
			return
		}
		c.latestRelease = release
		now := NewCodableTime(time.Now())
		c.config.UpdateLastCheckAt = &now
		if release != nil {
			hasNewer := HasNewerVersion(release, c.currentVersion)
			skipped := containsString(c.config.UpdateSkippedVersions, NormalizeVersion(release.TagName))
			if prompt && hasNewer && !skipped {
				copy := *release
				c.pendingUpdate = &copy
			} else if c.pendingUpdate == nil || c.pendingUpdate.TagName != release.TagName || !hasNewer || skipped {
				c.pendingUpdate = nil
			}
		} else {
			c.pendingUpdate = nil
		}
		persistErr := c.persistConfigurationLocked()
		c.mu.Unlock()
		if persistErr != nil {
			c.logger.Error("保存更新状态失败：" + persistErr.Error())
		}
		c.emit()
	}()
}

func (c *Controller) SetUpdateChannel(channel UpdateChannel) error {
	if channel != UpdateBeta {
		channel = UpdateStable
	}
	c.mu.Lock()
	if c.config.UpdateChannel == channel {
		c.mu.Unlock()
		return nil
	}
	c.config.UpdateChannel = channel
	c.latestRelease = nil
	c.pendingUpdate = nil
	c.updateLastError = nil
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	c.emit()
	return err
}

func (c *Controller) SetUpdateAutoCheck(enabled bool) error {
	c.mu.Lock()
	c.config.UpdateAutoCheck = enabled
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	c.emit()
	return err
}

func (c *Controller) DismissUpdate() {
	c.mu.Lock()
	c.pendingUpdate = nil
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) SkipUpdate(tagName string) error {
	version := NormalizeVersion(tagName)
	c.mu.Lock()
	if !containsString(c.config.UpdateSkippedVersions, version) {
		c.config.UpdateSkippedVersions = append(c.config.UpdateSkippedVersions, version)
		c.config.UpdateSkippedVersions = uniqueSorted(c.config.UpdateSkippedVersions)
	}
	c.pendingUpdate = nil
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	c.emit()
	return err
}

func (c *Controller) UnskipAndPromptUpdate() error {
	c.mu.Lock()
	if c.latestRelease == nil || !HasNewerVersion(c.latestRelease, c.currentVersion) {
		c.mu.Unlock()
		return nil
	}
	version := NormalizeVersion(c.latestRelease.TagName)
	filtered := []string{}
	for _, skipped := range c.config.UpdateSkippedVersions {
		if skipped != version {
			filtered = append(filtered, skipped)
		}
	}
	c.config.UpdateSkippedVersions = filtered
	copy := *c.latestRelease
	c.pendingUpdate = &copy
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	c.emit()
	return err
}
