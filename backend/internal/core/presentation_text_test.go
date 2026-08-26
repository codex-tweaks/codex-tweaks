package core

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

var presentationPlaceholderPattern = regexp.MustCompile(`\{[A-Za-z][A-Za-z0-9]*\}`)

func TestPresentationTranslationsCoverEveryKeyAndPreservePlaceholders(t *testing.T) {
	base := presentationTextZhCN()
	translations := map[AppLanguage]map[string]string{
		LanguageTraditionalChinese: presentationTextZhTWOverrides,
		LanguageEnglish:            presentationTextENOverrides,
		LanguageJapanese:           presentationTextJAOverrides,
		LanguageKorean:             presentationTextKOOverrides,
	}
	for locale, translation := range translations {
		t.Run(string(locale), func(t *testing.T) {
			if len(translation) != len(base) {
				t.Fatalf("translation has %d keys, want %d", len(translation), len(base))
			}
			for key, source := range base {
				value, ok := translation[key]
				if !ok || strings.TrimSpace(value) == "" {
					t.Fatalf("missing translation for %q", key)
				}
				if got, want := placeholders(value), placeholders(source); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
					t.Fatalf("placeholders for %q = %v, want %v", key, got, want)
				}
			}
			for key := range translation {
				if _, ok := base[key]; !ok {
					t.Fatalf("translation contains unknown key %q", key)
				}
			}
		})
	}
}

func TestPresentationTranslationsExposeLocalizedShellCopy(t *testing.T) {
	tests := map[AppLanguage]string{
		LanguageSimplifiedChinese:  "关于与更新",
		LanguageTraditionalChinese: "關於與更新",
		LanguageEnglish:            "About & Updates",
		LanguageJapanese:           "情報とアップデート",
		LanguageKorean:             "정보 및 업데이트",
	}
	for locale, expected := range tests {
		if got := PresentationTextForLocale(locale)["nav.updates"]; got != expected {
			t.Fatalf("%s nav.updates = %q, want %q", locale, got, expected)
		}
	}
}

func placeholders(value string) []string {
	result := presentationPlaceholderPattern.FindAllString(value, -1)
	sort.Strings(result)
	return result
}
