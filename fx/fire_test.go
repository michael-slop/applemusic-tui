package fx

import "testing"

func runFire(cols, rows, frames int, kick, bass float32) *fire {
	f := newFire().(*fire)
	f.SetPalette(testPalette)
	f.Resize(cols, rows)
	for range frames {
		f.advance(kick, bass)
	}
	f.quantise()
	return f
}

// fireReach is the fraction of the height the topmost lit glyph reaches.
func fireReach(f *fire) float64 {
	for r := range f.rows {
		for c := range f.cols {
			if f.glyphIndex(c, r) > 0 {
				return float64(f.rows-r) / float64(f.rows)
			}
		}
	}
	return 0
}

func TestFireValuesStayInRange(t *testing.T) {
	f := runFire(80, 25, 0, 0, 0)
	for range 500 {
		f.advance(1, 1)
		for _, h := range f.cells {
			if h < 0 || h > 1 || h != h {
				t.Fatalf("heat %v out of range", h)
			}
		}
	}
}

func TestFireBottomIsHotterAndTopGoesCold(t *testing.T) {
	f := runFire(78, 20, 300, 0, 0)
	avg := func(r int) float32 {
		var s float32
		for c := range f.cols {
			s += f.heat(c, r)
		}
		return s / float32(f.cols)
	}
	if avg(f.rows-1) <= avg(0) {
		t.Fatalf("fire is upside down: bottom %v vs top %v", avg(f.rows-1), avg(0))
	}
	// Regression from panefx: a smooth gradient with no cold region passed
	// "bottom hotter than top" yet looked nothing like fire.
	if avg(0) >= 0.12 {
		t.Fatalf("fire never cools: top row avg heat %v", avg(0))
	}
}

// A real fire has dark space between tongues: a good fraction of the upper
// half must be fully cold.
func TestFireHasColdGaps(t *testing.T) {
	f := runFire(78, 20, 300, 0, 0)
	cold, total := 0, 0
	for r := range f.rows / 2 {
		for c := range f.cols {
			total++
			if f.glyphIndex(c, r) == 0 {
				cold++
			}
		}
	}
	if frac := float64(cold) / float64(total); frac <= 0.5 {
		t.Fatalf("upper half is not mostly empty (cold fraction %v)", frac)
	}
}

// Flames must reach a similar FRACTION of the height at any size, and the top
// two rows must be fully dark (a lit cell in row 0 is a visible line).
func TestFireScalesWithPanelHeight(t *testing.T) {
	for _, rows := range []int{25, 50, 85} {
		f := runFire(120, rows, rows*12, 0, 0)
		if r := fireReach(f); r <= 0.45 || r >= 0.98 {
			t.Errorf("flames reach %.0f%% of a %d-row panel", r*100, rows)
		}
		for r := range 2 {
			for c := range f.cols {
				if f.glyphIndex(c, r) > 0 {
					t.Fatalf("row %d of a %d-row panel is lit", r, rows)
				}
			}
		}
	}
}

// The seed row is off-screen: the bottom visible row must not be a wall.
func TestFireSeedRowIsOffScreen(t *testing.T) {
	f := runFire(40, 10, 100, 0, 0)
	for c := range f.cols {
		if f.glyphIndex(c, f.rows-1) != len(fireRamp)-1 {
			return
		}
	}
	t.Fatal("bottom visible row is a solid seed wall")
}

// The music: bass and kicks make a hotter, taller fire.
func TestFireBassFeedsTheFlames(t *testing.T) {
	sum := func(f *fire) (s float32) {
		for _, h := range f.cells[:f.cols*f.rows] {
			s += h
		}
		return s
	}
	quiet, loud := runFire(80, 30, 300, 0, 0), runFire(80, 30, 300, 1, 1)
	if sum(loud) <= sum(quiet)*1.1 {
		t.Fatalf("a loud bass line should feed the fire: %v vs %v", sum(loud), sum(quiet))
	}
}
