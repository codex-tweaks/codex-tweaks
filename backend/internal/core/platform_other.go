//go:build !darwin && !windows

package core

import (
	"context"
	"errors"
	"runtime"
)

type unsupportedPlatform struct{}

func NewPlatform(CommandRunner) Platform { return unsupportedPlatform{} }

func (unsupportedPlatform) IsCodexRunning(context.Context) (bool, error) { return false, nil }
func (unsupportedPlatform) ActivateCodex(context.Context) error {
	return errors.New("当前系统尚未实现 Codex 桌面控制")
}
func (unsupportedPlatform) LaunchCodex(context.Context) error {
	return errors.New("当前系统尚未实现 Codex 桌面控制")
}
func (unsupportedPlatform) RestartCodex(context.Context) error {
	return errors.New("当前系统尚未实现 Codex 桌面控制")
}
func (unsupportedPlatform) Architecture() string { return runtime.GOARCH }
