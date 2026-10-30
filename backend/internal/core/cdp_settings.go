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

// Match the page itself, not settings-page-lazy or settings-page-frame.
var settingsPageScriptPattern = regexp.MustCompile(`/settings-page-[A-Za-z0-9_]+\.js$`)
var settingsRouteScriptPattern = regexp.MustCompile(`/app-runtime-[A-Za-z0-9_]+\.js$`)
var visibilityAssetPattern = regexp.MustCompile(`\./use-visible-settings-sections-[A-Za-z0-9_-]+\.js`)
var staticModuleImportPattern = regexp.MustCompile(`\bfrom\s*["'](\./[A-Za-z0-9_./-]+\.js)["']`)

func (s *rendererBridgeSession) ensureSettingsAdapter(
	ctx context.Context,
	payload Payload,
) (configuration *SettingsAdapterConfiguration, adapterError error) {
	sections := payloadSettingsSections(payload)
	if len(sections) == 0 {
		return nil, nil
	}
	// Settings belong to the main app; pet and detached renderers still receive
	// their packages and Node bridge without repeatedly probing absent modules.
	target, err := url.Parse(s.targetURL)
	if err != nil || target.Path != "/index.html" || target.Query().Get("initialRoute") == "/avatar-overlay" {
		return nil, nil
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	generation := s.executionGeneration.Load()
	if s.settingsAdapterGen == generation && s.settingsAdapterError != nil && time.Now().Before(s.settingsAdapterRetryAt) {
		return nil, s.settingsAdapterError
	}
	defer func() {
		if s.executionGeneration.Load() == generation {
			s.settingsAdapterGen = generation
			s.settingsAdapterError = adapterError
			if adapterError != nil {
				s.settingsAdapterRetryAt = time.Now().Add(30 * time.Second)
			}
		}
	}()
	if !s.debuggerEnabled {
		if _, err := s.call(ctx, "Debugger.enable", map[string]any{}); err != nil {
			return nil, fmt.Errorf("无法启用 Codex 模块适配：%w", err)
		}
		s.debuggerEnabled = true
	}
	if s.settingsAdapterCached && s.settingsAdapterGen == generation &&
		s.settingsRegistryURL != "" && s.settingsVisibilityURL != "" {
		return &SettingsAdapterConfiguration{
			RegistryModuleURL: s.settingsRegistryURL, VisibilityModuleURL: s.settingsVisibilityURL,
			NavigationModuleURL: s.settingsNavigationURL, IconRegistryKey: settingsIconRegistryKey,
			RouteRegistryKey: settingsRouteRegistryKey,
			Sections:         sections,
		}, nil
	}
	s.settingsAdapterCached = false
	assets, err := s.settingsAssetURLs(ctx)
	if err != nil {
		return nil, err
	}
	settingsModuleURL, err := settingsAssetURL(assets, settingsPageScriptPattern)
	if err != nil {
		return nil, err
	}
	routeModuleURL, err := settingsAssetURL(assets, settingsRouteScriptPattern)
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
	dependencies, err := s.findSettingsDependencies(ctx, settingsModuleURL, settingsSource)
	if err != nil {
		return nil, err
	}
	if err := s.exposeSettingsIconRegistry(ctx, visibilityURL); err != nil {
		return nil, err
	}
	if err := s.exposeSettingsRouteRegistry(ctx, routeModuleURL); err != nil {
		return nil, err
	}
	if s.executionGeneration.Load() != generation {
		return nil, errors.New("Codex 页面在设置模块适配期间发生刷新")
	}
	s.settingsRegistryURL = dependencies.RegistryURL
	s.settingsVisibilityURL = visibilityURL
	s.settingsNavigationURL = dependencies.NavigationURL
	s.settingsAdapterGen = generation
	s.settingsAdapterCached = true
	return &SettingsAdapterConfiguration{
		RegistryModuleURL: s.settingsRegistryURL, VisibilityModuleURL: s.settingsVisibilityURL,
		NavigationModuleURL: s.settingsNavigationURL, IconRegistryKey: settingsIconRegistryKey,
		RouteRegistryKey: settingsRouteRegistryKey,
		Sections:         sections,
	}, nil
}

type settingsDependencies struct {
	RegistryURL   string `json:"registryURL"`
	NavigationURL string `json:"navigationURL"`
}

func (s *rendererBridgeSession) findSettingsDependencies(
	ctx context.Context,
	settingsModuleURL, settingsSource string,
) (settingsDependencies, error) {
	candidates := []string{settingsModuleURL}
	for _, match := range staticModuleImportPattern.FindAllStringSubmatch(settingsSource, -1) {
		moduleURL, err := resolveModuleURL(settingsModuleURL, match[1])
		if err != nil {
			return settingsDependencies{}, err
		}
		candidates = append(candidates, moduleURL)
	}
	// The page has already initialized these imports. Identify exported values
	// by their role rather than assuming they still live in app-initial.
	expression := fmt.Sprintf(`(async () => {
  const dependencies = { registryURL: "", navigationURL: "" };
  for (const url of %s) {
    const values = Object.values(await import(url));
    if (!dependencies.registryURL && values.some(value =>
      Array.isArray(value)
      && value.some(entry => entry?.slug === "general-settings")
      && value.some(entry => entry?.slug === "personalization")
    )) dependencies.registryURL = url;
    if (!dependencies.navigationURL && values.some(value =>
      value && typeof value === "object"
      && typeof value.dispatchHostMessage === "function"
    )) dependencies.navigationURL = url;
    if (dependencies.registryURL && dependencies.navigationURL) break;
  }
  return dependencies;
})()`, JSONLiteral(candidates))
	raw, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true, "awaitPromise": true,
	})
	if err != nil {
		return settingsDependencies{}, err
	}
	var result struct {
		Result struct {
			Value settingsDependencies `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return settingsDependencies{}, err
	}
	if len(result.ExceptionDetails) != 0 {
		return settingsDependencies{}, errors.New("无法读取 Codex 设置依赖，请更新 Codex Tweaks 后重试")
	}
	if result.Result.Value.RegistryURL == "" || result.Result.Value.NavigationURL == "" {
		return settingsDependencies{}, errors.New("当前 Codex 设置结构不受支持，请更新 Codex Tweaks 后重试")
	}
	return result.Result.Value, nil
}

func settingsAssetURL(assets []string, pattern *regexp.Regexp) (string, error) {
	var found string
	for _, asset := range assets {
		if !pattern.MatchString(asset) {
			continue
		}
		if found != "" {
			return "", errors.New("Codex 资源目录存在多个匹配项，请重新安装 Codex 后重试")
		}
		found = asset
	}
	if found == "" {
		return "", errors.New("没有找到 Codex 设置资源，请更新 Codex Tweaks 后重试")
	}
	return found, nil
}

func (s *rendererBridgeSession) waitForScript(ctx context.Context, scriptURL string) string {
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

func (s *rendererBridgeSession) getScriptSource(ctx context.Context, scriptID string) (string, error) {
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

func (s *rendererBridgeSession) importModule(ctx context.Context, moduleURL string) error {
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
