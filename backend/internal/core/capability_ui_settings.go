package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	settingsSectionMaximumCount = 8
	settingsSectionMaximumTitle = 64
)

var settingsSectionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var settingsSectionIconPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var settingsSectionPackageSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

type UISettingsSectionPermissions struct {
	Sections []UISettingsSectionPermission `json:"sections"`
}

type UISettingsSectionPermission struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Group string `json:"group,omitempty"`
	Icon  string `json:"icon,omitempty"`
	After string `json:"after,omitempty"`
	Slug  string `json:"slug,omitempty"`
}

type RuntimeSettingsSection struct {
	PackageID string `json:"packageID"`
	UISettingsSectionPermission
}

type SettingsAdapterConfiguration struct {
	AppModuleURL        string                   `json:"appModuleUrl"`
	VisibilityModuleURL string                   `json:"visibilityModuleUrl"`
	Sections            []RuntimeSettingsSection `json:"sections"`
}

func uiSettingsSectionCapabilityDescriptor() CapabilityDescriptor {
	return CapabilityDescriptor{
		DescriptorVersion: 1,
		ID:                UISettingsSectionCapabilityID,
		Version:           UISettingsSectionCapabilityVersion,
		Summary:           "Registers package-owned content as a real Codex settings navigation item and route through the host's private-module adapter.",
		Usage: CapabilityUsageDescriptor{
			UseWhen: []string{
				"A package needs an independently navigable settings page inside Codex settings.",
				"The package should provide only its page content while the host owns private settings registry, grouping, icon, route, and navigation integration.",
			},
			Constraints: []string{
				"The capability is available only in the main Codex renderer and may be omitted when the current Codex private modules cannot be adapted.",
				"Declare it as optional when the package has useful behavior in other renderers or can run without a settings page.",
				"Do not clone native navigation rows, overlay native settings, intercept native clicks, or import and patch Codex private modules from package code.",
				"mount may run again after route remounting and must return idempotent cleanup for every owned DOM node and side effect.",
			},
			ManifestExample: `{
  "ui.settings-section": {
    "version": "^1.0.0",
    "optional": true,
    "permissions": {
      "sections": [{
        "id": "appearance-extra",
        "title": "扩展外观",
        "group": "personal",
        "icon": "personalization",
        "after": "personalization"
      }]
    }
  }
}`,
			RuntimeExample: `const settings = api.capabilities.get("ui.settings-section");
settings?.register({
  id: "appearance-extra",
  mount(container) {
    const page = document.createElement("div");
    container.append(page);
    return () => page.remove();
  }
});`,
		},
		Manifest: CapabilityManifestDescriptor{
			RequirementJSONPointer: "/codexTweaks/capabilities/ui.settings-section",
			Fields: []CapabilityFieldDescriptor{
				{
					Path: "version", Type: "string", Format: "semver-requirement", Required: true,
					Description: "Version requirement negotiated independently from codexTweaks.apiVersion.",
				},
				{
					Path: "optional", Type: "boolean", Required: false,
					Description: "When true, the package remains valid when the capability or current-renderer adapter is unavailable.",
					DefaultJSON: capabilityString("false"),
				},
				{
					Path: "permissions.sections", Type: "array", ItemType: "object", Required: true,
					Description:  "Settings sections the package is allowed to register.",
					MinimumItems: capabilityInt(1), MaximumItems: capabilityInt(settingsSectionMaximumCount),
				},
				{
					Path: "permissions.sections[].id", Type: "string", Format: "lowercase-kebab-id", Required: true,
					Description:   "Package-local stable section ID; 1 to 48 characters and unique inside the capability declaration.",
					MinimumLength: capabilityInt(1), MaximumLength: capabilityInt(48),
				},
				{
					Path: "permissions.sections[].title", Type: "string", Required: true,
					Description: "Navigation and accessible label.", MinimumLength: capabilityInt(1), MaximumLength: capabilityInt(settingsSectionMaximumTitle),
				},
				{
					Path: "permissions.sections[].group", Type: "string", Required: false,
					Description: "Native settings group used for placement.",
					Values:      []string{"personal", "integrations", "coding", "archived"}, DefaultJSON: capabilityString(`"personal"`),
				},
				{
					Path: "permissions.sections[].icon", Type: "string", Format: "settings-icon-key", Required: false,
					Description: "Existing Codex settings icon key; unavailable keys fall back to personalization.", DefaultJSON: capabilityString(`"personalization"`),
				},
				{
					Path: "permissions.sections[].after", Type: "string", Format: "settings-section-slug", Required: false,
					Description: "Existing section slug after which this item is inserted inside its group.",
				},
			},
		},
		Runtime: CapabilityRuntimeDescriptor{
			Scope:          "main-renderer",
			RequiredAccess: `api.capabilities.require("ui.settings-section")`,
			OptionalAccess: `api.capabilities.get("ui.settings-section")`,
			Properties: []CapabilityFieldDescriptor{
				{Path: "id", Type: "string", Required: true, Description: "Stable capability ID.", Values: []string{UISettingsSectionCapabilityID}},
				{Path: "version", Type: "string", Format: "semver", Required: true, Description: "Concrete granted version.", Values: []string{UISettingsSectionCapabilityVersion}},
			},
			Methods: []CapabilityMethodDescriptor{
				{
					Name: "list", Async: false,
					Signature:   "list() => section[]",
					Description: "Returns the settings sections declared and granted to this package.",
					Inputs:      []CapabilityFieldDescriptor{},
					Outputs: []CapabilityFieldDescriptor{
						{Path: "return", Type: "array", ItemType: "object", Required: true, Description: "Granted section descriptors."},
						{Path: "return[].id", Type: "string", Required: true, Description: "Package-local declared section ID."},
						{Path: "return[].title", Type: "string", Required: true, Description: "Declared section title."},
						{Path: "return[].slug", Type: "string", Format: "host-settings-route-slug", Required: true, Description: "Host-generated globally unique route slug."},
					},
					Errors: []CapabilityErrorDescriptor{},
				},
				{
					Name: "register", Async: false,
					Signature:   "register(options) => registration",
					Description: "Registers the mount callback for one declared section and returns its navigation lifecycle handle.",
					Inputs: []CapabilityFieldDescriptor{
						{Path: "id", Type: "string", Required: true, Description: "One section ID declared in manifest permissions."},
						{Path: "mount", Type: "function", Format: "(container: Element) => void | (() => void)", Required: true, Description: "Mount package-owned content into the supplied route container and optionally return cleanup."},
					},
					Outputs: []CapabilityFieldDescriptor{
						{Path: "id", Type: "string", Required: true, Description: "Registered package-local section ID."},
						{Path: "slug", Type: "string", Format: "host-settings-route-slug", Required: true, Description: "Host-generated route slug."},
						{Path: "open", Type: "function", Format: "() => void", Required: true, Description: "Navigates through the real Codex settings router."},
						{Path: "unregister", Type: "function", Format: "() => void", Required: true, Description: "Removes the section registration; the host also invokes it during package cleanup."},
					},
					Errors: []CapabilityErrorDescriptor{
						{Code: "TypeError", Description: "Options or mount are not valid."},
						{Code: "Error", Description: "The section was undeclared, already registered, or the private settings adapter is unavailable."},
					},
				},
			},
		},
	}
}

