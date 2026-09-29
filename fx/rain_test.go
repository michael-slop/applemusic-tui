package fx

import (
	"strings"
	"testing"
)

func newRainAt(cols, rows int) *rain {
	r := newRain().(*rain)
	r.SetPalette(testPalette)
	r.Resize(cols, rows)
	return r
}

func TestRainCharsetIsTheSitesExactSet(t *testing.T) {
	if !strings.HasPrefix(rainChars, "01ABCDEFGHIJKLMNOPQRSTUVWXYZ") || !strings.Contains(rainChars, "░▒▓") {
		t.Fatal("not the site's set")
	}
	for _, c := range rainChars {
		if c >= 0x30A0 && c <= 0x30FF {
			t.Fatal("ascii rain must not carry katakana")
		}
	}
}

// If every column started at row 0 the rain would fall as one sheet.
func TestRainColumnsStartStaggered(t *testing.T) {
	r := newRainAt(60, 30)
	seen := map[int]bool{}
	for _, d := range r.drops {
		if d > 0 {
			t.Fatalf("drop starts below the top: %d", d)
		}
		seen[d] = true
	}
	if len(seen) <= 5 {
		t.Fatalf("drops not staggered: %v", seen)
	}
}

func TestRainHeadsFallAndRespawn(t *testing.T) {
	r := newRainAt(20, 15)
	for range 400 {
		r.advance()
	}
	for _, d := range r.drops {
		if d < -25 || d > r.rows+35 {
			t.Fatalf("runaway drop %d", d)
		}
	}
}

// A trail must span several cells, or it reads as dots rather than rain.
func TestRainTrailsFadeRatherThanVanish(t *testing.T) {
	r := newRainAt(1, 40)
	for range 60 {
		r.advance()
	}
	lit := 0
	for row := range 40 {
		if _, _, ok := r.Cell(0, row); ok {
			lit++
		}
	}
	if lit < 3 {
		t.Fatalf("trail too short (%d cells)", lit)
	}
}

// The 55 ms tick holds at amtui's 30 fps, and treble quickens it.
func TestRainTicksAt55msAndTrebleQuickens(t *testing.T) {
	steps := func(treble float64) float64 {
		r := newRainAt(4, 4)
		for range 90 { // three seconds
			a := silence()
			a.Playing, a.Treble = true, treble
			r.Step(a)
		}
		return float64(r.steps) / 3
	}
	if s := steps(0.5); s < 17 || s > 19 {
		t.Fatalf("at rest the rain should step ~18 times a second, got %v", s)
	}
	if s := steps(1); s < 24 {
		t.Fatalf("full treble should quicken the rain, got %v steps/s", s)
	}
}
