package fx

import (
	"math"
	"testing"
)

func newSkullAt(cols, rows int) *skullSpin {
	s := New("skullspin").(*skullSpin)
	s.SetPalette(testPalette)
	s.Resize(cols, rows)
	return s
}

func spinLit(e Effect, cols, rows int) int {
	n := 0
	for r := range rows {
		for c := range cols {
			if _, _, ok := e.Cell(c, r); ok {
				n++
			}
		}
	}
	return n
}

// Step by a Rust-style frame time at reactivity 0.
func spinStepSilent(e Effect, dt float64) {
	a := silence()
	a.DT = dt
	e.Step(a)
}

func TestSkullSpriteIsRectangularAndUsesKnownLabels(t *testing.T) {
	// An unknown byte renders as air: a hole in the face, with nothing in any
	// log to say why.
	for i, row := range skullArt {
		if len(row) != skullCols {
			t.Fatalf("row %d is %d wide, want %d", i, len(row), skullCols)
		}
		for _, b := range []byte(row) {
			if _, _, ok := spinOutlineShade(b); !ok && b != ' ' && b != '#' {
				t.Fatalf("row %d: unknown label %q", i, b)
			}
		}
	}
}

func TestSkullOutlineClosesAllTheWayAround(t *testing.T) {
	// Every neighbour of a bone cell must be bone or outline: never air, and
	// never off the edge of the grid.
	for y := range skullRows {
		for x := range skullCols {
			if skullArt[y][x] != '#' {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					ny, nx := y+dy, x+dx
					if ny < 0 || nx < 0 || ny >= skullRows || nx >= skullCols || skullArt[ny][nx] == ' ' {
						t.Fatalf("bone cell (%d,%d) has no outline beyond it", x, y)
					}
				}
			}
		}
	}
}

func TestSkullSocketsAreTheEyes(t *testing.T) {
	// The glow must land in the eyes (rows 11..14), not across the forehead:
	// the removed panefx wink painted rows 8..12 and mostly missed.
	eyes := 0
	for y := range skullRows {
		for x := range skullCols {
			if !skullSockets[y][x] {
				continue
			}
			if y < 11 || y > 16 {
				t.Fatalf("socket cell at row %d, outside the eyes and nose", y)
			}
			eyes++
		}
	}
	if eyes < 10 {
		t.Fatalf("only %d socket cells", eyes)
	}
}

func TestSkullDraws(t *testing.T) {
	s := newSkullAt(90, 34)
	spinStepSilent(s, 0.05)
	if n := spinLit(s, 90, 34); n <= 80 {
		t.Fatalf("only %d cells drawn", n)
	}
}

func TestSkullNeverVanishesWhileTurning(t *testing.T) {
	s := newSkullAt(90, 34)
	for i := range 400 {
		spinStepSilent(s, 0.05)
		if spinLit(s, 90, 34) == 0 {
			t.Fatalf("the skull vanished at step %d (t=%.2f)", i, s.t)
		}
	}
}

func TestSkullActsNeverOverlapAndAllHappen(t *testing.T) {
	seen := map[skullAct]bool{}
	for t0 := 0.0; t0 < 200; t0 += 0.02 {
		act, phase, _ := skullSequence(t0, 6, skullSeed)
		if phase < 0 || phase > 1.0001 {
			t.Fatalf("phase %v out of range at t=%v in %v", phase, t0, act)
		}
		seen[act] = true
	}
	for _, want := range []skullAct{actSpin, actSettle, actJump} {
		if !seen[want] {
			t.Errorf("%v never occurred in 200s", want)
		}
	}
}

func TestSkullAngleHoldsStillWhileActing(t *testing.T) {
	// The angle comes from time spent SPINNING, not wall time. Compare each
	// acting sample against the previous acting one: the first frame of an act
	// legitimately follows spin time up to the boundary.
	frozen, have, held := 0.0, false, false
	for t0 := 0.0; t0 < 60; t0 += 0.01 {
		act, _, spun := skullSequence(t0, 4, skullSeed)
		if act == actSpin {
			have = false
			continue
		}
		if have && math.Abs(spun-frozen) > 1e-3 {
			t.Fatalf("spin time moved during %v at t=%v: %v -> %v", act, t0, frozen, spun)
		}
		frozen, have, held = spun, true, true
	}
	if !held {
		t.Fatal("never left the Spin state")
	}
}

