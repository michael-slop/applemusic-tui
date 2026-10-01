//go:build windows

package mpris

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeControls struct {
	asleep             atomic.Bool
	plays, nexts, prev atomic.Int32
}

func (f *fakeControls) Asleep() bool               { return f.asleep.Load() }
func (f *fakeControls) PlayPause() error           { f.plays.Add(1); return nil }
func (f *fakeControls) Next() error                { f.nexts.Add(1); return nil }
func (f *fakeControls) Prev() error                { f.prev.Add(1); return nil }
func (f *fakeControls) SeekTo(time.Duration) error { return nil }
func (f *fakeControls) SetVolume(int) error        { return nil }
func (f *fakeControls) Quit()                      {}

// fakeKeys swaps the real RegisterHotKey for a recorder: the real one claims
// the media keys from the whole desktop.
func fakeKeys(t *testing.T) (held func() map[uint32]bool) {
	var mu sync.Mutex
	keys := map[uint32]bool{}
	origReg, origUnreg := registerKey, unregisterKey
	registerKey = func(vk uint32) error { mu.Lock(); keys[vk] = true; mu.Unlock(); return nil }
	unregisterKey = func(vk uint32) { mu.Lock(); delete(keys, vk); mu.Unlock() }
	t.Cleanup(func() { registerKey, unregisterKey = origReg, origUnreg })
	return func() map[uint32]bool {
		mu.Lock()
		defer mu.Unlock()
		out := map[uint32]bool{}
		for k := range keys {
			out[k] = true
		}
		return out
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestMediaKeysAreHeldOnlyWhileTheBrowserSleeps(t *testing.T) {
	held := fakeKeys(t)
	c := &fakeControls{}
	s, err := Publish(c)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * pollEvery)
	if n := len(held()); n != 0 {
		t.Fatalf("awake: %d media keys held; Chrome must keep them", n)
	}
	c.asleep.Store(true)
	waitFor(t, "all three keys claimed while asleep", func() bool {
		h := held()
		return h[vkMediaPlayPause] && h[vkMediaNext] && h[vkMediaPrev]
	})
	c.asleep.Store(false)
	waitFor(t, "keys released on wake", func() bool { return len(held()) == 0 })
	c.asleep.Store(true)
	waitFor(t, "keys claimed again on the next sleep", func() bool { return len(held()) == 3 })
	s.Close()
	if n := len(held()); n != 0 {
		t.Fatalf("Close left %d media keys held", n)
	}
}

func TestMediaKeyPressReachesTheControls(t *testing.T) {
	fakeKeys(t)
	c := &fakeControls{}
	c.asleep.Store(true)
	s, err := Publish(c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// The message a real key press produces, posted to the listener's thread.
	for _, vk := range []uint32{vkMediaPlayPause, vkMediaNext, vkMediaPrev} {
		procPostThreadMessage.Call(uintptr(s.thread), wmHotkey, uintptr(vk), 0)
	}
	waitFor(t, "play/pause, next and prev to reach the controls", func() bool {
		return c.plays.Load() == 1 && c.nexts.Load() == 1 && c.prev.Load() == 1
	})
}

type noSleep struct{ fakeControls }

func (*noSleep) Asleep() {} // wrong shape on purpose: not a Sleeper

func TestControlsThatCannotReportSleepGetNoListener(t *testing.T) {
	fakeKeys(t)
	var c Controls = &noSleep{}
	if _, ok := c.(Sleeper); ok {
		t.Fatal("test setup: noSleep must not be a Sleeper")
	}
	if s, err := Publish(c); err == nil || s != nil {
		t.Fatal("Publish took controls that cannot say when to release the keys")
	}
}
