//go:build windows

package engine

// Windows minimizing works like macOS: a minimized Chrome window flips
// document.visibilityState to hidden and stops requestAnimationFrame, and Apple
// Music will not start the next track from a hidden page. So the window is
// parked offscreen and never minimized. Chrome honours -32000,-32000 on Windows
// (no clamping, no flash at launch) and the parked page stays "visible".
//
// A parked window still has a taskbar button and an Alt+Tab entry, and a tiling
// window manager (GlazeWM, Komorebi) tiles it straight back onto the screen.
// Giving it WS_EX_TOOLWINDOW removes all three: the taskbar and Alt+Tab skip
// tool windows, and GlazeWM refuses to manage them. The style only takes
// effect across a hide/show, which is also what makes a tiling manager drop a
// window it already manages, so the window is parked after the style is set.
//
// Measured with bench/winwindowprobe (Chrome 154, Windows 11 26200): parked and
// parked-as-tool-window pages stay visible with animation frames and audio
// running; minimized pages go hidden with zero animation frames.

import (
	"context"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procShowWindow        = user32.NewProc("ShowWindow")
	procGetWindow         = user32.NewProc("GetWindow")
)

const (
	gwlExStyle       = ^uintptr(19) // GWL_EXSTYLE (-20)
	gwOwner          = 4
	swHide           = 0
	swShowNoActivate = 4
	wsExToolWindow   = 0x00000080
	wsExAppWindow    = 0x00040000
)

// toolWindowStyle is the extended style that keeps a window out of the taskbar,
// Alt+Tab and tiling window managers.
func toolWindowStyle(ex uintptr) uintptr {
	return (ex | wsExToolWindow) &^ wsExAppWindow
}

// browserWindows lists pid's visible, unowned Chrome top-level windows: the
// browser frames, not its popups or tooltips.
func browserWindows(pid int) []windows.HWND {
	var out []windows.HWND
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var p uint32
		if _, err := windows.GetWindowThreadProcessId(h, &p); err != nil || int(p) != pid {
			return 1
		}
		if !windows.IsWindowVisible(h) {
			return 1
		}
		if owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner); owner != 0 {
			return 1
		}
		var cls [64]uint16
		if _, err := windows.GetClassName(h, &cls[0], int32(len(cls))); err != nil ||
			windows.UTF16ToString(cls[:]) != "Chrome_WidgetWin_1" {
			return 1
		}
		out = append(out, h)
		return 1
	})
	_ = windows.EnumWindows(cb, unsafe.Pointer(nil))
	return out
}

// hideFromShell turns pid's browser windows into tool windows. Windows that
// already are one are left alone, so repeated parks never blink the window.
var hideFromShell = func(pid int) {
	for _, h := range browserWindows(pid) {
		ex, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlExStyle)
		if ex == toolWindowStyle(ex) {
			continue
		}
		procShowWindow.Call(uintptr(h), swHide)
		procSetWindowLongPtrW.Call(uintptr(h), gwlExStyle, toolWindowStyle(ex))
		procShowWindow.Call(uintptr(h), swShowNoActivate)
	}
}

type windowsWindowController struct{ pid int }

func (w windowsWindowController) parkOffscreen(ctx context.Context) error {
	if w.pid > 0 {
		hideFromShell(w.pid)
	}
	return setWindowBounds(ctx, parkedWindowBounds())
}

// minimize re-parks instead of minimizing: a minimized window is a hidden page,
// and a hidden page cannot start the next track.
func (w windowsWindowController) minimize(ctx context.Context) error {
	return w.parkOffscreen(ctx)
}

// ponytail: nothing to restore — the window is never hidden, so hiddenStall
// cannot fire. Add a recovery here if a parked window ever stalls anyway.
func (windowsWindowController) ensurePlayable(context.Context) error { return nil }

func defaultWindowController(pid int) windowController {
	return windowsWindowController{pid: pid}
}
