package main

import (
	"errors"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/k1y0miiii/applemusic-tui/engine"
	"github.com/k1y0miiii/applemusic-tui/mpris"
)

// mprisControls adapts the engine to what MPRIS needs. Quit goes through a
// channel rather than calling os.Exit, so the desktop asking us to quit still
// runs the same shutdown as pressing q — the browser has to be closed.
type mprisControls struct {
	ref  *atomic.Pointer[engine.Engine] // nil engine = browser asleep
	quit chan struct{}
	wake chan wakeIntent
}

var errAsleep = errors.New("browser asleep")

// eng returns the live engine, or asks the UI to wake the browser (running
// intent once it is back) and returns nil.
func (c mprisControls) eng(intent wakeIntent) *engine.Engine {
	if e := c.ref.Load(); e != nil {
		return e
	}
	select {
	case c.wake <- intent:
	default: // a wake is already pending
	}
	return nil
}

func (c mprisControls) PlayPause() error {
	if e := c.eng(wakeIntent{play: true, jump: -1}); e != nil {
		return e.PlayPause()
	}
	return nil
}
func (c mprisControls) Next() error {
	if e := c.eng(wakeIntent{play: true, jump: -1}); e != nil {
		return e.Next()
	}
	return nil
}
func (c mprisControls) Prev() error {
	if e := c.eng(wakeIntent{play: true, jump: -1}); e != nil {
		return e.Prev()
	}
	return nil
}
func (c mprisControls) SeekTo(d time.Duration) error {
	if e := c.ref.Load(); e != nil {
		return e.SeekTo(d)
	}
	return errAsleep
}
func (c mprisControls) SetVolume(v int) error {
	if e := c.ref.Load(); e != nil {
		return e.SetVolume(v)
	}
	return errAsleep
}

func (c mprisControls) Quit() {
	select {
	case c.quit <- struct{}{}:
	default: // a quit is already on its way
	}
}

// mprisState converts what the UI polls into what the desktop shows.
func mprisState(st engine.State) mpris.State {
	return mpris.State{
		TrackID:  st.Now.ID,
		Title:    st.Now.Title,
		Artist:   st.Now.Artist,
		Album:    st.Now.Album,
		ArtURL:   artworkURL(st.Now.Art, 512),
		Duration: st.Dur,
		Position: st.Pos,
		Playing:  st.Playing,
		Volume:   st.Volume,
		Repeat:   st.Repeat,
		Shuffle:  st.Shuffle,
	}
}

// listenMPRISQuit turns a Quit from the desktop into a message, so shutdown
// runs through Update like any other quit rather than from a D-Bus goroutine.
func listenMPRISQuit(ch chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-ch
		return mprisQuitMsg{}
	}
}
