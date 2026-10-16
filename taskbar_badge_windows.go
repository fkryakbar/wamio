//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

var (
	taskbarBadgeMu               = sync.Mutex{}
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindow                = user32.NewProc("GetWindow")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowText            = user32.NewProc("GetWindowTextW")
	procCreateIcon               = user32.NewProc("CreateIcon")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
)

const gwOwner = 4

const (
	comAlreadyInitialized = uint32(1)          // S_FALSE
	comDifferentMode      = uint32(0x80010106) // RPC_E_CHANGED_MODE
)

type taskbarList3VTable struct {
	QueryInterface, AddRef, Release                       uintptr
	HrInit, AddTab, DeleteTab, ActivateTab, SetActiveAlt  uintptr
	MarkFullscreenWindow                                  uintptr
	SetProgressValue, SetProgressState                    uintptr
	RegisterTab, UnregisterTab, SetTabOrder, SetTabActive uintptr
	ThumbBarAddButtons, ThumbBarUpdateButtons             uintptr
	ThumbBarSetImageList, SetOverlayIcon                  uintptr
	SetThumbnailTooltip, SetThumbnailClip                 uintptr
}

type taskbarList3 struct {
	VTable *taskbarList3VTable
}

func currentProcessWindow() uintptr {
	currentPID := windows.GetCurrentProcessId()
	var result uintptr
	var fallback uintptr
	callback := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var processID uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&processID)))
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
		if visible == 0 || owner != 0 {
			return 1
		}
		var titleBuffer [256]uint16
		length, _, _ := procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&titleBuffer[0])), uintptr(len(titleBuffer)))
		title := windows.UTF16ToString(titleBuffer[:length])
		if processID == currentPID {
			result = hwnd
			return 0
		}
		if fallback == 0 && strings.Contains(strings.ToLower(title), "wamio") {
			fallback = hwnd
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	runtime.KeepAlive(callback)
	if result == 0 {
		result = fallback
	}
	return result
}

func failedHRESULT(value uintptr) bool {
	return int32(value) < 0
}

func setTaskbarBadge(count int) error {
	taskbarBadgeMu.Lock()
	defer taskbarBadgeMu.Unlock()

	hwnd := currentProcessWindow()
	if hwnd == 0 {
		return fmt.Errorf("Wamio window handle not found")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	comInitialized := false
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		oleError, ok := err.(*ole.OleError)
		if !ok || (uint32(oleError.Code()) != comAlreadyInitialized && uint32(oleError.Code()) != comDifferentMode) {
			return fmt.Errorf("initialize taskbar COM: %w", err)
		}
		comInitialized = uint32(oleError.Code()) == comAlreadyInitialized
	} else {
		comInitialized = true
	}
	if comInitialized {
		defer ole.CoUninitialize()
	}

	classID := ole.NewGUID("{56FDF344-FD6D-11D0-958A-006097C9A090}")
	interfaceID := ole.NewGUID("{EA1AFB91-9E28-4B86-90E9-9E9F8A5EEA84}")
	unknown, err := ole.CreateInstance(classID, interfaceID)
	if err != nil {
		return fmt.Errorf("create taskbar interface: %w", err)
	}
	defer unknown.Release()
	taskbar := (*taskbarList3)(unsafe.Pointer(unknown))
	result, _, _ := syscall.SyscallN(taskbar.VTable.HrInit, uintptr(unsafe.Pointer(taskbar)))
	if failedHRESULT(result) {
		return fmt.Errorf("initialize taskbar list: HRESULT 0x%08X", uint32(result))
	}

	var icon uintptr
	if count > 0 {
		andMask, xorMask := renderTaskbarBadge(count)
		icon, _, err = procCreateIcon.Call(
			0, taskbarBadgeSize, taskbarBadgeSize, 1, 32,
			uintptr(unsafe.Pointer(&andMask[0])), uintptr(unsafe.Pointer(&xorMask[0])),
		)
		runtime.KeepAlive(andMask)
		runtime.KeepAlive(xorMask)
		if icon == 0 {
			return fmt.Errorf("create taskbar badge icon: %w", err)
		}
		defer procDestroyIcon.Call(icon)
	}
	description, _ := windows.UTF16PtrFromString(fmt.Sprintf("%d pesan belum dibaca", count))
	result, _, _ = syscall.SyscallN(
		taskbar.VTable.SetOverlayIcon,
		uintptr(unsafe.Pointer(taskbar)), hwnd, icon, uintptr(unsafe.Pointer(description)),
	)
	if failedHRESULT(result) {
		return fmt.Errorf("set taskbar badge: HRESULT 0x%08X", uint32(result))
	}
	return nil
}
