package fx

import (
	"math"
	"slices"
	"testing"
)

func builtSpin(name string, cols, rows int) *spin3d {
	s := New(name).(*spin3d)
	s.SetPalette(testPalette)
	s.Resize(cols, rows)
	return s
}

// cubeLitRows is the first and last row the cube draws on.
func cubeLitRows(e Effect, cols, rows int) (first, last int) {
	first, last = -1, -1
	for r := range rows {
		for c := range cols {
			if _, _, ok := e.Cell(c, r); ok {
				if first < 0 {
					first = r
				}
				last = r
				break
			}
		}
	}
	return
}

func TestCubeBandGroupPushesItsFaceOut(t *testing.T) {
	// Bass owns the floor, treble the lid: a bass spike drops the bottom
	// edge, a treble spike raises the top, at any point of the spin.
	const w, h = 80, 40
	for _, frames := range []int{1, 17, 40, 71} {
		rest, bass, treble := builtSpin("cube", w, h), builtSpin("cube", w, h), builtSpin("cube", w, h)
		for range frames {
			rest.Step(ptsBands(0.3, 0, 0, 0.3))
			bass.Step(ptsBands(0.3, 0, 4, 1))
			treble.Step(ptsBands(0.3, 27, 31, 1))
		}
		rt, rb := cubeLitRows(rest, w, h)
		bt, bb := cubeLitRows(bass, w, h)
		tt, tb := cubeLitRows(treble, w, h)
		if bb <= rb || bt != rt {
			t.Errorf("frame %d: bass should push the floor down only: rows %d..%d vs rest %d..%d", frames, bt, bb, rt, rb)
		}
		if tt >= rt || tb != rb {
			t.Errorf("frame %d: treble should push the lid up only: rows %d..%d vs rest %d..%d", frames, tt, tb, rt, rb)
		}
	}
}

func TestCubeFaceOffsetIsTheTorusRule(t *testing.T) {
	for _, g := range []float64{0, 0.5, 1} {
		if got, want := faceOffset(g), cubeBase*(0.55+0.85*g); math.Abs(got-want) > 1e-12 {
			t.Fatalf("faceOffset(%v) = %v, want %v", g, got, want)
		}
	}
}

func TestCubeSilenceIsASmallRestingCube(t *testing.T) {
	quiet, normal := builtSpin("cube", 80, 40), builtSpin("cube", 80, 40)
	quiet.Step(ptsBands(0, 0, 0, 0))
	normal.Step(ptsBands(0.5, 0, 0, 0.5))
	q, n := ptsLit(quiet, 80, 40), ptsLit(normal, 80, 40)
	if q <= 20 || q*2 >= n {
		t.Fatalf("silence drew %d cells vs %d at a normal level: want a small but visible cube", q, n)
	}
}

func TestCubeZBufferHidesTheFarSurface(t *testing.T) {
	s := builtSpin("cube", 80, 40)
	s.Step(silence())
	if n := ptsLit(s, 80, 40); n >= 80*40/2 {
		t.Fatalf("%d/%d cells lit: the far side is showing through", n, 80*40)
	}
	if !slices.ContainsFunc(s.zbuf, func(z float64) bool { return z > 0 }) {
		t.Fatal("nothing ever claimed a depth slot")
	}
}

func TestCubeRotates(t *testing.T) {
	s := builtSpin("cube", 80, 40)
	s.Step(silence())
	first := slices.Clone(s.lum)
	for range 12 {
		s.Step(silence())
	}
	if slices.Equal(first, s.lum) {
		t.Fatal("the cube never turned")
	}
}

func TestCubeIsPacedByTimeNotFrameCount(t *testing.T) {
	fast, slow := builtSpin("cube", 20, 10), builtSpin("cube", 20, 10)
	a := silence()
	a.DT = 0.02
	for range 50 {
		fast.Step(a)
	}
	a.DT = 0.1
	for range 10 {
		slow.Step(a)
	}
	if math.Abs(fast.a-slow.a) > 1e-9 || math.Abs(fast.a-1) > 1e-9 {
		t.Fatalf("a = %v / %v, want 1 rad after 1 s", fast.a, slow.a)
	}
}

func TestCubeStaysOnScreenAtAnyPanelShape(t *testing.T) {
	for _, sz := range [][2]int{{200, 20}, {20, 200}, {80, 40}} {
		s := builtSpin("cube", sz[0], sz[1])
		s.Step(silence())
		if n := ptsLit(s, sz[0], sz[1]); n <= 20 {
			t.Errorf("%dx%d drew only %d cells", sz[0], sz[1], n)
		}
	}
}

func TestCubeKickAddsRotation(t *testing.T) {
	calm, hit := builtSpin("cube", 40, 20), builtSpin("cube", 40, 20)
	a := silence()
	a.Playing = true
	calm.Step(a)
	a.Kick = 1
	hit.Step(a)
	if hit.a <= calm.a {
		t.Fatalf("a kick should spin it faster: %v !> %v", hit.a, calm.a)
	}
}

func TestCubeStepDoesNotAllocate(t *testing.T) {
	s := builtSpin("cube", 137, 41)
	a := loud(3)
	if n := testing.AllocsPerRun(20, func() { s.Step(a) }); n != 0 {
		t.Fatalf("Step allocated %v times", n)
	}
}
