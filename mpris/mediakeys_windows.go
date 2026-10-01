//go:build windows

package mpris

// Windows has no MPRIS. While amtui's browser is awake, Chrome's own media
// integration answers the media keys; while it sleeps (sleep.go closes Chrome
// after a long pause) nothing of amtui's is listening, so a media key did
// nothing and only space in the terminal woke it.
//
// So while the browser sleeps, and only then, the play/pause, next and
// previous keys are claimed with RegisterHotKey and routed to the same
// Controls the Linux MPRIS server drives: a press becomes the wake intent
// that pressing space would. The moment the browser is back the keys are
// released, so Chrome — and any other player — gets them as before.

import (
	"errors"
	"log"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Sleeper is what a Controls must also be for media keys to work here: it
// reports whether the browser is asleep.
type Sleeper interface {
	Asleep() bool
}

const (
	vkMediaNext      = 0xB0
	vkMediaPrev      = 0xB1
	vkMediaPlayPause = 0xB3
	wmHotkey         = 0x0312
	wmQuit           = 0x0012
	pmRemove         = 0x0001
	pollEvery        = 250 * time.Millisecond
)

// mediaKeys is the hotkey id of each key; the id is the VK itself.
var mediaKeys = []uint32{vkMediaPlayPause, vkMediaNext, vkMediaPrev}

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey    = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	procPeekMessageW      = user32.NewProc("PeekMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
)

var errNoSleeper = errors.New("media keys: the controls cannot report sleep")

// Indirection for tests: the real calls claim keys from the whole desktop.
var (
	registerKey = func(vk uint32) error {
		if r, _, err := procRegisterHotKey.Call(0, uintptr(vk), 0, uintptr(vk)); r == 0 {
			return err
		}
		return nil
	}
	unregisterKey = func(vk uint32) {
		procUnregisterHotKey.Call(0, uintptr(vk))
	}
)

type Server struct {
	thread uint32
	done   chan struct{}
}

// Publish starts the media-key listener. Controls that cannot report sleep get
// nothing to do: without it the keys would be held while Chrome wants them.
func Publish(c Controls) (*Server, error) {
	sl, ok := c.(Sleeper)
	if !ok {
		return nil, errNoSleeper
	}
	s := &Server{done: make(chan struct{})}
	ready := make(chan struct{})
	go s.run(c, sl, ready)
	<-ready
	return s, nil
}

// run owns the hotkeys: RegisterHotKey binds them to the calling thread's
// message queue, so registering, receiving and releasing all happen here.
func (s *Server) run(c Controls, sl Sleeper, ready chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(s.done)
	s.thread = windows.GetCurrentThreadId()
	var msg [48]byte
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, 0) // create the queue
	close(ready)

	held := false
	defer func() {
		if held {
			release()
		}
	}()
	for {
		switch want := sl.Asleep(); {
		case want && !held:
			held = claim()
		case !want && held:
			release()
			held = false
		}
		for {
			r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, pmRemove)
			if r == 0 {
				break
			}
			// MSG on amd64: hwnd(8) message(4) pad(4) wParam(8) lParam(8) ...
			id := *(*uint32)(unsafe.Pointer(&msg[8]))
			wparam := *(*uintptr)(unsafe.Pointer(&msg[16]))
			if id == wmQuit {
				return
			}
			if id == wmHotkey {
				press(c, uint32(wparam))
			}
		}
		time.Sleep(pollEvery)
	}
}

// claim registers the media keys; it reports whether any were taken.
func claim() bool {
	taken := false
	for _, vk := range mediaKeys {
		if err := registerKey(vk); err != nil {
			log.Printf("media keys: cannot claim 0x%X while asleep: %v", vk, err)
			continue
		}
		taken = true
	}
	return taken
}

func release() {
	for _, vk := range mediaKeys {
		unregisterKey(vk)
	}
}

// press routes a claimed key to the Controls the Linux server uses; asleep,
// each of them turns into a wake intent.
func press(c Controls, vk uint32) {
	var err error
	switch vk {
	case vkMediaPlayPause:
		err = c.PlayPause()
	case vkMediaNext:
		err = c.Next()
	case vkMediaPrev:
		err = c.Prev()
	}
	if err != nil {
		log.Printf("media keys: 0x%X: %v", vk, err)
	}
}

func (s *Server) Update(State) {}

// Close stops the listener and releases any keys it holds.
func (s *Server) Close() {
	if s == nil {
		return
	}
	procPostThreadMessage.Call(uintptr(s.thread), wmQuit, 0, 0)
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}
