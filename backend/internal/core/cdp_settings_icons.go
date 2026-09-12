package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const settingsIconRegistryKey = "__CODEX_TWEAKS_SETTINGS_ICON_REGISTRY__"

type settingsRemoteObject struct {
	ObjectID    string `json:"objectId"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type settingsRemoteProperty struct {
	Name  string               `json:"name"`
	Value settingsRemoteObject `json:"value"`
}

type settingsRemoteProperties struct {
	Result   []settingsRemoteProperty `json:"result"`
	Internal []settingsRemoteProperty `json:"internalProperties"`
}

func (s *rendererBridgeSession) settingsProperties(ctx context.Context, objectID string) (settingsRemoteProperties, error) {
	raw, err := s.call(ctx, "Runtime.getProperties", map[string]any{"objectId": objectID, "ownProperties": true})
	if err != nil {
		return settingsRemoteProperties{}, err
	}
	var properties settingsRemoteProperties
	err = json.Unmarshal(raw, &properties)
	return properties, err
}

// New Codex builds keep the icon table in module scope, behind an exported
// accessor. Read only that module's scopes through CDP; never evaluate a paused
// frame, patch application sources, or search global/user state.
func (s *rendererBridgeSession) exposeSettingsIconRegistry(ctx context.Context, moduleURL string) error {
	group := "codex-tweaks-settings-icons"
	defer func() {
		_, _ = s.call(ctx, "Runtime.releaseObjectGroup", map[string]any{"objectGroup": group})
	}()
	raw, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression":   fmt.Sprintf("import(%s).then(module => Object.values(module))", JSONLiteral(moduleURL)),
		"awaitPromise": true, "objectGroup": group,
	})
	if err != nil {
		return err
	}
	var result struct {
		Result settingsRemoteObject `json:"result"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Result.ObjectID == "" {
		return errors.New("无法读取 Codex 设置图标模块")
	}
	exports, err := s.settingsProperties(ctx, result.Result.ObjectID)
	if err != nil {
		return err
	}
	if found, err := s.publishSettingsIconRegistry(ctx, exports.Result); err != nil || found {
		return err
	}
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
				if scope.Value.Description != "Module" || scope.Value.ObjectID == "" {
					continue
				}
				variables, err := s.settingsProperties(ctx, scope.Value.ObjectID)
				if err != nil {
					return err
				}
				if found, err := s.publishSettingsIconRegistry(ctx, variables.Result); err != nil || found {
					return err
				}
			}
		}
	}
	return errors.New("当前 Codex 构建中未找到设置图标表")
}

func (s *rendererBridgeSession) publishSettingsIconRegistry(ctx context.Context, properties []settingsRemoteProperty) (bool, error) {
	arguments := []map[string]any{{"value": settingsIconRegistryKey}}
	for _, property := range properties {
		if property.Value.Type == "object" && property.Value.ObjectID != "" {
			arguments = append(arguments, map[string]any{"objectId": property.Value.ObjectID})
		}
	}
	if len(arguments) == 1 {
		return false, nil
	}
	raw, err := s.call(ctx, "Runtime.callFunctionOn", map[string]any{
		"objectId": arguments[1]["objectId"], "arguments": arguments, "returnByValue": true,
		"functionDeclaration": `function(key, ...values) {
  const icon = value => typeof value === "function" || Boolean(value && typeof value === "object" && value.component);
  const registry = values.find(value => {
    const general = Object.getOwnPropertyDescriptor(value, "general-settings")?.value;
    const personalization = Object.getOwnPropertyDescriptor(value, "personalization")?.value;
    return icon(general) && icon(personalization);
  });
  if (!registry) return false;
  globalThis[key] = registry;
  return true;
}`,
	})
	if err != nil {
		return false, err
	}
	var result struct {
		Result struct {
			Value bool `json:"value"`
		} `json:"result"`
	}
	err = json.Unmarshal(raw, &result)
	return result.Result.Value, err
}
