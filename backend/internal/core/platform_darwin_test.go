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
	if err := platform.LaunchCodex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 3 || invocations[0].executable != "/usr/bin/pgrep" || invocations[1].executable != "/usr/bin/open" {
		t.Fatalf("unexpected commands: %#v", invocations)
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
