package main

import (
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/k1y0miiii/applemusic-tui/engine"
)

// Browser sleep: the hidden Chrome costs ~800 MB whether or not anything is
// playing. After a stretch of being paused, amtui snapshots the queue, closes
// the browser, and brings it back on the next key or media key that needs it.

const defaultSleepAfter = 10 * time.Minute

const asleepNote = "browser asleep to save memory · space wakes it"

type (
	sleptMsg struct {
		snap engine.Snapshot
		err  error
	}
	wakeMsg struct{ intent wakeIntent }
)

// wakeIntent is what to do once the browser is back.
type wakeIntent struct {
	play bool
	jump int // queue index to start from, or -1
}

// sleepAfterFromConfig reads browser.sleep_after_minutes (0 disables); the
// AMTUI_SLEEP_AFTER env var (a Go duration like "30s") overrides it.
func sleepAfterFromConfig(cfg map[string]string) time.Duration {
	if v := os.Getenv("AMTUI_SLEEP_AFTER"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return time.Duration(configInt(cfg, "browser.sleep_after_minutes",
		int(defaultSleepAfter/time.Minute))) * time.Minute
}

// setEngine publishes the current engine to the MPRIS adapter.
func (m *model) setEngine(e *engine.Engine) {
	m.eng = e
	if m.engRef != nil {
		m.engRef.Store(e)
	}
}

// maybeSleep is called after each successful state refresh.
func (m *model) maybeSleep() tea.Cmd {
	idle := m.eng != nil && !m.st.Playing && !m.st.Initializing &&
		!m.initPending && m.loading == "" && m.sleepAfter > 0
	if !idle {
		m.pausedSince = time.Time{}
		return nil
	}
	if m.pausedSince.IsZero() {
		m.pausedSince = time.Now()
		return nil
	}
	if time.Since(m.pausedSince) < m.sleepAfter {
		return nil
	}
	eng := m.eng
	m.setEngine(nil)
	m.asleep, m.sleepInFlight, m.pausedSince = true, true, time.Time{}
	m.note, m.noteAt = asleepNote, m.t
	return func() tea.Msg {
		snap, err := eng.Snapshot()
		eng.Sleep()
		return sleptMsg{snap, err}
	}
}

// wake starts the browser again; the intent runs once it is connected.
func (m *model) wake(intent wakeIntent) tea.Cmd {
	if intent.play || intent.jump >= 0 || m.wakeIntent == nil {
		m.wakeIntent = &intent
	}
	if m.waking {
		return nil
	}
	m.note, m.noteAt = "waking Apple Music…", m.t
	if m.sleepInFlight {
		m.wakeQueued = true // sleptMsg starts the connect once the old browser is gone
		return nil
	}
	m.waking = true
	return connectCmd(m.statusCh)
}

// restoreAfterWake reloads the snapshot into the new engine.
func (m *model) restoreAfterWake(eng *engine.Engine) tea.Cmd {
	snap, intent := m.snap, m.wakeIntent
	m.snap, m.wakeIntent = nil, nil
	m.asleep, m.waking = false, false
	if m.note == asleepNote || m.note == "waking Apple Music…" {
		m.note = ""
	}
	if snap == nil || snap.Empty() {
		return nil
	}
	s, play := *snap, false
	if intent != nil {
		play = intent.play
		if intent.jump >= 0 && intent.jump < len(s.IDs) {
			s.Pos, s.Time, play = intent.jump, 0, true
		}
	}
	return func() tea.Msg {
		if err := eng.Restore(s, play); err != nil {
			return noteMsg("couldn't restore the queue: " + err.Error())
		}
		return nil
	}
}

// asleepKey handles a key while the browser sleeps: UI-only keys work as
// usual (ok=false lets the caller carry on), anything that needs the player
// wakes it.
func (m *model) asleepKey(key string) (tea.Cmd, bool) {
	switch key {
	case "up", "down", "k", "j", "tab", "shift+tab", "?", "v", "t", "esc",
		"pgup", "pgdown", "home", "end":
		return nil, false
	case " ":
		return m.wake(wakeIntent{play: true, jump: -1}), true
	case "enter":
		if m.focus == focusQueue {
			return m.wake(wakeIntent{play: true, jump: m.selIdx}), true
		}
	}
	return m.wake(wakeIntent{jump: -1}), true
}

func listenWake(ch chan wakeIntent) tea.Cmd {
	return func() tea.Msg { return wakeMsg{<-ch} }
}
