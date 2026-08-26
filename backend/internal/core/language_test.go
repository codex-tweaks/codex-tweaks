package core

import "testing"

func TestResolveAppLanguageUsesManualPreference(t *testing.T) {
	if got := ResolveAppLanguage(LanguageJapanese, []string{"zh-Hans"}); got != LanguageJapanese {
		t.Fatalf("manual language = %q, want %q", got, LanguageJapanese)
	}
}

func TestResolveAppLanguageDetectsSupportedSystemLanguages(t *testing.T) {
	tests := map[string]AppLanguage{
		"zh-Hans-CN": LanguageSimplifiedChinese,
		"zh_CN":      LanguageSimplifiedChinese,
		"zh-Hant-HK": LanguageTraditionalChinese,
		"zh-TW":      LanguageTraditionalChinese,
		"en-US":      LanguageEnglish,
		"ja-JP":      LanguageJapanese,
		"ko-KR":      LanguageKorean,
	}
	for identifier, expected := range tests {
		t.Run(identifier, func(t *testing.T) {
			if got := ResolveAppLanguage(LanguageAuto, []string{identifier}); got != expected {
				t.Fatalf("resolved language = %q, want %q", got, expected)
			}
		})
	}
}

func TestResolveAppLanguageSkipsUnsupportedPreferencesAndFallsBackToEnglish(t *testing.T) {
	if got := ResolveAppLanguage(LanguageAuto, []string{"fr-FR", "ko-KR"}); got != LanguageKorean {
		t.Fatalf("resolved language = %q, want %q", got, LanguageKorean)
	}
	if got := ResolveAppLanguage(LanguageAuto, []string{"fr-FR"}); got != LanguageEnglish {
		t.Fatalf("fallback language = %q, want %q", got, LanguageEnglish)
	}
	if got := ResolveAppLanguage("unsupported", []string{"ja-JP"}); got != LanguageJapanese {
		t.Fatalf("invalid stored preference should normalize to auto, got %q", got)
	}
}

func TestAppLanguageOptionsUseGoOrderAndLocalizedTitles(t *testing.T) {
	order, options := AppLanguageOptions(PresentationTextForLocale(LanguageEnglish))
	want := []string{"auto", "zh-CN", "zh-TW", "en", "ja", "ko"}
	if len(order) != len(want) {
		t.Fatalf("language order = %v, want %v", order, want)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("language order = %v, want %v", order, want)
		}
	}
	if options["auto"] != "Automatic (System Language)" || options["ja"] != "日本語" {
		t.Fatalf("unexpected localized language options: %#v", options)
	}
}
