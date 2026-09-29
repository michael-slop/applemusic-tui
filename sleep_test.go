package main

import (
	"testing"
	"time"

	"github.com/k1y0miiii/applemusic-tui/engine"
)

func TestMaybeSleepWaitsForTheIdleStretch(t *testing.T) {
	m := model{sleepAfter: time.Minute, eng: &engine.Engine{}}
	m.st = engine.State{Authed: true}
	if cmd := m.maybeSleep(); cmd != nil || m.pausedSince.IsZero() {
		t.Fatal("first paused refresh should only start the idle clock")
	}
	if cmd := m.maybeSleep(); cmd != nil {
		t.Fatal("slept before the idle stretch elapsed")
	}
	m.pausedSince = time.Now().Add(-2 * time.Minute)
	if cmd := m.maybeSleep(); cmd == nil || !m.asleep || m.eng != nil {
		t.Fatalf("expected sleep: cmd=%v asleep=%v eng=%v", cmd != nil, m.asleep, m.eng)
	}
}

func TestMaybeSleepNeverWhilePlayingOrDisabled(t *testing.T) {
	m := model{sleepAfter: time.Minute, eng: &engine.Engine{}}
	m.st = engine.State{Playing: true}
	m.pausedSince = time.Now().Add(-time.Hour)
	if m.maybeSleep() != nil || !m.pausedSince.IsZero() {
		t.Fatal("playing must reset the idle clock")
	}
	m = model{sleepAfter: 0, eng: &engine.Engine{}}
	m.pausedSince = time.Now().Add(-time.Hour)
	if m.maybeSleep() != nil {
		t.Fatal("sleep_after_minutes = 0 must disable sleeping")
	}
}

func TestAsleepKeys(t *testing.T) {
	m := model{asleep: true}
	if _, handled := m.asleepKey("down"); handled {
		t.Fatal("navigation should work while asleep")
	}
	m.focus, m.selIdx = focusQueue, 7
	if cmd, handled := m.asleepKey("enter"); !handled || cmd == nil {
		t.Fatal("enter on the queue should wake")
	}
	if m.wakeIntent == nil || m.wakeIntent.jump != 7 || !m.wakeIntent.play {
		t.Fatalf("wake intent = %+v, want jump 7 and play", m.wakeIntent)
	}
}

func TestSleepAfterConfig(t *testing.T) {
	t.Setenv("AMTUI_SLEEP_AFTER", "")
	if got := sleepAfterFromConfig(map[string]string{}); got != defaultSleepAfter {
		t.Fatalf("default = %v", got)
	}
	if got := sleepAfterFromConfig(map[string]string{"browser.sleep_after_minutes": "0"}); got != 0 {
		t.Fatalf("0 minutes = %v, want disabled", got)
	}
	t.Setenv("AMTUI_SLEEP_AFTER", "30s")
	if got := sleepAfterFromConfig(nil); got != 30*time.Second {
		t.Fatalf("env override = %v", got)
	}
}
