package core

import "testing"

func TestSemanticVersionPrecedenceAndLargeNumbers(t *testing.T) {
	beta9, _ := ParseSemanticVersion("v1.0.0-beta.9")
	beta10, _ := ParseSemanticVersion("1.0.0-beta.10")
	release, _ := ParseSemanticVersion("1.0.0+build.7")
	next, _ := ParseSemanticVersion("1.0.1-beta.1")
	if beta9.Compare(beta10) >= 0 || beta10.Compare(release) >= 0 || release.Compare(next) >= 0 {
		t.Fatal("unexpected SemVer precedence")
	}
	small, ok := ParseSemanticVersion("99999999999999999999.0.0")
	if !ok {
		t.Fatal("large version should parse")
	}
	large, _ := ParseSemanticVersion("100000000000000000000.0.0")
	if small.Compare(large) >= 0 {
		t.Fatal("large numeric identifiers must compare by magnitude")
	}
}

func TestSemanticVersionRejectsInvalidIdentifiers(t *testing.T) {
	for _, value := range []string{"1.0", "01.0.0", "1.0.0-beta.01", "1.0.0-beta_1", "1.0.0+"} {
		if _, ok := ParseSemanticVersion(value); ok {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestVersionRequirementsMatchSwiftContract(t *testing.T) {
	tests := []struct {
		requirement string
		version     string
		want        bool
	}{
		{"^1.2.3", "1.9.0", true},
		{"^1.2.3", "2.0.0", false},
		{"~1.2.3", "1.2.99", true},
		{"~1.2.3", "1.3.0", false},
		{"1.x", "1.8.0", true},
		{">=1.2.3", "2.0.0", true},
		{"1.2.3", "v1.2.3", true},
		{"0.0.3", "0.0.3", true},
		{"0.0.x", "0.0.9", true},
	}
	for _, test := range tests {
		requirement, ok := ParseVersionRequirement(test.requirement)
		if !ok {
			t.Fatalf("requirement %q should parse", test.requirement)
		}
		if got := requirement.Contains(test.version); got != test.want {
			t.Errorf("%s contains %s = %v, want %v", test.requirement, test.version, got, test.want)
		}
	}
}