func TestSkullRhythmIsIrregular(t *testing.T) {
	var starts []float64
	wasSpin := true
	for t0 := 0.0; t0 < 300; t0 += 0.01 {
		act, _, _ := skullSequence(t0, 5, skullSeed)
		spinning := act == actSpin
		if wasSpin && !spinning {
			starts = append(starts, t0)
		}
		wasSpin = spinning
	}
	if len(starts) <= 8 {
		t.Fatalf("only %d acts in 300s", len(starts))
	}
	lo, hi := math.MaxFloat64, 0.0
	for i := 1; i < len(starts); i++ {
		g := starts[i] - starts[i-1]
		lo, hi = min(lo, g), max(hi, g)
	}
	if hi-lo <= 1 {
		t.Fatalf("gaps are near-identical: %v..%v", lo, hi)
	}
}

func TestSkullActsCanBeSwitchedOff(t *testing.T) {
	act, _, spun := skullSequence(12.5, 0, 1)
	if act != actSpin || math.Abs(spun-12.5) > 1e-9 {
		t.Fatalf("got %v, %v", act, spun)
	}
}

func TestSkullJumpLeavesTheGroundAndLands(t *testing.T) {
	if l, _ := skullJumpShape(0, 10); l != 0 {
		t.Fatalf("lift at 0 = %v", l)
	}
	if l, _ := skullJumpShape(1, 10); l != 0 {
		t.Fatalf("lift at 1 = %v", l)
	}
	peak, lo, hi := 0.0, 1.0, 1.0
	for p := 0.0; p <= 1; p += 0.005 {
		l, sq := skullJumpShape(p, 10)
		if l < 0 || l > 10.001 {
			t.Fatalf("lift %v at %v", l, p)
		}
		peak, lo, hi = max(peak, l), min(lo, sq), max(hi, sq)
	}
	if peak <= 8 {
		t.Fatalf("the skull barely left the ground: %v", peak)
	}
	if lo >= 0.9 || hi <= 1.1 {
		t.Fatalf("no squash-and-stretch: %v..%v", lo, hi)
	}
}

func TestSkullIsPacedByTimeNotFrameCount(t *testing.T) {
	fast, slow := newSkullAt(90, 34), newSkullAt(90, 34)
	for range 50 {
		spinStepSilent(fast, 0.02)
	}
	for range 10 {
		spinStepSilent(slow, 0.1)
	}
	if math.Abs(fast.t-slow.t) > 1e-9 || frame(fast, 90, 34) != frame(slow, 90, 34) {
		t.Fatalf("frame rate changed the picture: %v vs %v", fast.t, slow.t)
	}
}

func TestSkullStaysOnScreenAtAnyPanelShape(t *testing.T) {
	for _, sz := range [][2]int{{200, 20}, {30, 90}, {90, 34}} {
		s := newSkullAt(sz[0], sz[1])
		spinStepSilent(s, 0.05)
		if n := spinLit(s, sz[0], sz[1]); n <= 10 {
			t.Errorf("%dx%d drew only %d cells", sz[0], sz[1], n)
		}
	}
}

func TestSkullSocketsStayDarkAndBoneStaysFlat(t *testing.T) {
	// Sockets must never fill in as it turns, and the bone must be ONE glyph
	// and ONE colour at every angle. Bone is told from outline by the SPRITE
	// (via the colour, which is unique to bone here), not the glyph: a bone
	// cell that regressed to '*' must show up, not be filtered away.
	s := newSkullAt(46, 26)
	bone := map[RGB]bool{}
	for range 400 {
		spinStepSilent(s, 0.04)
		dark := false
		for r := range 26 {
			for c := range 46 {
				ch, col, ok := s.Cell(c, r)
				if !ok {
					continue
				}
				if ch != '#' {
					dark = true
				}
				if col == testPalette.Text && ch != skullBoneGlyph {
					t.Fatalf("bone drew %q", ch)
				}
				if ch == skullBoneGlyph {
					bone[col] = true
				}
			}
		}
		if !dark {
			t.Fatalf("no dark cells at t=%v", s.t)
		}
	}
	if len(bone) != 1 {
		t.Fatalf("bone should be one flat colour at every angle, got %v", bone)
	}
}

func TestSkullSpinsFasterToMusicAndEyesGlowOnKick(t *testing.T) {
	quiet, loudS := newSkullAt(60, 30), newSkullAt(60, 30)
	for i := range 30 {
		spinStepSilent(quiet, 1.0/30)
		loudS.Step(loud(i))
	}
	if loudS.extra <= 0 || quiet.extra != 0 {
		t.Fatalf("music should add spin: loud %v, quiet %v", loudS.extra, quiet.extra)
	}
	// Eyes: with a kick the socket cells take on the glow.
	a := loud(0)
	a.Kick = 1
	loudS.Step(a)
	calm := newSkullAt(60, 30)
	calm.t, calm.extra = loudS.t, loudS.extra
	calm.raster()
	if frame(calm, 60, 30) == frame(loudS, 60, 30) {
		t.Fatal("a kick changed nothing")
	}
}
