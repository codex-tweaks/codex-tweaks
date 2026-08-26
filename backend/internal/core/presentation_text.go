package core

func PresentationText() map[string]string {
	return presentationTextZhCN()
}

func PresentationTextForLocale(locale AppLanguage) map[string]string {
	switch locale {
	case LanguageTraditionalChinese:
		return presentationTextZhTW()
	case LanguageEnglish:
		return presentationTextEN()
	case LanguageJapanese:
		return presentationTextJA()
	case LanguageKorean:
		return presentationTextKO()
	default:
		return presentationTextZhCN()
	}
}

func presentationTextZhTW() map[string]string {
	return localizedPresentationText(presentationTextZhTWOverrides)
}

func presentationTextEN() map[string]string {
	return localizedPresentationText(presentationTextENOverrides)
}

func presentationTextJA() map[string]string {
	return localizedPresentationText(presentationTextJAOverrides)
}

func presentationTextKO() map[string]string {
	return localizedPresentationText(presentationTextKOOverrides)
}

func localizedPresentationText(overrides map[string]string) map[string]string {
	result := presentationTextZhCN()
	for key, value := range overrides {
		result[key] = value
	}
	return result
}
