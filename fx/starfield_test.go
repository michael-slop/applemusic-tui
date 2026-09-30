package fx

import "testing"

func builtStarfield(cols, rows int) *starfield {
	s := newStarfield().(*starfield)
	s.SetPalette(testPalette)
	s.Resize(cols, rows)
	return s
}

func TestStarfieldSpikedBandLaunchesOnlyInItsDirection(t *testing.T) {
	const w, h = 80, 40
	cx, cy := float64(w)/2, float64(h)/2
	for _, tc := range []struct {
		band int
		down bool
	}{{0, true}, {31, false}} {
		s := builtStarfield(w, h)
		for range 30 {
			s.Step(ptsBands(0, tc.band, tc.band, 1))
		}
		if len(s.stars) == 0 {
			t.Fatalf("band %d launched nothing", tc.band)
		}
		ring := s.ringRadius()
		for r := range h {
			for c := range w {
				if _, _, ok := s.Cell(c, r); !ok {
					continue
				}
				dx, dy := float64(c)+0.5-cx, (float64(r)+0.5-cy)*2
				if dx*dx+dy*dy <= (ring+2)*(ring+2) {
					continue // the central ring
				}
				// Band 0 owns straight down, band 31 straight up: a narrow
				// fan either way.
				if (dy > 0) != tc.down || starAbs(dx) > starAbs(dy)*0.25+2 {
					t.Fatalf("band %d lit a star at (%d,%d), outside its lane", tc.band, c, r)
				}
			}
		}
	}
}

func starAbs(x float64) float64 { return max(x, -x) }

func TestStarfieldSilenceEmptiesTheField(t *testing.T) {
	s := builtStarfield(80, 40)
	for range 60 {
		s.Step(ptsBands(0.8, 0, 0, 0.8))
	}
	busy := ptsLit(s, 80, 40)
	for range 90 {
		s.Step(ptsBands(0, 0, 0, 0))
	}
	if len(s.stars) != 0 {
		t.Fatalf("%d stars survive three seconds of silence", len(s.stars))
	}
	rest := ptsLit(s, 80, 40)
	if rest == 0 || rest >= busy/4 {
		t.Fatalf("silence should leave only the faint central ring: %d lit (busy %d)", rest, busy)
	}
}

func TestStarfieldLouderBandLaunchesMore(t *testing.T) {
	quiet, loudS := builtStarfield(80, 40), builtStarfield(80, 40)
	for range 60 {
		quiet.Step(ptsBands(0.3, 0, 0, 0.3))
		loudS.Step(ptsBands(0.9, 0, 0, 0.9))
	}
	if len(loudS.stars) <= len(quiet.stars)*2 {
		t.Fatalf("loud %d stars vs quiet %d", len(loudS.stars), len(quiet.stars))
	}
}

func TestStarfieldKickWarps(t *testing.T) {
	calm, hit := builtStarfield(80, 40), builtStarfield(80, 40)
	for range 20 {
		calm.Step(ptsBands(0.7, 0, 0, 0.7))
		hit.Step(ptsBands(0.7, 0, 0, 0.7))
	}
	r0 := calm.stars[0].r
	a := ptsBands(0.7, 0, 0, 0.7)
	calm.Step(a)
	a.Kick = 1
	hit.Step(a)
	if hit.stars[0].r-r0 <= calm.stars[0].r-r0 {
		t.Fatal("a kick should speed the stars outward")
	}
}

func TestStarfieldResizeRebuildsTheGrid(t *testing.T) {
	s := builtStarfield(80, 40)
	s.Step(loud(0))
	s.Resize(30, 15)
	s.Step(loud(1))
	if len(s.grid) != 30*15 {
		t.Fatalf("grid %d, want %d", len(s.grid), 30*15)
	}
}

func TestStarfieldStepDoesNotAllocate(t *testing.T) {
	s := builtStarfield(137, 41)
	for i := range 60 {
		s.Step(loud(i))
	}
	i := 0
	if n := testing.AllocsPerRun(30, func() { s.Step(loud(i)); i++ }); n != 0 {
		t.Fatalf("Step allocated %v times", n)
	}
}
