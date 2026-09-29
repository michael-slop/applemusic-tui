package fx

import "testing"

func newFlamesAt(cols, rows int) *flames {
	f := newFlames().(*flames)
	f.SetPalette(testPalette)
	f.Resize(cols, rows)
	return f
}

// topLit is how many rows up from the bottom the highest lit cell sits.
func flamesReach(f *flames) int {
	for r := 0; r < f.height; r++ {
		for c := 0; c < f.width; c++ {
			if _, _, ok := f.Cell(c, r); ok {
				return f.height - r
			}
		}
	}
	return 0
}

func TestFlamesRampMatchesTheGist(t *testing.T) {
	if flamesChars[0] != ' ' || flamesChars[9] != '$' {
		t.Fatal("ramp is not the gist's")
	}
}

// DELIBERATE: with the gist's seed value the fire reaches ~20 rows regardless
// of panel height, so on a tall panel it is a band along the bottom rather
// than a full-height effect. That is the chosen look, not a shortfall.
func TestFlamesStaysABottomBandOnATallPanel(t *testing.T) {
	f := newFlamesAt(143, 84)
	for range 600 {
		f.advance(0, 0)
	}
	reach := flamesReach(f)
	if reach <= 5 {
		t.Fatalf("fire barely renders at all (%d rows)", reach)
	}
	if reach >= 50 {
		t.Fatalf("fire now fills %d/84 rows: the bottom-band look is gone", reach)
	}
}

func TestFlamesValuesStayBounded(t *testing.T) {
	f := newFlamesAt(143, 84)
	for i := range 600 {
		f.advance(float64(i%3)/2, 1)
		for _, v := range f.b {
			if v < 0 || v > flamesSeedValue+flamesKickLift {
				t.Fatalf("value %d out of range", v)
			}
		}
	}
}

// The breath has to actually go up AND down, by the full amount, and frame 0
// must be the configured height.
func TestFlamesBreathRisesAndFalls(t *testing.T) {
	f := newFlamesAt(8, 8)
	f.osc, f.oscSecs = 40, 10 // 100 frames at 10 fps
	if f.seedNow() != flamesSeedValue {
		t.Fatal("frame 0 must be the configured height")
	}
	lo, hi := int32(999), int32(-1)
	for range 100 {
		v := f.seedNow()
		lo, hi = min(lo, v), max(hi, v)
		f.tick++
	}
	if hi < 104 || hi > 106 || lo < 24 || lo > 26 {
		t.Fatalf("breath should span ~25..105, got %d..%d", lo, hi)
	}
	f.osc, f.oscSecs = 120, 4
	for range 200 {
		if v := f.seedNow(); v < 1 || v > 255 {
			t.Fatalf("seed left its range: %d", v)
		}
		f.tick++
	}
}

// The music: kicks throw the fire higher than the gist alone reaches.
func TestFlamesKickRaisesTheFire(t *testing.T) {
	quiet, loud := newFlamesAt(80, 60), newFlamesAt(80, 60)
	for range 300 {
		quiet.advance(0, 0)
		loud.advance(1, 1)
	}
	if q, l := flamesReach(quiet), flamesReach(loud); l <= q {
		t.Fatalf("kicks should raise the flames: quiet %d rows, loud %d rows", q, l)
	}
}

// Stepping at 30 fps must advance the gist at its own 10 fps.
func TestFlamesRunsAtItsOwnRate(t *testing.T) {
	f := newFlamesAt(40, 20)
	for range 30 {
		f.Step(silence())
	}
	if f.tick < 9 || f.tick > 10 {
		t.Fatalf("one second at 30 fps ran %d gist frames, want 10", f.tick)
	}
}
