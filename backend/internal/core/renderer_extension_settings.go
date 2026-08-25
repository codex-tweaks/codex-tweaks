package core

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	SettingsSectionsAPIVersion  = 1
	settingsSectionMaximumCount = 8
	settingsSectionMaximumTitle = 64
)

var settingsSectionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var settingsSectionIconPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var settingsSectionPackageSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

type RuntimeSettingsSection struct {
	PackageID string `json:"packageID"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Group     string `json:"group,omitempty"`
	Icon      string `json:"icon,omitempty"`
	After     string `json:"after,omitempty"`
	Slug      string `json:"slug"`
}

type SettingsAdapterConfiguration struct {
	AppModuleURL        string                   `json:"appModuleUrl"`
	VisibilityModuleURL string                   `json:"visibilityModuleUrl"`
	Sections            []RuntimeSettingsSection `json:"sections"`
}

func normalizePackageUI(configuration *PackageUIConfiguration) error {
	if configuration == nil || configuration.SettingsSections == nil {
		return nil
	}
	extension := configuration.SettingsSections
	if extension.APIVersion != SettingsSectionsAPIVersion {
		return fmt.Errorf("不支持 ui.settingsSections API v%d。", extension.APIVersion)
	}
	if extension.Required == nil {
		value := true
		extension.Required = &value
	}
	if len(extension.Items) == 0 || len(extension.Items) > settingsSectionMaximumCount {
		return fmt.Errorf("ui.settingsSections.items 数量必须为 1 到 %d。", settingsSectionMaximumCount)
	}
	seen := map[string]bool{}
	for index := range extension.Items {
		section := &extension.Items[index]
		section.ID = strings.TrimSpace(section.ID)
		section.Title = strings.TrimSpace(section.Title)
		section.Group = strings.TrimSpace(section.Group)
		section.Icon = strings.TrimSpace(section.Icon)
		section.After = strings.TrimSpace(section.After)
		if !settingsSectionIDPattern.MatchString(section.ID) || seen[section.ID] {
			return fmt.Errorf("ui.settingsSections section id 无效或重复：%s", section.ID)
		}
		seen[section.ID] = true
		if section.Title == "" || utf8.RuneCountInString(section.Title) > settingsSectionMaximumTitle {
			return fmt.Errorf("ui.settingsSections section %s 的 title 必须为 1 到 %d 个字符。", section.ID, settingsSectionMaximumTitle)
		}
		if section.Group == "" {
			section.Group = "personal"
		}
		switch section.Group {
		case "personal", "integrations", "coding", "archived":
		default:
			return fmt.Errorf("ui.settingsSections section %s 的 group 无效。", section.ID)
		}
		if section.Icon == "" {
			section.Icon = "personalization"
		}
		if !settingsSectionIconPattern.MatchString(section.Icon) {
			return fmt.Errorf("ui.settingsSections section %s 的 icon 无效。", section.ID)
		}
		if section.After != "" && !settingsSectionIconPattern.MatchString(section.After) {
			return fmt.Errorf("ui.settingsSections section %s 的 after 无效。", section.ID)
		}
	}
	return nil
}

func settingsSectionsRequired(extension *SettingsSectionsExtension) bool {
	return extension != nil && (extension.Required == nil || *extension.Required)
}

func compilePackageUI(packageID string, configuration PackageUIConfiguration) (CompiledPackageUI, error) {
	if configuration.SettingsSections == nil {
		return CompiledPackageUI{}, nil
	}
	if len(configuration.SettingsSections.Items) == 0 {
		return CompiledPackageUI{}, errors.New("ui.settingsSections 没有可用的页面声明。")
	}
	sections := bindSettingsSections(packageID, configuration.SettingsSections.Items)
	return CompiledPackageUI{SettingsSections: &CompiledSettingsSections{
		Required: settingsSectionsRequired(configuration.SettingsSections),
		Items:    sections,
	}}, nil
}

func bindSettingsSections(packageID string, declarations []UISettingsSectionDeclaration) []RuntimeSettingsSection {
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
	result := make([]RuntimeSettingsSection, 0, len(declarations))
	for _, section := range declarations {
		result = append(result, RuntimeSettingsSection{
			PackageID: packageID,
			ID:        section.ID,
			Title:     section.Title,
			Group:     section.Group,
			Icon:      section.Icon,
			After:     section.After,
			Slug:      fmt.Sprintf("codex-tweaks-%s-%s-%s", packageSlug, section.ID, suffix[:8]),
		})
	}
	return result
}

func payloadSettingsSections(payload Payload) []RuntimeSettingsSection {
	result := []RuntimeSettingsSection{}
	for _, pkg := range payload.Packages {
		if pkg.UI.SettingsSections != nil {
			result = append(result, pkg.UI.SettingsSections.Items...)
		}
	}
	return result
}
