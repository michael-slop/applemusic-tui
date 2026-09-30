//go:build windows

// winwindowprobe measures how Chrome behaves on Windows in each of the window
// states amtui can put its browser in: where the launch flags place it, and
// whether the page stays visible and keeps animating and playing audio when
// parked offscreen, minimized, or parked as a tool window (no taskbar button).
//
// Chrome runs on a private, never-shown Win32 desktop, so nothing appears on
// the user's screen. The page plays a silent looping WAV, so nothing is heard.
//
//	go run ./bench/winwindowprobe
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procCreateDesktopW     = user32.NewProc("CreateDesktopW")
	procCloseDesktop       = user32.NewProc("CloseDesktop")
	procEnumDesktopWindows = user32.NewProc("EnumDesktopWindows")
	procGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible    = user32.NewProc("IsWindowVisible")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procGetWindowLongPtrW  = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW  = user32.NewProc("SetWindowLongPtrW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procGetWindow          = user32.NewProc("GetWindow")
	procGetClassNameW      = user32.NewProc("GetClassNameW")
)

const (
	gwlExStyle       = ^uintptr(19) // -20
	wsExToolWindow   = 0x00000080
	wsExAppWindow    = 0x00040000
	swHide           = 0
	swShowNoActivate = 4
	gwOwner          = 4
	desktopAllAccess = 0x01FF
)

const chromePath = `C:\Program Files\Google\Chrome\Application\chrome.exe`

// page counts animation frames and timer ticks per second and reports how far
// a silent looping audio element has played.
const page = `data:text/html,<script>
const sr=8000,n=sr*30;const b=new ArrayBuffer(44+n*2),v=new DataView(b);
const w=(o,s)=>{for(let i=0;i<s.length;i++)v.setUint8(o+i,s.charCodeAt(i))};
w(0,'RIFF');v.setUint32(4,36+n*2,true);w(8,'WAVEfmt ');v.setUint32(16,16,true);
v.setUint16(20,1,true);v.setUint16(22,1,true);v.setUint32(24,sr,true);v.setUint32(28,sr*2,true);
v.setUint16(32,2,true);v.setUint16(34,16,true);w(36,'data');v.setUint32(40,n*2,true);
const a=new Audio(URL.createObjectURL(new Blob([b],{type:'audio/wav'})));a.loop=true;a.play();
window.__a=a;window.__raf=0;window.__tick=0;
(function f(){window.__raf++;requestAnimationFrame(f)})();
setInterval(()=>window.__tick++,100);
</script>`

type sample struct {
	Visibility string  `json:"v"`
	RAF        int     `json:"r"`
	Tick       int     `json:"t"`
	AudioTime  float64 `json:"a"`
	Paused     bool    `json:"p"`
}

func measure(ctx context.Context, label string) {
	var s0, s1 sample
	js := `({v:document.visibilityState,r:window.__raf,t:window.__tick,a:window.__a.currentTime,p:window.__a.paused})`
	must(chromedp.Run(ctx, chromedp.Evaluate(js, &s0)))
	time.Sleep(2 * time.Second)
	must(chromedp.Run(ctx, chromedp.Evaluate(js, &s1)))
	fmt.Printf("%-28s visibility=%-8s rAF/s=%5.1f timer/s=%4.1f audio+%.2fs paused=%v\n",
		label, s1.Visibility, float64(s1.RAF-s0.RAF)/2, float64(s1.Tick-s0.Tick)/2,
		s1.AudioTime-s0.AudioTime, s1.Paused)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
}

// topWindows returns Chrome's unowned, visible top-level windows for pid on
// the given desktop.
func topWindows(desk windows.Handle, pid uint32) []windows.HWND {
	var out []windows.HWND
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var p uint32
		procGetWindowThreadPID.Call(uintptr(h), uintptr(unsafe.Pointer(&p)))
		vis, _, _ := procIsWindowVisible.Call(uintptr(h))
		owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner)
		var cls [64]uint16
		procGetClassNameW.Call(uintptr(h), uintptr(unsafe.Pointer(&cls[0])), 64)
		if p == pid && vis != 0 && owner == 0 && windows.UTF16ToString(cls[:]) == "Chrome_WidgetWin_1" {
			out = append(out, h)
		}
		return 1
	})
	procEnumDesktopWindows.Call(uintptr(desk), cb, 0)
	return out
}

func rect(h windows.HWND) windows.Rect {
	var r windows.Rect
	procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r)))
	return r
}

