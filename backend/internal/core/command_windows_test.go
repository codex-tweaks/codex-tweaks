//go:build windows

package core

import (
	"context"
	"os/exec"
	"testing"
)

func TestWindowsCommandsNeverCreateAVisibleConsole(t *testing.T) {
	command := exec.Command("cmd.exe", "/D", "/C", "exit", "0")
	configureCommand(command)
	assertCommandHasNoVisibleConsole(t, command)
}

func TestWindowsNodeRuntimeNeverCreatesAVisibleConsole(t *testing.T) {
	command := newNodeRuntimeCommand(context.Background(), "node.exe", "-e", "")
	assertCommandHasNoVisibleConsole(t, command)
}

func assertCommandHasNoVisibleConsole(t *testing.T, command *exec.Cmd) {
	t.Helper()
	if command.SysProcAttr == nil {
		t.Fatal("configureCommand did not set Windows process attributes")
	}
	if !command.SysProcAttr.HideWindow {
		t.Fatal("Windows child process is not hidden")
	}
	if command.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CREATE_NO_WINDOW missing from flags 0x%X", command.SysProcAttr.CreationFlags)
	}
}
