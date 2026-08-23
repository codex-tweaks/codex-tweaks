//go:build windows

package core

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	clsctxLocalServer   = 0x4
	rpcEChangedMode     = 0x80010106
	coinitMultithreaded = 0x0
)

var (
	ole32                      = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx         = ole32.NewProc("CoInitializeEx")
	procCoUninitialize         = ole32.NewProc("CoUninitialize")
	procCoCreateInstance       = ole32.NewProc("CoCreateInstance")
	applicationActivationCLSID = syscall.GUID{
		Data1: 0x45ba127d,
		Data2: 0x10a8,
		Data3: 0x46ea,
		Data4: [8]byte{0x8a, 0xb7, 0x56, 0xea, 0x90, 0x78, 0x94, 0x3c},
	}
	applicationActivationIID = syscall.GUID{
		Data1: 0x2e941141,
		Data2: 0x7f97,
		Data3: 0x4756,
		Data4: [8]byte{0xba, 0x1d, 0x9d, 0xec, 0xde, 0x89, 0x4a, 0x3d},
	}
)

type applicationActivationManager struct {
	vtbl *applicationActivationManagerVtbl
}

type applicationActivationManagerVtbl struct {
	queryInterface      uintptr
	addRef              uintptr
	release             uintptr
	activateApplication uintptr
	activateForFile     uintptr
	activateForProtocol uintptr
}

func activatePackagedApplication(appUserModelID, arguments string) (uint32, error) {
	appUserModelIDPointer, err := syscall.UTF16PtrFromString(appUserModelID)
	if err != nil {
		return 0, err
	}
	argumentsPointer, err := syscall.UTF16PtrFromString(arguments)
	if err != nil {
		return 0, err
	}

	// COM apartments are thread-local. Keep the complete activation sequence on
	// one OS thread so the sidecar can safely call it from any goroutine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	initializeResult, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	initialized := !hresultFailed(initializeResult)
	if hresultFailed(initializeResult) && uint32(initializeResult) != rpcEChangedMode {
		return 0, windowsHRESULTError{"初始化 Windows 应用激活服务", uint32(initializeResult)}
	}
	if initialized {
		defer procCoUninitialize.Call()
	}

	var manager *applicationActivationManager
	createResult, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&applicationActivationCLSID)),
		0,
		clsctxLocalServer,
		uintptr(unsafe.Pointer(&applicationActivationIID)),
		uintptr(unsafe.Pointer(&manager)),
	)
	if hresultFailed(createResult) {
		return 0, windowsHRESULTError{"创建 Windows 应用激活服务", uint32(createResult)}
	}
	if manager == nil || manager.vtbl == nil {
		return 0, fmt.Errorf("Windows 应用激活服务返回了空对象")
	}
	defer syscall.SyscallN(manager.vtbl.release, uintptr(unsafe.Pointer(manager)))

	var processID uint32
	activateResult, _, _ := syscall.SyscallN(
		manager.vtbl.activateApplication,
		uintptr(unsafe.Pointer(manager)),
		uintptr(unsafe.Pointer(appUserModelIDPointer)),
		uintptr(unsafe.Pointer(argumentsPointer)),
		0,
		uintptr(unsafe.Pointer(&processID)),
	)
	runtime.KeepAlive(appUserModelIDPointer)
	runtime.KeepAlive(argumentsPointer)
	runtime.KeepAlive(manager)
	if hresultFailed(activateResult) {
		return 0, windowsHRESULTError{"激活 Codex Windows 应用", uint32(activateResult)}
	}
	return processID, nil
}

func hresultFailed(result uintptr) bool {
	return int32(uint32(result)) < 0
}

type windowsHRESULTError struct {
	action string
	code   uint32
}

func (e windowsHRESULTError) Error() string {
	return fmt.Sprintf("%s失败（HRESULT 0x%08X）", e.action, e.code)
}
