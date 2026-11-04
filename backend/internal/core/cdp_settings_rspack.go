package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Rspack replaces named Vite assets with numbered chunks. Its entry module is
// asynchronous; wait for initialization before identifying settings factories.
const rspackSettingsModuleDiscovery = `async (require) => {
  const startup = Object.entries(require.c)
    .filter(([id, module]) => module.exports instanceof Promise
      && /\.a\([^,]+,\s*async function/.test(String(require.m[id])))
    .map(([, module]) => module.exports);
  await Promise.all(startup);
  // Native settings chunks remain lazy even after app startup. Use the app's
  // own prefetch hook to load their dependencies without navigating the UI.
  const prefetchers = [];
  for (const [id, factory] of Object.entries(require.m)) {
    if (!/\.SettingsLayout\b/.test(String(factory))) continue;
    for (const value of Object.values(require(id))) {
      if (typeof value === "function" && typeof value.prefetch === "function") prefetchers.push(value.prefetch);
    }
  }
  if (prefetchers.length !== 1) throw new Error("Codex Rspack settings prefetch module matches: " + prefetchers.length);
  await prefetchers[0]();
  const matches = { registry: [], navigation: [], icons: [], routes: [] };
  for (const [id, factory] of Object.entries(require.m)) {
    const cached = require.c[id];
    if (cached?.error) continue;
    const source = Function.prototype.toString.call(factory);
    if (/slug\s*:\s*["']general-settings["']/.test(source)
        && /slug\s*:\s*["']personalization["']/.test(source)) {
      if (Object.values(require(id)).some(value => Array.isArray(value)
          && value.some(entry => entry?.slug === "general-settings")
          && value.some(entry => entry?.slug === "personalization"))) matches.registry.push(id);
    }
    if (source.includes("dispatchHostMessage(") && source.includes("this.handlers")
        && source.includes("this.bufferedMessages") && source.includes("getInstance()")) {
      if (Object.values(require(id)).some(value => value && typeof value === "object"
          && value.handlers instanceof Map
          && typeof value.dispatchHostMessage === "function")) matches.navigation.push(id);
    }
    if (/["']general-settings["']\s*:\s*\{\s*component\s*:/.test(source)
        && /personalization\s*:\s*\{\s*component\s*:/.test(source)) matches.icons.push(id);
    if (/path\s*:\s*["']\/settings["']/.test(source)
        && /path\s*:\s*["']\*["']/.test(source)) matches.routes.push(id);
  }
  for (const [role, ids] of Object.entries(matches)) {
    if (ids.length !== 1) throw new Error("Codex Rspack settings " + role
      + " module matches: " + ids.length);
  }
  return Object.fromEntries(Object.entries(matches).map(([role, ids]) => [role, ids[0]]));
}`

func settingsModuleExpression(moduleURL, moduleID string) string {
	expression := fmt.Sprintf("import(%s)", JSONLiteral(moduleURL))
	if moduleID != "" {
		expression += fmt.Sprintf(".then(module => module.__webpack_require__(%s))", JSONLiteral(moduleID))
	}
	return expression
}

func settingsModuleScope(description, moduleID string) bool {
	return description == "Module" || (moduleID != "" && description == "Closure ("+moduleID+")")
}

func (s *rendererBridgeSession) ensureRspackSettingsAdapter(
	ctx context.Context, sections []RuntimeSettingsSection, generation uint64,
) (*SettingsAdapterConfiguration, error) {
	expression := fmt.Sprintf(`(async () => {
  const discover = %s;
  // The normal monitor will revisit a renderer still showing its startup UI.
  // Do not hold its short CDP deadline while the primary app loads.
  if (!document.querySelector("main")) return { pending: true };
  for (const script of document.querySelectorAll('script[type="module"][src]')) {
    const url = new URL(script.src);
    if (url.protocol !== "app:" || url.host !== "-" || !url.pathname.startsWith("/assets/")) continue;
    const module = await import(url.href);
    const require = module.__webpack_require__;
    if (typeof require !== "function" || !require.m || !require.c) continue;
    return { url: url.href, ...await discover(require) };
  }
  throw new Error("Codex Rspack renderer runtime unavailable");
})()`, rspackSettingsModuleDiscovery)
	raw, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "awaitPromise": true, "returnByValue": true,
	})
	if err != nil {
		return nil, err
	}
	var result struct {
		Result struct {
			Value struct {
				Pending    bool   `json:"pending"`
				URL        string `json:"url"`
				Registry   string `json:"registry"`
				Navigation string `json:"navigation"`
				Icons      string `json:"icons"`
				Routes     string `json:"routes"`
			} `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	modules := result.Result.Value
	if details := result.ExceptionDetails; details != nil {
		message := details.Exception.Description
		if message == "" {
			message = details.Text
		}
		return nil, fmt.Errorf("无法定位 Codex Rspack 设置模块：%s", strings.SplitN(message, "\n", 2)[0])
	}
	if modules.Pending {
		return nil, nil
	}
	if modules.URL == "" || modules.Registry == "" ||
		modules.Navigation == "" || modules.Icons == "" || modules.Routes == "" {
		return nil, errors.New("无法定位 Codex Rspack 设置模块，请检查模块适配")
	}
	if err := s.exposeSettingsIconRegistryInModule(ctx, modules.URL, modules.Icons); err != nil {
		return nil, err
	}
	if err := s.exposeSettingsRouteRegistryInModule(ctx, modules.URL, modules.Routes); err != nil {
		return nil, err
	}
	if s.executionGeneration.Load() != generation {
		return nil, errors.New("Codex 页面在设置模块适配期间发生刷新")
	}
	s.settingsRegistryURL = modules.URL
	s.settingsVisibilityURL = modules.URL
	s.settingsNavigationURL = modules.URL
	s.settingsRegistryModuleID = modules.Registry
	s.settingsNavigationModuleID = modules.Navigation
	s.settingsAdapterGen = generation
	s.settingsAdapterCached = true
	return &SettingsAdapterConfiguration{
		RegistryModuleURL: modules.URL, VisibilityModuleURL: modules.URL,
		NavigationModuleURL: modules.URL, RegistryModuleID: modules.Registry,
		NavigationModuleID: modules.Navigation, IconRegistryKey: settingsIconRegistryKey,
		RouteRegistryKey: settingsRouteRegistryKey, Sections: sections,
	}, nil
}
