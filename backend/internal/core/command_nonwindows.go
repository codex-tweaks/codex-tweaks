//go:build !windows

package core

import "os/exec"

func configureCommand(*exec.Cmd) {}
