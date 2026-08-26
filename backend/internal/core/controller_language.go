package core

import "fmt"

func (c *Controller) SetLanguage(language AppLanguage) error {
	if normalized := NormalizeAppLanguage(language); normalized != language {
		return fmt.Errorf("unsupported app language: %s", language)
	}
	c.mu.Lock()
	if c.config.Language == language {
		c.mu.Unlock()
		return nil
	}
	c.config.Language = language
	err := c.persistConfigurationLocked()
	c.mu.Unlock()
	c.emit()
	return err
}
