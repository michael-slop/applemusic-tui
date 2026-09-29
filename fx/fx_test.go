package fx

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// testPalette is the slop house theme.
var testPalette = Palette{
	Background: RGB{0x05, 0x07, 0x0A},
	Ramp:       [5]RGB{{0x1A, 0x24, 0x30}, {0x3D, 0x8F, 0xA8}, {0x62, 0xE6, 0x70}, {0xC8, 0xFF, 0xD0}, {0xD8, 0xD4, 0xC4}},
	Accent:     RGB{0x62, 0xE6, 0x70}, AccentHi: RGB{0xC8, 0xFF, 0xD0}, AccentLo: RGB{0x3D, 0x8F, 0xA8},
	Text: RGB{0xD8, 0xD4, 0xC4}, Dim: RGB{0xAC, 0xA4, 0xC8}, Faint: RGB{0x5C, 0x64, 0x70},
}

// silence is a paused, reactivity-0 frame: React at rest, no kick.
func silence() Audio {
	var a Audio
	for i := range a.React {
		a.React[i] = 0.5
	}
	a.Bass, a.Mid, a.Treble, a.DT = 0.5, 0.5, 0.5, 1.0/30
	return a
}

// loud is a playing frame at full reactivity with a beat on every 8th frame.
func loud(frame int) Audio {
	a := Audio{Playing: true, Reactivity: 1, DT: 1.0 / 30}
	for i := range a.Bands {
		a.Bands[i] = 0.7
		a.React[i] = 0.5 + 0.5*float64((frame+i)%5)/4
	}
	a.Level, a.Bass, a.Mid, a.Treble = 0.7, 0.9, 0.6, 0.7
	if frame%8 == 0 {
		a.Kick = 1
	}
	return a
}

func frame(e Effect, cols, rows int) string {
	return Render(e, cols, rows, termenv.TrueColor)
}

// conformance checks the contract every effect must meet.
func conformance(t *testing.T, name string) {
	t.Helper()
	for _, size := range [][2]int{{1, 1}, {3, 2}, {20, 8}, {80, 24}, {137, 41}} {
		e := New(name)
		if e == nil {
			t.Fatalf("%s: not registered", name)
		}
		if e.Name() != name {
			t.Fatalf("New(%q).Name() = %q", name, e.Name())
		}
		e.SetPalette(testPalette)
		e.Resize(size[0], size[1])
		for i := range 90 { // three seconds
			e.Step(loud(i))
		}
		out := frame(e, size[0], size[1])
		lines := strings.Split(out, "\n")
		if len(lines) != size[1] {
			t.Fatalf("%s %dx%d: %d lines", name, size[0], size[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != size[0] {
				t.Fatalf("%s %dx%d: line %d is %d cells wide", name, size[0], size[1], i, w)
			}
		}
		// Out-of-range cells must not panic (the renderer never asks, but a
		// resize racing a frame must not crash the TUI).
		_, _, _ = e.Cell(size[0]+5, size[1]+5)
		_, _, _ = e.Cell(-1, -1)
	}

	// It must animate, both on its own and to music.
	e := New(name)
	e.SetPalette(testPalette)
	e.Resize(60, 20)
	for range 30 {
		e.Step(silence())
	}
	a := frame(e, 60, 20)
	for range 30 {
		e.Step(silence())
	}
	if b := frame(e, 60, 20); a == b && !isStill[name] {
		t.Errorf("%s: no idle motion over one second of silence", name)
	}

	// At reactivity 0 the audio fields are all at rest, so two runs with
	// different Bands (which effects must only use scaled by Reactivity)
	// must produce the same frames: the panefx original, untouched.
	x, y := New(name), New(name)
	for _, e := range []Effect{x, y} {
		e.SetPalette(testPalette)
		e.Resize(40, 12)
	}
	for i := range 45 {
		qa, qb := silence(), silence()
		for j := range qb.Bands {
			qb.Bands[j] = float64((i+j)%7) / 7
		}
		qb.Level = 0.6
		x.Step(qa)
		y.Step(qb)
	}
	if frame(x, 40, 12) != frame(y, 40, 12) {
		t.Errorf("%s: at reactivity 0 the absolute Bands still changed the picture", name)
	}
}

// isStill lists effects allowed to hold a frame for a whole second of silence
// (none yet; a sprite that only moves to the beat would go here).
var isStill = map[string]bool{}

func TestEveryRegisteredEffectConforms(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Skip("no effects registered yet")
	}
	for _, n := range names {
		t.Run(n, func(t *testing.T) { conformance(t, n) })
	}
}

func TestRenderGroupsColourRuns(t *testing.T) {
	e := &solid{c: RGB{1, 2, 3}}
	e.Resize(10, 1)
	out := frame(e, 10, 1)
	if n := strings.Count(out, "\x1b[38"); n != 1 {
		t.Fatalf("a row of one colour should set the colour once, got %d times: %q", n, out)
	}
}

func TestPaletteRampIsOrdered(t *testing.T) {
	lum := func(c RGB) float64 { return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B) }
	if lum(testPalette.At(0)) >= lum(testPalette.At(1)) {
		t.Fatal("ramp should run dark to bright")
	}
	_ = fmt.Sprint(testPalette.At(0.5))
}

type solid struct {
	c          RGB
	cols, rows int
}

func (s *solid) Name() string                    { return "solid" }
func (s *solid) Resize(c, r int)                 { s.cols, s.rows = c, r }
func (s *solid) SetPalette(Palette)              {}
func (s *solid) Step(Audio)                      {}
func (s *solid) Cell(c, r int) (rune, RGB, bool) { return '#', s.c, c < s.cols && r < s.rows }

func TestRenderAsciiProfileEmitsNoEscapes(t *testing.T) {
	e := &solid{c: RGB{1, 2, 3}}
	e.Resize(5, 2)
	if out := Render(e, 5, 2, termenv.Ascii); strings.Contains(out, "\x1b") {
		t.Fatalf("an ASCII terminal should get no escape codes: %q", out)
	}
}
