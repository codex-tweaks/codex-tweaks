package core

import "testing"

func TestValidateSourceAcceptsSupportedNonCredentialedGitURLs(t *testing.T) {
	for _, value := range []string{
		"https://github.com/example/package.git",
		"ssh://git@example.com/example/package.git",
		"git@example.com:example/package.git",
	} {
		source := PackageSource{URL: value, Selector: NewRemoteSelector(SelectorBranch, "main")}
		if err := ValidateSource(source); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
}

func TestValidateSourceAcceptsDefaultBranchWithoutValue(t *testing.T) {
	source := PackageSource{
		URL:      "https://github.com/example/package.git",
		Selector: NewRemoteSelector(SelectorDefaultBranch, ""),
	}
	if err := ValidateSource(source); err != nil {
		t.Fatal(err)
	}
}

func TestValidateSourceRejectsCredentialsInvalidSelectorsAndNonGitHubReleases(t *testing.T) {
	values := []PackageSource{
		{URL: "https://user@example.com/package.git", Selector: NewRemoteSelector(SelectorBranch, "main")},
		{URL: "https://user:secret@example.com/package.git", Selector: NewRemoteSelector(SelectorBranch, "main")},
		{URL: "file:///tmp/package", Selector: NewRemoteSelector(SelectorBranch, "main")},
		{URL: "https://example.com/package.git", Selector: NewRemoteSelector(SelectorBranch, "")},
		{URL: "https://example.com/package.git", Selector: NewRemoteSelector(SelectorGitHubLatestRelease, "")},
		{URL: "https://github.com/example/package.git", Selector: NewRemoteSelector(RemoteSelectorType("future"), "")},
	}
	for _, source := range values {
		if err := ValidateSource(source); err == nil {
			t.Fatalf("expected rejection: %#v", source)
		}
	}
}

func TestRepositoryProjectPageURLNormalizesSupportedGitSources(t *testing.T) {
	tests := map[string]string{
		"https://github.com/example/codex-tweaks-package.git":   "https://github.com/example/codex-tweaks-package",
		"https://git.example.com/team/package.git/":             "https://git.example.com/team/package",
		"ssh://git@github.com/example/codex-tweaks-package.git": "https://github.com/example/codex-tweaks-package",
		"git@git.example.com:team/codex-tweaks-package.git":     "https://git.example.com/team/codex-tweaks-package",
	}
	for source, expected := range tests {
		actual := repositoryProjectPageURL(source)
		if actual == nil || *actual != expected {
			t.Fatalf("repositoryProjectPageURL(%q) = %#v, want %q", source, actual, expected)
		}
	}
}

func TestRepositoryProjectPageURLRejectsNonRemoteAndMalformedSources(t *testing.T) {
	for _, source := range []string{"", "file:///tmp/package", "https://github.com", "not a repository"} {
		if actual := repositoryProjectPageURL(source); actual != nil {
			t.Fatalf("repositoryProjectPageURL(%q) = %q, want nil", source, *actual)
		}
	}
}
