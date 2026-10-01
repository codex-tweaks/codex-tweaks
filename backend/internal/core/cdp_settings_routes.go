package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const settingsRouteRegistryKey = "__CODEX_TWEAKS_SETTINGS_ROUTE_REGISTRY__"

// Routes are converted to plain objects in app module scope before React renders
// them. Discover that registry once per execution context, never via DOM/Fibers.
func (s *rendererBridgeSession) exposeSettingsRouteRegistry(ctx context.Context, moduleURL string) error {
	group := "codex-tweaks-settings-routes"
	defer func() {
		_, _ = s.call(ctx, "Runtime.releaseObjectGroup", map[string]any{"objectGroup": group})
	}()

	// Keep remote descriptions bounded: returning every exported function from a
	// large application bundle can serialize megabytes of function source.
	raw, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": fmt.Sprintf(`import(%s).then(module => Object.values(module)
  .filter(value => typeof value === "function" && value.name && value.toString().length <= 2048)
  .slice(0, 16))`, JSONLiteral(moduleURL)),
		"awaitPromise": true, "objectGroup": group,
	})
	if err != nil {
		return err
	}
	var result struct {
		Result settingsRemoteObject `json:"result"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Result.ObjectID == "" {
		return errors.New("无法读取 Codex 设置路由模块")
	}
	exports, err := s.settingsProperties(ctx, result.Result.ObjectID)
	if err != nil {
		return err
	}
	seenScopes := map[string]bool{}
	for _, export := range exports.Result {
		if export.Value.Type != "function" || export.Value.ObjectID == "" {
			continue
		}
		function, err := s.settingsProperties(ctx, export.Value.ObjectID)
		if err != nil {
			return err
		}
		for _, internal := range function.Internal {
			if internal.Name != "[[Scopes]]" || internal.Value.ObjectID == "" {
				continue
			}
			scopes, err := s.settingsProperties(ctx, internal.Value.ObjectID)
			if err != nil {
				return err
			}
			for _, scope := range scopes.Result {
				if scope.Value.Description != "Module" || scope.Value.ObjectID == "" || seenScopes[scope.Value.ObjectID] {
					continue
				}
				seenScopes[scope.Value.ObjectID] = true
				found, err := s.publishSettingsRouteRegistry(ctx, scope.Value.ObjectID)
				if err != nil {
					return err
				}
				if found {
					return nil
				}
			}
		}
	}
	return errors.New("当前 Codex 构建中未找到模块级设置路由表")
}

func (s *rendererBridgeSession) publishSettingsRouteRegistry(ctx context.Context, scopeID string) (bool, error) {
	raw, err := s.call(ctx, "Runtime.callFunctionOn", map[string]any{
		"objectId": scopeID, "arguments": []map[string]any{{"value": settingsRouteRegistryKey}}, "returnByValue": true,
		// CDP represents an internal module scope as {description, object}. Read
		// only route-shaped arrays in that module, not global or component state.
		"functionDeclaration": `function(key) {
  if (!this.object) return false;
  const arrays = Object.values(this.object).filter(Array.isArray);
  const seen = new Set();
  const queue = arrays.flatMap(array => array);
  for (let index = 0; index < queue.length && index < 10000; index++) {
    const route = queue[index];
    if (!route || typeof route !== "object" || seen.has(route)
        || !Object.hasOwn(route, "id")) continue;
    seen.add(route);
    const children = route.children;
    if (!Array.isArray(children)) continue;
    if (route.path === "/settings"
        && children.some(child => child.path === "general-settings")
        && children.some(child => child.path === "personalization")) {
      const template = children.find(child => child.path === "*");
      if (!template?.element || Object.isFrozen(children)) return false;
      globalThis[key] = { children, template };
      return true;
    }
    queue.push(...children);
  }
  return false;
}`,
	})
	if err != nil {
		return false, err
	}
	var result struct {
		Result struct {
			Value bool `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, err
	}
	if len(result.ExceptionDetails) != 0 {
		return false, errors.New("无法读取 Codex 模块级设置路由表")
	}
	return result.Result.Value, nil
}
