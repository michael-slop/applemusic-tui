package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
)

// Bubble Tea only treats its output as a terminal when it is a term.File;
// otherwise it would lose the console modes and window size.
var _ term.File = timedOutput{}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	lastSlowLog.Store(0)
	t.Cleanup(func() { log.SetOutput(prev); lastSlowLog.Store(0) })
	return &buf
}

func TestWatchdogLogsOnlyStalls(t *testing.T) {
	buf := captureLog(t)
	noteWrite(40_000, 2*time.Millisecond)
	noteTick(34*time.Millisecond, "flames")
	if buf.Len() != 0 {
		t.Fatalf("a smooth frame was logged: %q", buf.String())
	}
	noteWrite(60_000, 400*time.Millisecond)
	if !strings.Contains(buf.String(), "terminal write took 400ms") {
		t.Fatalf("a 400 ms write was not logged: %q", buf.String())
	}
}

func TestWatchdogLogsALateTickWithItsMode(t *testing.T) {
	buf := captureLog(t)
	noteTick(700*time.Millisecond, "plasma")
	if !strings.Contains(buf.String(), "700ms") || !strings.Contains(buf.String(), "plasma") {
		t.Fatalf("late tick not logged with its mode: %q", buf.String())
	}
}

func TestWatchdogWritesAtMostOneLineASecond(t *testing.T) {
	buf := captureLog(t)
	for range 50 {
		noteWrite(1000, time.Second)
	}
	if n := strings.Count(buf.String(), "slow:"); n != 1 {
		t.Fatalf("%d lines for a burst of stalls, want 1", n)
	}
}
