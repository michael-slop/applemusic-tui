package main

// A quiet watchdog for "the animation froze for a moment". A replay of the
// frame loop showed amtui building a frame in 2-3 ms, but two stalls cannot be
// seen from inside it: the terminal blocking a write (the screen freezes while
// the UI loop runs on), and the frame tick itself arriving late. Each is timed
// where it happens and, past a threshold, logged to amtui.log — at most one
// line a second, nothing at all when everything is smooth.

import (
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"
)

const (
	slowWrite = 150 * time.Millisecond // one terminal write this long is a visible freeze
	lateTick  = 300 * time.Millisecond // a 30 fps tick this late is ten frames lost
)

var lastSlowLog atomic.Int64 // unix nanos of the last line written

// slowLog writes one watchdog line, at most one a second.
func slowLog(format string, args ...any) {
	now := time.Now().UnixNano()
	if last := lastSlowLog.Load(); now-last < int64(time.Second) || !lastSlowLog.CompareAndSwap(last, now) {
		return
	}
	log.Printf("slow: "+format, args...)
}

// timedOutput is the terminal Bubble Tea writes frames to, with a stopwatch on
// each write. Read, Close and Fd come from the *os.File, so Bubble Tea still
// sees a terminal (it asks for exactly those).
type timedOutput struct{ *os.File }

func (t timedOutput) Write(p []byte) (int, error) {
	start := time.Now()
	n, err := t.File.Write(p)
	noteWrite(len(p), time.Since(start))
	return n, err
}

func noteWrite(size int, took time.Duration) {
	if took >= slowWrite {
		slowLog("a %s terminal write took %v (the screen held still that long)", kb(size), took.Round(time.Millisecond))
	}
}

// noteTick is called with the time since the previous frame tick while
// frames are meant to run at full rate.
func noteTick(gap time.Duration, mode string) {
	if gap >= lateTick {
		slowLog("a frame tick came %v after the last (mode %s)", gap.Round(time.Millisecond), mode)
	}
}

func kb(n int) string { return fmt.Sprintf("%.1f KB", float64(n)/1024) }

// vizModeName is the visualizer mode's name (bars, torus, flames...).
func (m model) vizModeName() string {
	if l := vizModeList(); m.vizMode >= 0 && m.vizMode < len(l) {
		return l[m.vizMode]
	}
	return "?"
}