func normalizeUISettingsSectionPermissions(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("必须声明 sections")
	}
	permissions := UISettingsSectionPermissions{}
	if err := decodeCapabilityJSON(raw, &permissions); err != nil {
		return nil, err
	}
	if len(permissions.Sections) == 0 || len(permissions.Sections) > settingsSectionMaximumCount {
		return nil, fmt.Errorf("sections 数量必须为 1 到 %d", settingsSectionMaximumCount)
	}
	seen := map[string]bool{}
	for index := range permissions.Sections {
		section := &permissions.Sections[index]
		section.ID = strings.TrimSpace(section.ID)
		section.Title = strings.TrimSpace(section.Title)
		section.Group = strings.TrimSpace(section.Group)
		section.Icon = strings.TrimSpace(section.Icon)
		section.After = strings.TrimSpace(section.After)
		section.Slug = ""
		if !settingsSectionIDPattern.MatchString(section.ID) || seen[section.ID] {
			return nil, fmt.Errorf("section id 无效或重复：%s", section.ID)
		}
		seen[section.ID] = true
		if section.Title == "" || utf8.RuneCountInString(section.Title) > settingsSectionMaximumTitle {
			return nil, fmt.Errorf("section %s 的 title 必须为 1 到 %d 个字符", section.ID, settingsSectionMaximumTitle)
		}
		if section.Group == "" {
			section.Group = "personal"
		}
		switch section.Group {
		case "personal", "integrations", "coding", "archived":
		default:
			return nil, fmt.Errorf("section %s 的 group 无效", section.ID)
		}
		if section.Icon == "" {
			section.Icon = "personalization"
		}
		if !settingsSectionIconPattern.MatchString(section.Icon) {
			return nil, fmt.Errorf("section %s 的 icon 无效", section.ID)
		}
		if section.After != "" && !settingsSectionIconPattern.MatchString(section.After) {
			return nil, fmt.Errorf("section %s 的 after 无效", section.ID)
		}
	}
	return json.Marshal(permissions)
}

func bindUISettingsSectionPermissions(packageID string, raw json.RawMessage) (json.RawMessage, error) {
	permissions := UISettingsSectionPermissions{}
	if err := json.Unmarshal(raw, &permissions); err != nil {
		return nil, err
	}
	packageSlug := strings.Trim(
		settingsSectionPackageSlugPattern.ReplaceAllString(strings.ToLower(packageID), "-"),
		"-",
	)
	if packageSlug == "" {
		packageSlug = "package"
	}
	if len(packageSlug) > 32 {
		packageSlug = strings.Trim(packageSlug[:32], "-")
	}
	suffix := FingerprintString(packageID)
	for index := range permissions.Sections {
		permissions.Sections[index].Slug = fmt.Sprintf(
			"codex-tweaks-%s-%s-%s", packageSlug, permissions.Sections[index].ID, suffix[:8],
		)
	}
	return json.Marshal(permissions)
}

func payloadSettingsSections(payload Payload) []RuntimeSettingsSection {
	result := []RuntimeSettingsSection{}
	for _, pkg := range payload.Packages {
		grant, ok := pkg.Capabilities[UISettingsSectionCapabilityID]
		if !ok {
			continue
		}
		permissions := UISettingsSectionPermissions{}
		if json.Unmarshal(grant.Permissions, &permissions) == nil {
			for _, section := range permissions.Sections {
				result = append(result, RuntimeSettingsSection{
					PackageID: pkg.ID, UISettingsSectionPermission: section,
				})
			}
		}
	}
	return result
}
