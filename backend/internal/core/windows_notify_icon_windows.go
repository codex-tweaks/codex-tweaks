//go:build windows

package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Codex owns its notification area icon through a dedicated message window and adds the icon while
// it starts up. Stopping Codex with taskkill leaves the shell holding the record of the dead
// instance, and the shell refuses the add of the freshly started instance while that record is still
// there, so Codex comes back without an icon and never tries again.
//
// Workaround: the shell asks every application to add its icons again by sending TaskbarCreated, and
// Codex honours that message. Posting it to the icon host window of the new instance makes Codex add
// the icon again, and the shell drops the stale record in the process. The message goes to that one
// window, so no other notification area application is involved. This works around shell behaviour
// Codex Tweaks cannot fix; it is not a fix of the underlying refusal.
const (
	codexNotifyIconHostClass       = "NotifyIconHostWindow"
	codexNotifyIconImageName       = "ChatGPT.exe"
	codexNotifyIconWindowTimeout   = 20 * time.Second
	codexNotifyIconWindowInterval  = 250 * time.Millisecond
	codexNotifyIconPathBufferSize  = 1024
	processQueryLimitedInformation = 0x1000
	// The icon host window shows up slightly before Codex adds the icon, so the repair has to
	// arrive after that failed add instead of before it.
	codexNotifyIconSettleDelay = 2 * time.Second
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	procEnumWindows                = user32.NewProc("EnumWindows")
	procGetClassNameW              = user32.NewProc("GetClassNameW")
	procGetWindowThreadProcessID   = user32.NewProc("GetWindowThreadProcessId")
	procPostMessageW               = user32.NewProc("PostMessageW")
	procRegisterWindowMessageW     = user32.NewProc("RegisterWindowMessageW")
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")

	// EnumWindows needs a C callback and Go only allows a limited number of them, so the callback
	// is created once and reports the window it found through the guarded variable below.
	codexNotifyIconMutex    sync.Mutex
	codexNotifyIconFound    uintptr
	codexNotifyIconCallback = syscall.NewCallback(func(window uintptr, _ uintptr) uintptr {
		if !strings.Contains(windowClassName(window), codexNotifyIconHostClass) {
			return 1
		}
		if !strings.EqualFold(filepath.Base(windowProcessImagePath(window)), codexNotifyIconImageName) {
			return 1
		}
		codexNotifyIconFound = window
		return 0
	})
)

func restoreCodexNotifyIcon(ctx context.Context) error {
	window, err := waitForCodexNotifyIconHost(ctx)
	if err != nil {
		return err
	}
	if err := waitContext(ctx, codexNotifyIconSettleDelay); err != nil {
		return err
	}
	name, err := syscall.UTF16PtrFromString("TaskbarCreated")
	if err != nil {
		return err
	}
	message, _, registerErr := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(name)))
	if message == 0 {
		return fmt.Errorf("注册 TaskbarCreated 消息失败：%w", registerErr)
	}
	if posted, _, postErr := procPostMessageW.Call(window, message, 0, 0); posted == 0 {
		return fmt.Errorf("向 Codex 通知区域图标窗口发送 TaskbarCreated 失败：%w", postErr)
	}
	return nil
}

func waitForCodexNotifyIconHost(ctx context.Context) (uintptr, error) {
	deadline := time.Now().Add(codexNotifyIconWindowTimeout)
	for {
		if window := findCodexNotifyIconHost(); window != 0 {
			return window, nil
		}
		if time.Now().After(deadline) {
			return 0, errors.New("没有找到 Codex 的通知区域图标窗口")
		}
		if err := waitContext(ctx, codexNotifyIconWindowInterval); err != nil {
			return 0, err
		}
	}
}

func findCodexNotifyIconHost() uintptr {
	codexNotifyIconMutex.Lock()
	defer codexNotifyIconMutex.Unlock()
	codexNotifyIconFound = 0
	_, _, _ = procEnumWindows.Call(codexNotifyIconCallback, 0)
	return codexNotifyIconFound
}

func windowClassName(window uintptr) string {
	buffer := make([]uint16, 256)
	length, _, _ := procGetClassNameW.Call(
		window,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if length == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:length])
}

func windowProcessImagePath(window uintptr) string {
	var processID uint32
	_, _, _ = procGetWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&processID)))
	if processID == 0 {
		return ""
	}
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(processID))
	if handle == 0 {
		return ""
	}
	defer syscall.CloseHandle(syscall.Handle(handle))
	buffer := make([]uint16, codexNotifyIconPathBufferSize)
	size := uint32(len(buffer))
	queried, _, _ := procQueryFullProcessImageNameW.Call(
		handle,
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if queried == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:size])
}
