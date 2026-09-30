package main

import (
	"math"
	"testing"
	"time"

	"github.com/k1y0miiii/applemusic-tui/fx"
)

func settle(m *model, frames int) {
	for range frames {
		m.advanceMotion()
	}
}

func TestMotionFollowsTheMusic(t *testing.T) {
	loud := [32]float64{}
	for i := range loud {
		loud[i] = 0.8
	}
	cases := []struct {
		name       string
		playing    bool
		bands      [32]float64
		reactivity float64
		want       float64
	}{
		{"paused", false, loud, 0.5, motionIdle},
		{"loud song", true, loud, 0.5, 1},
		{"silent stretch", true, [32]float64{}, 0.5, motionIdle},
		{"reactivity 0 plays at original speed", true, [32]float64{}, 0, 1},
		{"paused at reactivity 0 is still still", false, loud, 0, motionIdle},
	}
	for _, c := range cases {
		m := model{reactivity: c.reactivity}
		m.st.Playing, m.vizBands = c.playing, c.bands
		settle(&m, 150) // 5 s
		if got := m.motionSpeed(); math.Abs(got-c.want) > 0.01 {
			t.Errorf("%s: speed %.3f, want %.3f", c.name, got, c.want)
		}
	}
}

func TestMotionEasesRatherThanSnapping(t *testing.T) {
	m := model{reactivity: 0.5}
	m.st.Playing = true
	for i := range m.vizBands {
		m.vizBands[i] = 0.8
	}
	settle(&m, 150)
	m.st.Playing = false
	settle(&m, 3) // a tenth of a second after pausing
	if got := m.motionSpeed(); got < 0.5 {
		t.Fatalf("pausing should ease down over ~0.5 s, not snap: %.2f after 0.1 s", got)
	}
}

// End to end: an effect barely changes over a second while paused, and
// clearly changes while a song plays.
func TestFxAlmostStillWhenPaused(t *testing.T) {
	changed := func(playing bool) int {
		m := model{w: 120, h: 35, phase: phaseReady, st: demoState(), reactivity: 0.5, vizLive: true}
		m.st.Playing = playing
		for i := range m.vizTargets {
			m.vizTargets[i] = 0.8
		}
		for mode, n := range vizModeList() {
			if n == "tunnel" {
				m.vizMode = mode
			}
		}
		now := time.Now()
		m.stepFrame(now)
		_ = m.View()
		// Real music changes every frame; a constant spectrum would (by the
		// torus blueprint) draw a still picture even while playing.
		song := func(i int) {
			if playing {
				for b := range m.vizTargets {
					m.vizTargets[b] = 0.5 + 0.4*math.Sin(float64(i)*0.3+float64(b)*0.7)
				}
			}
		}
		for i := range 150 {
			song(i)
			m.stepFrame(now) // let the speed settle
		}
		a := fx.Render(m.fx.eff, m.fx.cols, m.fx.rows, 3)
		for i := range 30 {
			song(150 + i)
			m.stepFrame(now)
		}
		b := fx.Render(m.fx.eff, m.fx.cols, m.fx.rows, 3)
		n := 0
		for i := range min(len(a), len(b)) {
			if a[i] != b[i] {
				n++
			}
		}
		return n
	}
	paused, playing := changed(false), changed(true)
	if paused*5 > playing {
		t.Fatalf("paused should look nearly still next to playing: %d vs %d cells changed in 1 s", paused, playing)
	}
}