func main() {
	name, _ := windows.UTF16PtrFromString("amtui-winwindowprobe")
	d, _, err := procCreateDesktopW.Call(uintptr(unsafe.Pointer(name)), 0, 0, 0, desktopAllAccess, 0)
	if d == 0 {
		must(fmt.Errorf("CreateDesktop: %w", err))
	}
	desk := windows.Handle(d)
	defer procCloseDesktop.Call(d)

	profile, _ := os.MkdirTemp("", "amtui-probe-profile")
	defer os.RemoveAll(profile)
	// The same launch flags amtui's engine uses for its hidden browser.
	cmd := exec.Command(chromePath,
		"--remote-debugging-port=9337", "--user-data-dir="+filepath.Clean(profile),
		"--no-first-run", "--no-default-browser-check",
		"--autoplay-policy=no-user-gesture-required",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-ipc-flooding-protection",
		"--disable-features=IntensiveWakeUpThrottling",
		"--window-position=-32000,-32000", "--window-size=1000,700",
		"about:blank")
	deskName, _ := windows.UTF16PtrFromString("amtui-winwindowprobe")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	// exec has no STARTUPINFO.lpDesktop knob; start through CreateProcess.
	pid := startOnDesktop(cmd, deskName)
	defer func() {
		p, err := os.FindProcess(int(pid))
		if err == nil {
			p.Kill()
		}
	}()

	actx, acancel := chromedp.NewRemoteAllocator(context.Background(), "http://127.0.0.1:9337")
	defer acancel()
	// Attach to the window's own tab: a context without a target opens a new
	// background tab, and a background tab always reports hidden.
	var ctx context.Context
	var cancel context.CancelFunc
	for i := 0; i < 50 && ctx == nil; i++ {
		time.Sleep(200 * time.Millisecond)
		bctx, bcancel := chromedp.NewContext(actx)
		targets, err := chromedp.Targets(bctx)
		for _, t := range targets {
			if err == nil && t.Type == "page" {
				ctx, cancel = chromedp.NewContext(actx, chromedp.WithTargetID(t.TargetID))
				break
			}
		}
		if ctx == nil {
			bcancel()
		}
	}
	if ctx == nil {
		must(fmt.Errorf("chrome never answered on :9337"))
	}
	defer cancel()
	must(chromedp.Run(ctx, chromedp.Navigate(page)))
	time.Sleep(time.Second)

	wins := topWindows(desk, pid)
	fmt.Printf("chrome top-level windows: %d\n", len(wins))
	for _, h := range wins {
		r := rect(h)
		fmt.Printf("launch rect: (%d,%d)-(%d,%d)\n", r.Left, r.Top, r.Right, r.Bottom)
	}
	measure(ctx, "launched (flags)")

	setBounds := func(b *cdpbrowser.Bounds) {
		must(chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			id, _, err := cdpbrowser.GetWindowForTarget().Do(ctx)
			if err != nil {
				return err
			}
			return cdpbrowser.SetWindowBounds(id, b).Do(ctx)
		})))
	}
	park := &cdpbrowser.Bounds{Left: -32000, Top: -32000, Width: 1000, Height: 700, WindowState: cdpbrowser.WindowStateNormal}

	setBounds(park)
	for _, h := range wins {
		r := rect(h)
		fmt.Printf("parked rect: (%d,%d)-(%d,%d)\n", r.Left, r.Top, r.Right, r.Bottom)
	}
	measure(ctx, "parked (CDP)")

	setBounds(&cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateMinimized})
	measure(ctx, "minimized (CDP)")
	setBounds(&cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateNormal})
	setBounds(park)
	measure(ctx, "restored + re-parked")

	for _, h := range wins {
		ex, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlExStyle)
		procShowWindow.Call(uintptr(h), swHide)
		procSetWindowLongPtrW.Call(uintptr(h), gwlExStyle, (ex|wsExToolWindow)&^wsExAppWindow)
		procShowWindow.Call(uintptr(h), swShowNoActivate)
		after, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlExStyle)
		fmt.Printf("tool window style set: %v\n", after&wsExToolWindow != 0)
		r := rect(h)
		fmt.Printf("tool window rect: (%d,%d)-(%d,%d)\n", r.Left, r.Top, r.Right, r.Bottom)
	}
	measure(ctx, "parked tool window")
	setBounds(park)
	for _, h := range wins {
		after, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlExStyle)
		fmt.Printf("tool style after CDP re-park: %v\n", after&wsExToolWindow != 0)
	}
	measure(ctx, "tool window re-parked")
}

func startOnDesktop(cmd *exec.Cmd, desk *uint16) uint32 {
	cmdline := windows.ComposeCommandLine(cmd.Args)
	argv, _ := windows.UTF16PtrFromString(cmdline)
	app, _ := windows.UTF16PtrFromString(cmd.Path)
	si := &windows.StartupInfo{Desktop: desk}
	si.Cb = uint32(unsafe.Sizeof(*si))
	var pi windows.ProcessInformation
	must(windows.CreateProcess(app, argv, nil, nil, false, 0, nil, nil, si, &pi))
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return pi.ProcessId
}
