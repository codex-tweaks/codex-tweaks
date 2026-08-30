package core

func (c *Controller) SetDisableGPUAcceleration(enabled bool) error {
	c.mu.Lock()
	if c.config.DisableGPUAcceleration == enabled {
		c.mu.Unlock()
		return nil
	}
	c.config.DisableGPUAcceleration = enabled
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if enabled {
		c.logger.Info("下次启动 Codex 时将禁用 GPU 加速")
	} else {
		c.logger.Info("下次启动 Codex 时将恢复 GPU 加速")
	}
	c.emit()
	return nil
}

func (c *Controller) codexLaunchOptions() CodexLaunchOptions {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CodexLaunchOptions{
		DisableGPUAcceleration: c.config.DisableGPUAcceleration,
	}
}
