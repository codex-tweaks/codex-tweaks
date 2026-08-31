//go:build windows

package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Codex owns its notification area icon through a dedicated message window and adds the icon once,
// while it starts up. Stopping Codex with taskkill leaves the shell holding the record of the dead
// instance, and the shell refuses the add of the freshly started instance while that record is still
// around, so Codex comes back without an icon and never tries again.
//
// Workaround: the shell asks every application to add its icons again by sending TaskbarCreated, and
// Codex honours that message. Posting it to the icon host window of the new instance makes Codex add
// the icon again, and the shell drops the stale record while doing so. The message reaches that one
// window, so no other notification area application is involved. This works around shell behaviour
// Codex Tweaks cannot fix: the refusal happens inside the shell, and one process cannot delete the
// notification area record of another one (a cross process NIM_DELETE returns E_FAIL).
const (
	// Codex hosts the icon on OwlElectron_NotifyIconHostWindow. Only the suffix is matched so a
	// different vendor prefix keeps working, and the window still has to belong to the instance that
	// was just started.
	codexNotifyIconHostClassSuffix = "NotifyIconHostWindow"
	codexNotifyIconAppearTimeout   = 20 * time.Second
	codexNotifyIconPollInterval    = 250 * time.Millisecond
	// The host window shows up slightly before Codex adds the icon, so the repair has to arrive
	// after that refused add instead of before it.
	codexNotifyIconSettleDelay = 2 * time.Second
	// PostMessage only proves that the message entered the queue. A second targeted pulse covers a
	// slower first NIM_ADD without inspecting or manipulating the shell's private tray state.
	codexNotifyIconPulseCount = 2
	codexNotifyIconPulseDelay = 2 * time.Second
	// The repair outlives the call that started Codex, so it carries its own deadline.
	codexNotifyIconRepairTimeout = 60 * time.Second

	processQueryLimitedInformation = 0x1000
	codexNotifyIconPathBufferSize  = 1024
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
)

// codexNotifyIconTarget identifies the Codex instance whose icon has to come back. The packaged
// activation reports the process it started, the unpackaged launch only knows the executable it ran,
// and either one has to be specific enough to skip an unrelated ChatGPT.exe.
type codexNotifyIconTarget struct {
	processID      uint32
	executablePath string
}

func (target codexNotifyIconTarget) describe() string {
	if target.processID != 0 {
		return fmt.Sprintf("pid %d", target.processID)
	}
	if target.executablePath != "" {
		return target.executablePath
	}
	return "未知实例"
}

func (target codexNotifyIconTarget) identifiesInstance() bool {
	return target.processID != 0 || target.executablePath != ""
}

func (target codexNotifyIconTarget) matches(processID uint32, imagePath string) bool {
	if target.processID != 0 {
		return processID == target.processID
	}
	if target.executablePath == "" || imagePath == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(imagePath), filepath.Clean(target.executablePath))
}

// codexNotifyIconRepair keeps the window lookup and the message behind function fields so the whole
// sequence can be tested without a running Codex.
type codexNotifyIconRepair struct {
	find       func(codexNotifyIconTarget) uintptr
	post       func(uintptr) error
	appear     time.Duration
	poll       time.Duration
	settle     time.Duration
	pulseDelay time.Duration
}

func (repair codexNotifyIconRepair) run(ctx context.Context, target codexNotifyIconTarget) error {
	if !target.identifiesInstance() {
		return fmt.Errorf("没有可以定位 Codex 通知区域图标窗口的进程或路径")
	}
	if err := repair.waitForHostWindow(ctx, target); err != nil {
		return err
	}
	if err := waitContext(ctx, repair.settle); err != nil {
		return err
	}

	// The handle is looked up again instead of reused: Codex can replace the window while the repair
	// waits, and posting to a handle that is already gone would repair nothing. Two pulses are
	// intentional: PostMessage succeeding only means the message was queued, not that Codex's failed
	// first NIM_ADD had already happened or that the icon was restored.
	delivered := false
	var lastError error
	for attempt := range codexNotifyIconPulseCount {
		if attempt > 0 {
			if err := waitContext(ctx, repair.pulseDelay); err != nil {
				if delivered {
					return nil
				}
				return err
			}
		}
		window := repair.find(target)
		if window == 0 {
			lastError = fmt.Errorf("Codex（%s）的通知区域图标窗口消失了", target.describe())
			continue
		}
		if err := repair.post(window); err != nil {
			lastError = err
			continue
		}
		delivered = true
	}
	if delivered {
		return nil
	}
	return lastError
}

func (repair codexNotifyIconRepair) waitForHostWindow(
	ctx context.Context,
	target codexNotifyIconTarget,
) error {
	deadline := time.Now().Add(repair.appear)
	for {
		if repair.find(target) != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("没有找到 Codex（%s）的通知区域图标窗口", target.describe())
		}
		if err := waitContext(ctx, repair.poll); err != nil {
			return err
		}
	}
}

func restoreCodexNotifyIcon(ctx context.Context, target codexNotifyIconTarget) error {
	return codexNotifyIconRepair{
		find:       findCodexNotifyIconHost,
		post:       postTaskbarCreated,
		appear:     codexNotifyIconAppearTimeout,
		poll:       codexNotifyIconPollInterval,
		settle:     codexNotifyIconSettleDelay,
		pulseDelay: codexNotifyIconPulseDelay,
	}.run(ctx, target)
}

var (
	// EnumWindows needs a C callback and Go only allows a limited number of them, so the callback is
	// created once and exchanges its input and its result through the guarded variables below.
	codexNotifyIconMutex    sync.Mutex
	codexNotifyIconWanted   codexNotifyIconTarget
	codexNotifyIconFound    uintptr
	codexNotifyIconCallback = syscall.NewCallback(func(window uintptr, _ uintptr) uintptr {
		if !strings.HasSuffix(windowClassName(window), codexNotifyIconHostClassSuffix) {
			return 1
		}
		processID := windowProcessID(window)
		if processID == 0 {
			return 1
		}
		imagePath := ""
		if codexNotifyIconWanted.processID == 0 {
			imagePath = processImagePath(processID)
		}
		if !codexNotifyIconWanted.matches(processID, imagePath) {
			return 1
		}
		codexNotifyIconFound = window
		return 0
	})
)

func findCodexNotifyIconHost(target codexNotifyIconTarget) uintptr {
	codexNotifyIconMutex.Lock()
	defer codexNotifyIconMutex.Unlock()
	codexNotifyIconWanted = target
	codexNotifyIconFound = 0
	_, _, _ = procEnumWindows.Call(codexNotifyIconCallback, 0)
	return codexNotifyIconFound
}

func postTaskbarCreated(window uintptr) error {
	name, err := syscall.UTF16PtrFromString("TaskbarCreated")
	if err != nil {
		return err
	}
	message, _, registerError := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(name)))
	if message == 0 {
		return fmt.Errorf("注册 TaskbarCreated 消息失败：%w", registerError)
	}
	if posted, _, postError := procPostMessageW.Call(window, message, 0, 0); posted == 0 {
		return fmt.Errorf("向 Codex 通知区域图标窗口发送 TaskbarCreated 失败：%w", postError)
	}
	return nil
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

func windowProcessID(window uintptr) uint32 {
	var processID uint32
	_, _, _ = procGetWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&processID)))
	return processID
}

func processImagePath(processID uint32) string {
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
