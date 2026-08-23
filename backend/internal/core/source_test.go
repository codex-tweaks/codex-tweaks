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
