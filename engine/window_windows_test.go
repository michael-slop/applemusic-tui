//go:build windows

package engine

import (
	"context"
	"testing"

	cdpbrowser "github.com/chromedp/cdproto/browser"
)

func TestToolWindowStyleDropsAppWindowAndIsIdempotent(t *testing.T) {
	got := toolWindowStyle(wsExAppWindow | 0x100)
	if got != wsExToolWindow|0x100 {
		t.Fatalf("toolWindowStyle = %#x, want tool window without app window", got)
	}
	if toolWindowStyle(got) != got {
		t.Fatal("toolWindowStyle is not idempotent; repeated parks would blink the window")
	}
}

func TestWindowsControllerParksInsteadOfMinimizing(t *testing.T) {
	var bounds []cdpbrowser.Bounds
	origSet, origHide := setWindowBounds, hideFromShell
	setWindowBounds = func(_ context.Context, b *cdpbrowser.Bounds) error {
		bounds = append(bounds, *b)
		return nil
	}
	var hidden []int
	hideFromShell = func(pid int) { hidden = append(hidden, pid) }
	t.Cleanup(func() { setWindowBounds, hideFromShell = origSet, origHide })

	w := defaultWindowController(42)
	if err := w.parkOffscreen(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := w.minimize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := w.ensurePlayable(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, b := range bounds {
		if b != *parkedWindowBounds() {
			t.Fatalf("window moved to %+v, want only the parked bounds (a minimized page is hidden)", b)
		}
	}
	if len(bounds) != 2 || len(hidden) != 2 || hidden[0] != 42 {
		t.Fatalf("bounds=%d hides=%v, want park+hide on parkOffscreen and minimize, nothing on ensurePlayable", len(bounds), hidden)
	}

	hidden = nil
	if err := defaultWindowController(0).parkOffscreen(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 0 {
		t.Fatal("unknown pid must not enumerate windows")
	}
}
