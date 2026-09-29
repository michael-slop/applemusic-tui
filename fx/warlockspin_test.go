package fx

import (
	"math"
	"testing"
)

func newWarlockAt(cols, rows int) *warlockSpin {
	w := New("warlockspin").(*warlockSpin)
	w.SetPalette(testPalette)
	w.Resize(cols, rows)
	return w
}

func warlockSolid(b byte) bool { return b == '#' || b == '=' || b == '%' || b == '|' }

func TestWarlockSpriteIsRectangularAndUsesKnownLabels(t *testing.T) {
	for i, row := range warlockArt {
		if len(row) != warlockCols {
			t.Fatalf("row %d is the wrong width", i)
		}
		for _, b := range []byte(row) {
			if _, _, ok := spinOutlineShade(b); !ok && b != ' ' && !warlockSolid(b) {
				t.Fatalf("row %d has an unknown label %q", i, b)
			}
		}
	}
}

func TestWarlockOutlineClosesAllTheWayAround(t *testing.T) {
	for y := range warlockRows {
		for x := range warlockCols {
			if !warlockSolid(warlockArt[y][x]) {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					ny, nx := y+dy, x+dx
					if ny < 0 || nx < 0 || ny >= warlockRows || nx >= warlockCols || warlockArt[ny][nx] == ' ' {
						t.Fatalf("figure cell (%d,%d) has no outline beyond it", x, y)
					}
				}
			}
		}
	}
}

func TestWarlockRocksRatherThanSpinningThroughItsOwnBack(t *testing.T) {
	// THE point of this effect. The warlock faces right; turning past edge-on
	// would show a back the sprite does not have.
	for i := range 2000 {
		tt := float64(i) * 0.01
		if c := warlockRock(tt, 0.25, 1); c < -0.001 || c > 1.001 {
			t.Fatalf("cos %v at t=%v turned past edge-on", c, tt)
		}
	}
	// And with the music at full tilt, over the effect itself.
	w := newWarlockAt(60, 30)
	for i := range 600 {
		w.Step(loud(i))
		if c := warlockRock(w.rt, warlockRate, w.sway); c < -0.001 || c > 1.001 {
			t.Fatalf("to music, cos %v at step %d turned past edge-on", c, i)
		}
	}
}

func TestWarlockSwayZeroHoldsStillFacingTheViewer(t *testing.T) {
	for i := range 200 {
		if c := warlockRock(float64(i)*0.05, 0.25, 0); math.Abs(c-1) > 1e-9 {
			t.Fatalf("sway=0 should not move, got %v", c)
		}
	}
}

func TestWarlockEveryPartStaysOneFlatColourWhileItTurns(t *testing.T) {
	w := newWarlockAt(60, 34)
	seen := map[rune]map[RGB]bool{}
	for range 400 {
		spinStepSilent(w, 0.04)
		for r := range 34 {
			for c := range 60 {
				if g, col, ok := w.Cell(c, r); ok {
					if seen[g] == nil {
						seen[g] = map[RGB]bool{}
					}
					seen[g][col] = true
				}
			}
		}
	}
	for g, cols := range seen {
		if _, _, outline := spinOutlineShade(byte(g)); outline {
			continue // the halo is graded on purpose
		}
		if len(cols) != 1 {
			t.Errorf("glyph %q drew %d different colours", g, len(cols))
		}
	}
	for _, g := range []rune{'#', '=', '|', '%'} {
		if seen[g] == nil {
			t.Errorf("part %q never drew", g)
		}
	}
}

func TestWarlockStaysInsideThePanelAtAnyShape(t *testing.T) {
	for _, sz := range [][2]int{{20, 10}, {200, 60}, {40, 120}, {8, 8}} {
		w := newWarlockAt(sz[0], sz[1])
		spinStepSilent(w, 0.04)
		if spinLit(w, sz[0], sz[1]) == 0 {
			t.Errorf("%dx%d drew nothing", sz[0], sz[1])
		}
	}
}

func TestWarlockGrooveAndGlint(t *testing.T) {
	quiet, groovy := newWarlockAt(60, 30), newWarlockAt(60, 30)
	for i := range 60 {
		spinStepSilent(quiet, 1.0/30)
		groovy.Step(loud(i))
	}
	if groovy.rt <= quiet.rt+0.1 {
		t.Fatalf("the bass should speed the rock: %v vs %v", groovy.rt, quiet.rt)
	}
	if groovy.sway <= warlockSway || groovy.sway > 1 {
		t.Fatalf("the bass should widen the sway within edge-on: %v", groovy.sway)
	}
	// A kick lights the lenses.
	a := silence()
	a.Kick = 0.6
	quiet.Step(a)
	glinted := false
	for r := range 30 {
		for c := range 60 {
			if ch, col, ok := quiet.Cell(c, r); ok && ch == '=' && col != testPalette.Accent {
				glinted = true
			}
		}
	}
	if !glinted {
		t.Fatal("no glint on a kick")
	}
}
