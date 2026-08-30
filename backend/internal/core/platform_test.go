package core

import "testing"

func TestCodexLaunchArgumentsIncludeDisableGPUOnlyWhenEnabled(t *testing.T) {
	enabledArguments := codexLaunchArguments(CodexLaunchOptions{DisableGPUAcceleration: true})
	if !containsString(enabledArguments, "--disable-gpu") {
		t.Fatalf("enabled arguments omitted --disable-gpu: %#v", enabledArguments)
	}

	defaultArguments := codexLaunchArguments(CodexLaunchOptions{})
	if containsString(defaultArguments, "--disable-gpu") {
		t.Fatalf("default arguments unexpectedly disabled GPU acceleration: %#v", defaultArguments)
	}
}
