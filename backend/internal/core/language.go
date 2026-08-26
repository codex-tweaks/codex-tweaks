package core

import "strings"

type AppLanguage string

const (
	LanguageAuto               AppLanguage = "auto"
	LanguageSimplifiedChinese  AppLanguage = "zh-CN"
	LanguageTraditionalChinese AppLanguage = "zh-TW"
	LanguageEnglish            AppLanguage = "en"
	LanguageJapanese           AppLanguage = "ja"
	LanguageKorean             AppLanguage = "ko"
)

var supportedAppLanguages = []AppLanguage{
	LanguageAuto,
	LanguageSimplifiedChinese,
	LanguageTraditionalChinese,
	LanguageEnglish,
	LanguageJapanese,
	LanguageKorean,
}

func SupportedAppLanguages() []AppLanguage {
	return append([]AppLanguage(nil), supportedAppLanguages...)
}

func AppLanguageOptions(text map[string]string) ([]string, map[string]string) {
	order := make([]string, 0, len(supportedAppLanguages))
	options := make(map[string]string, len(supportedAppLanguages))
	for _, language := range supportedAppLanguages {
		value := string(language)
		order = append(order, value)
		options[value] = text[appLanguageTitleKey(language)]
	}
	return order, options
}

func appLanguageTitleKey(language AppLanguage) string {
	switch language {
	case LanguageSimplifiedChinese:
		return "language.simplifiedChinese"
	case LanguageTraditionalChinese:
		return "language.traditionalChinese"
	case LanguageEnglish:
		return "language.english"
	case LanguageJapanese:
		return "language.japanese"
	case LanguageKorean:
		return "language.korean"
	default:
		return "language.auto"
	}
}

func NormalizeAppLanguage(language AppLanguage) AppLanguage {
	for _, supported := range supportedAppLanguages {
		if language == supported {
			return language
		}
	}
	return LanguageAuto
}

func ResolveAppLanguage(preference AppLanguage, preferredLanguages []string) AppLanguage {
	preference = NormalizeAppLanguage(preference)
	if preference != LanguageAuto {
		return preference
	}
	for _, identifier := range preferredLanguages {
		if language, ok := matchPreferredLanguage(identifier); ok {
			return language
		}
	}
	return LanguageEnglish
}

func matchPreferredLanguage(identifier string) (AppLanguage, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(identifier), "_", "-"))
	if normalized == "" {
		return "", false
	}
	parts := strings.Split(normalized, "-")
	switch parts[0] {
	case "zh":
		for _, part := range parts[1:] {
			switch part {
			case "hant", "tw", "hk", "mo":
				return LanguageTraditionalChinese, true
			case "hans":
				return LanguageSimplifiedChinese, true
			}
		}
		return LanguageSimplifiedChinese, true
	case "en":
		return LanguageEnglish, true
	case "ja":
		return LanguageJapanese, true
	case "ko":
		return LanguageKorean, true
	default:
		return "", false
	}
}
