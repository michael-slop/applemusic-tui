//go:build windows

package engine

// On Linux chromedp starts Chrome with Pdeathsig, so the browser dies with
// amtui however amtui exits. Windows has no such thing: quit amtui while it is
// still loading, close its terminal, or crash it, and the hidden Chrome keeps
// running on amtui's profile. Every later launch then fails with "Opening in
// existing browser session", because Chrome hands the new window to the
// orphan and exits.
//
// Two halves fix that. tieBrowserToProcess puts each browser amtui starts into
// a kill-on-close job object, the Windows counterpart of Pdeathsig: when amtui
// exits for any reason the job handle closes and the kernel ends the browser.
// clearStaleBrowser clears an orphan an older build (or a hard kill) left
// behind, found the way Chrome itself finds a running instance: its singleton
// message window, titled with the profile path.

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procFindWindowExW = user32.NewProc("FindWindowExW")

	jobOnce sync.Once
	job     windows.Handle
)

// hwndMessage is HWND_MESSAGE, the parent of message-only windows.
const hwndMessage = ^uintptr(2) // (HWND)-3

// browserJob returns amtui's kill-on-close job, created on first use. The
// handle is never closed; process exit closes it, which kills the browser.
func browserJob() windows.Handle {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			log.Printf("browser job: %v", err)
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			log.Printf("browser job: %v", err)
			windows.CloseHandle(h)
			return
		}
		job = h
	})
	return job
}

// tieBrowserToProcess makes the browser die with amtui. Best effort: if it
// fails, the browser still closes on every normal exit path.
func tieBrowserToProcess(pid int) {
	j := browserJob()
	if j == 0 || pid <= 0 {
		return
	}
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		log.Printf("browser job: open pid %d: %v", pid, err)
		return
	}
	defer windows.CloseHandle(p)
	if err := windows.AssignProcessToJobObject(j, p); err != nil {
		log.Printf("browser job: assign pid %d: %v", pid, err)
	}
}

// profileOwner returns the PID of the Chrome running on profile dir, or 0.
func profileOwner(dir string) uint32 {
	cls, _ := windows.UTF16PtrFromString("Chrome_MessageWindow")
	title, _ := windows.UTF16PtrFromString(dir)
	h, _, _ := procFindWindowExW.Call(hwndMessage, 0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)))
	if h == 0 {
		return 0
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(windows.HWND(h), &pid); err != nil {
		return 0
	}
	return pid
}

// liveParentIsAmtui reports whether pid's parent is still a running amtui, so a
// second amtui never kills the first one's browser.
func liveParentIsAmtui(pid uint32) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	names := map[uint32]string{}
	var parent uint32
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		names[e.ProcessID] = windows.UTF16ToString(e.ExeFile[:])
		if e.ProcessID == pid {
			parent = e.ParentProcessID
		}
	}
	self, _ := os.Executable()
	name, ok := names[parent]
	return ok && parent != 0 && strings.EqualFold(name, filepath.Base(self))
}

// clearStaleBrowser ends a browser left running on dir by an amtui that is
// gone, and refuses to start when another amtui is still using the profile.
func clearStaleBrowser(dir string) error {
	pid := profileOwner(dir)
	if pid == 0 {
		return nil
	}
	if liveParentIsAmtui(pid) {
		return fmt.Errorf("another amtui is already running (its browser is pid %d)", pid)
	}
	p, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return fmt.Errorf("a leftover amtui browser (pid %d) holds the profile and cannot be stopped: %w", pid, err)
	}
	defer windows.CloseHandle(p)
	log.Printf("ending leftover browser pid %d on %s", pid, dir)
	if err := windows.TerminateProcess(p, 1); err != nil {
		return fmt.Errorf("stopping leftover amtui browser (pid %d): %w", pid, err)
	}
	windows.WaitForSingleObject(p, 5000)
	// The singleton window goes with the browser process; wait for it so the
	// next launch does not find it and hand off to a dying instance.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && profileOwner(dir) != 0; {
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
