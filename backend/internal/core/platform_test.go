package core

import "testing"

func TestCodexLaunchArgumentsUsePlatformSoftwareRenderingOnlyWhenEnabled(t *testing.T) {
	windowsArguments := codexLaunchArguments(
		CodexLaunchOptions{DisableGPUAcceleration: true},
		"windows",
	)
	if !containsString(windowsArguments, "--disable-gpu") {
		t.Fatalf("Windows arguments omitted --disable-gpu: %#v", windowsArguments)
	}

	macOSArguments := codexLaunchArguments(
		CodexLaunchOptions{DisableGPUAcceleration: true},
		"darwin",
	)
	for _, argument := range []string{"--use-gl=angle", "--use-angle=swiftshader"} {
		if !containsString(macOSArguments, argument) {
			t.Fatalf("macOS arguments omitted %q: %#v", argument, macOSArguments)
		}
	}
	if containsString(macOSArguments, "--disable-gpu") {
		t.Fatalf("macOS arguments retained the non-starting --disable-gpu path: %#v", macOSArguments)
	}

	defaultArguments := codexLaunchArguments(CodexLaunchOptions{}, "darwin")
	if containsString(defaultArguments, "--disable-gpu") {
		t.Fatalf("default arguments unexpectedly disabled GPU acceleration: %#v", defaultArguments)
	}
	for _, argument := range []string{"--use-gl=angle", "--use-angle=swiftshader"} {
		if containsString(defaultArguments, argument) {
			t.Fatalf("default arguments unexpectedly enabled %q: %#v", argument, defaultArguments)
		}
	}
}
