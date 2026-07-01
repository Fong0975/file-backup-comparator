//go:build windows

package ui

import (
	"os"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
)

// nativeDialogClass is the window class Windows uses for the common
// SHBrowseForFolder / GetOpenFileName dialogs opened by github.com/sqweek/dialog.
// That package creates them with no owner window, so the OS doesn't enforce
// any Z-order relationship to our own window -- which is what lets them open
// behind it. There's no public API on that package to set an owner, so
// instead we poll briefly right after launching one for a newly-created
// top-level window of this class belonging to our own process, and force it
// to the foreground as soon as it appears.
const nativeDialogClass = "#32770"

// bringNativeDialogToFront should be called just before opening a dialog via
// github.com/sqweek/dialog, so the dialog ends up in front of the main
// window instead of behind it.
func bringNativeDialogToFront() {
	pid := uint32(os.Getpid())
	deadline := time.Now().Add(2 * time.Second)
	go func() {
		for time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			if hwnd := findOwnDialogWindow(pid); hwnd != 0 {
				_, _, _ = procSetForegroundWindow.Call(hwnd)
				_, _, _ = procBringWindowToTop.Call(hwnd)
				return
			}
		}
	}()
}

func findOwnDialogWindow(pid uint32) uintptr {
	var found uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}

		var winPid uint32
		_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&winPid)))
		if winPid != pid {
			return 1
		}

		buf := make([]uint16, 256)
		n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n == 0 {
			return 1
		}
		if string(utf16.Decode(buf[:n])) == nativeDialogClass {
			found = hwnd
			return 0 // stop enumeration
		}
		return 1
	})
	_, _, _ = procEnumWindows.Call(cb, 0)
	return found
}
