package core

import (
	"bytes"
	"context"
	"os/exec"
)

type CommandResult struct {
	Status int
	Output string
}

type CommandRunner interface {
	Run(ctx context.Context, executable string, arguments []string, directory string, environment []string) (CommandResult, error)
}

type SystemCommandRunner struct{}

func (SystemCommandRunner) Run(ctx context.Context, executable string, arguments []string, directory string, environment []string) (CommandResult, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Env = environment
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if err == nil {
		return CommandResult{Output: output.String()}, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		return CommandResult{Status: exitError.ExitCode(), Output: output.String()}, nil
	}
	return CommandResult{}, err
}
