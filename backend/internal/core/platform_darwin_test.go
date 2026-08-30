//go:build darwin

package core

import (
	"context"
	"testing"
)

func TestDarwinPlatformUsesNonInteractiveProcessControlsAndLoopbackLaunch(t *testing.T) {
	type invocation struct {
		executable string
		arguments  []string
	}
	invocations := []invocation{}
	runner := commandRunnerFunc(func(_ context.Context, executable string, arguments []string, _ string, _ []string) (CommandResult, error) {
		invocations = append(invocations, invocation{executable: executable, arguments: append([]string(nil), arguments...)})
		if executable == "/usr/bin/lsappinfo" {
			return CommandResult{Output: `ASN:0x0-0x1234-"ChatGPT":`}, nil
		}
		return CommandResult{}, nil
	})
	platform := NewPlatform(runner)
	running, err := platform.IsCodexRunning(context.Background())
	if err != nil || !running {
		t.Fatalf("running = %v, err = %v", running, err)
	}
	if err := platform.ActivateCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := platform.LaunchCodex(context.Background(), CodexLaunchOptions{DisableGPUAcceleration: true}); err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 3 || invocations[0].executable != "/usr/bin/lsappinfo" || invocations[1].executable != "/usr/bin/open" {
		t.Fatalf("unexpected commands: %#v", invocations)
	}
	lookup := invocations[0]
	if !containsString(lookup.arguments, "bundleID="+CodexBundleIdentifier) {
		t.Fatalf("unexpected running-app lookup: %#v", lookup)
	}
	launch := invocations[2]
	if launch.executable != "/usr/bin/open" || !containsString(launch.arguments, "-n") || !containsString(launch.arguments, "-b") {
		t.Fatalf("unexpected launch: %#v", launch)
	}
	for _, argument := range CodexDebuggingArguments {
		if !containsString(launch.arguments, argument) {
			t.Fatalf("launch omitted %q: %#v", argument, launch.arguments)
		}
	}
	for _, argument := range []string{"--use-gl=angle", "--use-angle=swiftshader"} {
		if !containsString(launch.arguments, argument) {
			t.Fatalf("macOS launch omitted %q: %#v", argument, launch.arguments)
		}
	}
	if containsString(launch.arguments, "--disable-gpu") {
		t.Fatalf("macOS launch retained the non-starting --disable-gpu path: %#v", launch.arguments)
	}
}

func TestDarwinPlatformTreatsEmptyLaunchServicesLookupAsNotRunning(t *testing.T) {
	platform := NewPlatform(commandRunnerFunc(func(_ context.Context, executable string, _ []string, _ string, _ []string) (CommandResult, error) {
		if executable != "/usr/bin/lsappinfo" {
			t.Fatalf("unexpected executable: %s", executable)
		}
		return CommandResult{}, nil
	}))

	running, err := platform.IsCodexRunning(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if running {
		t.Fatal("empty LaunchServices lookup should not report Codex as running")
	}
}

func TestDarwinPlatformRestartUsesApplicationNameAndWaitsForLaunchServicesExit(t *testing.T) {
	type invocation struct {
		executable string
		arguments  []string
	}
	invocations := []invocation{}
	runner := commandRunnerFunc(func(_ context.Context, executable string, arguments []string, _ string, _ []string) (CommandResult, error) {
		invocations = append(invocations, invocation{executable: executable, arguments: append([]string(nil), arguments...)})
		return CommandResult{}, nil
	})

	if err := NewPlatform(runner).RestartCodex(context.Background(), CodexLaunchOptions{DisableGPUAcceleration: true}); err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 3 {
		t.Fatalf("unexpected commands: %#v", invocations)
	}
	terminate := invocations[0]
	if terminate.executable != "/usr/bin/killall" || !containsString(terminate.arguments, "-TERM") || !containsString(terminate.arguments, "ChatGPT") {
		t.Fatalf("unexpected terminate command: %#v", terminate)
	}
	if invocations[1].executable != "/usr/bin/lsappinfo" || invocations[2].executable != "/usr/bin/open" {
		t.Fatalf("unexpected restart sequence: %#v", invocations)
	}
	for _, argument := range []string{"--use-gl=angle", "--use-angle=swiftshader"} {
		if !containsString(invocations[2].arguments, argument) {
			t.Fatalf("macOS restart omitted %q: %#v", argument, invocations[2].arguments)
		}
	}
}

type commandRunnerFunc func(
	context.Context,
	string,
	[]string,
	string,
	[]string,
) (CommandResult, error)

func (function commandRunnerFunc) Run(
	ctx context.Context,
	executable string,
	arguments []string,
	directory string,
	environment []string,
) (CommandResult, error) {
	return function(ctx, executable, arguments, directory, environment)
}
