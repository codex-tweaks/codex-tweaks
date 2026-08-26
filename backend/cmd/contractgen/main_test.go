package main

import "testing"

func TestQuotedStringKeepsSwiftCompatibleAmpersands(t *testing.T) {
	if got, want := quotedString("About & Updates"), `"About & Updates"`; got != want {
		t.Fatalf("quoted string = %q, want %q", got, want)
	}
}
