package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"
)

var appInitialScriptPattern = regexp.MustCompile(`/app-initial-[A-Za-z0-9_-]+\.js(?:\?.*)?$`)
var settingsPageImportPattern = regexp.MustCompile("import\\(`(\\./settings-page-[A-Za-z0-9_-]+\\.js)`\\)")
var visibilityAssetPattern = regexp.MustCompile(`\./use-visible-settings-sections-[A-Za-z0-9_-]+\.js`)

func (s *capabilitySession) ensureSettingsAdapter(
	ctx context.Context,
	payload Payload,
) (*SettingsAdapterConfiguration, error) {
	sections := payloadSettingsSections(payload)
	if len(sections) == 0 {
		return nil, nil
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if !s.debuggerEnabled {
		if _, err := s.call(ctx, "Debugger.enable", map[string]any{}); err != nil {
			return nil, fmt.Errorf("无法启用 Codex 模块适配：%w", err)
		}
		s.debuggerEnabled = true
	}
	appModuleURL, appScriptID := s.waitForMatchingScript(ctx, appInitialScriptPattern)
	if appScriptID == "" {
		return nil, errors.New("当前 Codex 构建中未找到 app-initial 模块")
	}
	appSource, err := s.getScriptSource(ctx, appScriptID)
	if err != nil {
		return nil, err
	}
	settingsAsset := matchAsset(settingsPageImportPattern, appSource)
	if settingsAsset == "" {
		return nil, errors.New("当前 Codex 构建中未找到设置模块入口")
	}
	settingsModuleURL, err := resolveModuleURL(appModuleURL, settingsAsset)
	if err != nil {
		return nil, err
	}
	if err := s.importModule(ctx, settingsModuleURL); err != nil {
		return nil, fmt.Errorf("无法加载 Codex 设置模块：%w", err)
	}
	settingsScriptID := s.waitForScript(ctx, settingsModuleURL)
	if settingsScriptID == "" {
		return nil, errors.New("Codex 设置模块没有出现在调试器中")
	}
	if s.settingsScriptID != settingsScriptID {
		settingsSource, err := s.getScriptSource(ctx, settingsScriptID)
		if err != nil {
			return nil, err
		}
		visibilityAsset := visibilityAssetPattern.FindString(settingsSource)
		if visibilityAsset == "" {
			return nil, errors.New("当前 Codex 构建中未找到设置可见性模块")
		}
		visibilityURL, err := resolveModuleURL(settingsModuleURL, visibilityAsset)
		if err != nil {
			return nil, err
		}
		s.settingsScriptID = settingsScriptID
		s.settingsAppModuleURL = appModuleURL
		s.settingsVisibilityURL = visibilityURL
	}
	return &SettingsAdapterConfiguration{
		AppModuleURL: s.settingsAppModuleURL, VisibilityModuleURL: s.settingsVisibilityURL,
		Sections: sections,
	}, nil
}

func matchAsset(pattern *regexp.Regexp, source string) string {
	match := pattern.FindStringSubmatch(source)
	if len(match) == 2 {
		return match[1]
	}
	return pattern.FindString(source)
}

func (s *capabilitySession) findScript(pattern *regexp.Regexp) (string, string) {
	s.scriptMu.RLock()
	defer s.scriptMu.RUnlock()
	for scriptURL, scriptID := range s.scripts {
		if pattern.MatchString(scriptURL) {
			return scriptURL, scriptID
		}
	}
	return "", ""
}

func (s *capabilitySession) waitForMatchingScript(
	ctx context.Context,
	pattern *regexp.Regexp,
) (string, string) {
	if scriptURL, scriptID := s.findScript(pattern); scriptID != "" {
		return scriptURL, scriptID
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ""
		case <-timer.C:
			return "", ""
		case <-ticker.C:
			if scriptURL, scriptID := s.findScript(pattern); scriptID != "" {
				return scriptURL, scriptID
			}
		}
	}
}

func (s *capabilitySession) waitForScript(ctx context.Context, scriptURL string) string {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		s.scriptMu.RLock()
		scriptID := s.scripts[scriptURL]
		s.scriptMu.RUnlock()
		if scriptID != "" {
			return scriptID
		}
		select {
		case <-ctx.Done():
			return ""
		case <-timer.C:
			return ""
		case <-ticker.C:
		}
	}
}

func (s *capabilitySession) getScriptSource(ctx context.Context, scriptID string) (string, error) {
	raw, err := s.call(ctx, "Debugger.getScriptSource", map[string]any{"scriptId": scriptID})
	if err != nil {
		return "", err
	}
	result := struct {
		ScriptSource string `json:"scriptSource"`
	}{}
	if err := json.Unmarshal(raw, &result); err != nil || result.ScriptSource == "" {
		return "", errors.New("Codex 设置模块源码无法读取")
	}
	return result.ScriptSource, nil
}

func (s *capabilitySession) importModule(ctx context.Context, moduleURL string) error {
	expression := fmt.Sprintf("import(%s).then(() => true)", JSONLiteral(moduleURL))
	raw, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true, "awaitPromise": true, "userGesture": false,
	})
	if err != nil {
		return err
	}
	result := map[string]any{}
	if json.Unmarshal(raw, &result) != nil || result["exceptionDetails"] != nil {
		return errors.New("模块动态导入失败")
	}
	return nil
}

func resolveModuleURL(baseURL, relative string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	reference, err := url.Parse(relative)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(reference).String(), nil
}
