package fx

import (
	"fmt"
	"math"
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

	// --- the torus blueprint (see the package doc) --------------------------
	const w, h = 60, 20
	run := func(mk func(i int) Audio, frames int) Effect {
		e := New(name)
		e.SetPalette(testPalette)
		e.Resize(w, h)
		for i := range frames {
			e.Step(mk(i))
		}
		return e
	}
	flat := func(v float64) func(int) Audio {
		return func(int) Audio {
			a := silence()
			a.Playing = true
			for i := range a.React {
				a.React[i] = v
			}
			a.Bass, a.Mid, a.Treble = v, v, v
			return a
		}
	}
	// 1. Shape is the spectrum: a loud spectrum draws a clearly bigger or
	// brighter picture than a quiet one.
	quiet, loudE := run(flat(0.1), 60), run(flat(0.9), 60)
	lq, ll := litWeight(quiet, w, h), litWeight(loudE, w, h)
	if ll < lq*1.3+5 {
		t.Errorf("%s: a loud spectrum should draw a bigger/brighter shape than a quiet one (weight %.0f vs %.0f)", name, ll, lq)
	}
	// 2. Every band owns a region: a bass spike and a treble spike must draw
	// clearly different pictures.
	spike := func(lo, hi int) func(int) Audio {
		return func(int) Audio {
			a := flat(0.3)(0)
			for i := lo; i <= hi; i++ {
				a.React[i] = 1
			}
			return a
		}
	}
	bass, treble := run(spike(0, 5), 45), run(spike(26, 31), 45)
	if d := cellsDiffer(bass, treble, w, h); d < w*h/20 {
		t.Errorf("%s: bass and treble spikes should light different regions (only %d of %d cells differ)", name, d, w*h)
	}
	// 3. Silence is a resting silhouette: after the music stops, at most a
	// rigid rotation moves it -- no churn. DT is amtui's silent pace (motion.go
	// runs time at 4% when nothing plays).
	hush := func(int) Audio { a := flat(0)(0); a.DT *= 0.04; return a }
	still := run(hush, 90)
	before := snapshot(still, w, h)
	for range 30 {
		still.Step(hush(0))
	}
	if d := diffSnap(before, snapshot(still, w, h)); d > w*h/10 {
		t.Errorf("%s: in silence %d of %d cells changed in a second -- motion not tied to the music", name, d, w*h)
	}
	// 3b. A steady spectrum draws a steady shape. Rotation or a scroll of a
	// stable shape is fine; the size/brightness of the picture must not
	// churn on its own (random births, flicker, noise that is not a band).
	steady := run(flat(0.7), 60)
	var ws []float64
	for range 30 {
		steady.Step(flat(0.7)(0))
		ws = append(ws, litWeight(steady, w, h))
	}
	if mean, sd := meanSD(ws); mean > 0 && sd/mean > 0.10 {
		t.Errorf("%s: a steady spectrum should hold a steady shape; its weight varies %.0f%% over a second", name, 100*sd/mean)
	}
	// 3c. ...and, unless the effect declares its free motion (a rigid spin or
	// scroll like the torus, or particles its bands launch), the cells
	// themselves hold still: no flicker, no churn.
	if _, declared := steady.(FreeMover); !declared {
		held := snapshot(steady, w, h)
		for range 30 {
			steady.Step(flat(0.7)(0))
		}
		if d := diffSnap(held, snapshot(steady, w, h)); d > w*h/10 {
			t.Errorf("%s: a steady spectrum changed %d of %d cells in a second -- motion not tied to the music (declare FreeMotion if it is a rigid spin/scroll)", name, d, w*h)
		}
	}

	// 4. Effects read the reactive, slider-scaled signal (React/Bass/Mid/
	// Treble/Kick), never the raw absolute Bands/Level: two runs that differ
	// only in Bands must draw the same frames (this is also a determinism
	// check -- any RNG must be seeded with a constant).
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

// litWeight sums the brightness of every drawn cell: size and glow together.
func litWeight(e Effect, w, h int) float64 {
	var s float64
	for r := range h {
		for c := range w {
			if ch, col, ok := e.Cell(c, r); ok && ch != ' ' {
				s += (0.2126*float64(col.R) + 0.7152*float64(col.G) + 0.0722*float64(col.B)) / 255
			}
		}
	}
	return s
}

type cellSnap struct {
	ch rune
	c  RGB
	ok bool
}

func snapshot(e Effect, w, h int) []cellSnap {
	out := make([]cellSnap, 0, w*h)
	for r := range h {
		for c := range w {
			ch, col, ok := e.Cell(c, r)
			if !ok || ch == ' ' {
				ch, col, ok = 0, RGB{}, false
			}
			out = append(out, cellSnap{ch, col, ok})
		}
	}
	return out
}

func diffSnap(a, b []cellSnap) int {
	n := 0
	for i := range a {
		if a[i] != b[i] {
			n++
		}
	}
	return n
}

func meanSD(xs []float64) (float64, float64) {
	var m, v float64
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return m, math.Sqrt(v / float64(len(xs)))
}

func cellsDiffer(x, y Effect, w, h int) int { return diffSnap(snapshot(x, w, h), snapshot(y, w, h)) }

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
