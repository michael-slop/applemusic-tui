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

func TestSpin3dEveryShapeDrawsSomething(t *testing.T) {
	// A shape whose parameterisation is wrong renders as an empty screen,
	// which looks exactly like a broken effect.
	for _, name := range []string{"cube"} {
		s := builtSpin(name, 80, 40)
		s.Step(silence())
		if n := ptsLit(s, 80, 40); n <= 50 {
			t.Errorf("%s drew only %d cells", name, n)
		}
	}
}

func TestSpin3dZBufferHidesTheFarSurface(t *testing.T) {
	// Without the depth test the back draws over the front and the solid
	// reads as a tangle covering far more of the screen.
	s := builtSpin("cube", 80, 40)
	s.Step(silence())
	if n := ptsLit(s, 80, 40); n >= 80*40/2 {
		t.Fatalf("%d/%d cells lit: the far side is showing through", n, 80*40)
	}
	if !slices.ContainsFunc(s.zbuf, func(z float64) bool { return z > 0 }) {
		t.Fatal("nothing ever claimed a depth slot")
	}
}

func TestSpin3dRotates(t *testing.T) {
	s := builtSpin("cube", 80, 40)
	s.Step(silence())
	first := slices.Clone(s.lum)
	for range 12 {
		s.Step(silence())
	}
	if slices.Equal(first, s.lum) {
		t.Fatal("the solid never turned")
	}
}

func TestSpin3dIsPacedByTimeNotFrameCount(t *testing.T) {
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

func TestSpin3dStaysOnScreenAtAnyPanelShape(t *testing.T) {
	// Fitting to the wrong axis puts a wide panel's solid off the top, or a
	// tall one's off the sides.
	for _, sz := range [][2]int{{200, 20}, {20, 200}, {80, 40}} {
		s := builtSpin("cube", sz[0], sz[1])
		s.Step(silence())
		if n := ptsLit(s, sz[0], sz[1]); n <= 20 {
			t.Errorf("%dx%d drew only %d cells", sz[0], sz[1], n)
		}
	}
}

func TestSpin3dKickAddsRotation(t *testing.T) {
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
