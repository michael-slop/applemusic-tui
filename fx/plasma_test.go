package fx

import (
	"math"
	"testing"
)

func builtPlasma(cols, rows int) *plasma {
	p := newPlasma().(*plasma)
	p.SetPalette(testPalette)
	p.Resize(cols, rows)
	return p
}

func ptsLit(e Effect, cols, rows int) int {
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

func TestPlasmaFieldStaysNormalised(t *testing.T) {
	// The ramp index is derived from this, so a value outside 0..1 either
	// panics on the index or flattens the whole effect to one end.
	p := builtPlasma(80, 40)
	for i := range 40 {
		p.Step(loud(i))
		for r := range p.rows {
			for c := range p.cols {
				if v := p.field(c, r); v < 0 || v > 1 {
					t.Fatalf("field out of range: %v", v)
				}
			}
		}
	}
}

func TestPlasmaIsPacedByTimeNotFrameCount(t *testing.T) {
	fast, slow := builtPlasma(20, 10), builtPlasma(20, 10)
	a := silence()
	a.DT = 0.02
	for range 50 {
		fast.Step(a)
	}
	a.DT = 0.1
	for range 10 {
		slow.Step(a)
	}
	if math.Abs(fast.t-slow.t) > 1e-9 {
		t.Fatalf("%v != %v", fast.t, slow.t)
	}
	// panefx's rate: 0.08 of a cycle per second at speed 1.
	if math.Abs(fast.t-0.08) > 1e-9 {
		t.Fatalf("one second advanced the phase by %v, want 0.08", fast.t)
	}
}

func TestPlasmaFeatureSizeDoesNotStretchWithThePanel(t *testing.T) {
	// Both axes are normalised by the SHORT one, so a feature is the same
	// number of cells across whatever the panel shape. Measured as the mean
	// distance between zero crossings along a row.
	period := func(p *plasma, row int) int {
		var xs []int
		prev := p.field(0, row) - 0.5
		for c := 1; c < p.cols; c++ {
			cur := p.field(c, row) - 0.5
			if (prev < 0) != (cur < 0) {
				xs = append(xs, c)
			}
			prev = cur
		}
		if len(xs) < 2 {
			return 0
		}
		return (xs[len(xs)-1] - xs[0]) / (len(xs) - 1)
	}
	a, b := period(builtPlasma(200, 60), 30), period(builtPlasma(60, 60), 30)
	if a == 0 || b == 0 {
		t.Fatalf("no crossings: %d, %d", a, b)
	}
	if float64(max(a, b))/float64(min(a, b)) >= 2 {
		t.Fatalf("feature size stretched with the panel: %d vs %d", a, b)
	}
}

func TestPlasmaLeavesTheDarkHalfEmpty(t *testing.T) {
	p := builtPlasma(60, 20)
	p.Step(silence())
	if lit := ptsLit(p, 60, 20); lit == 0 || lit == 60*20 {
		t.Fatalf("%d of %d cells lit; the dark cut is not working", lit, 60*20)
	}
}

func TestPlasmaKickLightsMoreOfTheField(t *testing.T) {
	calm, hit := builtPlasma(60, 20), builtPlasma(60, 20)
	for range 10 {
		a := loud(1) // odd frame: no kick
		calm.Step(a)
		a.Kick = 1
		hit.Step(a)
	}
	if h, c := ptsLit(hit, 60, 20), ptsLit(calm, 60, 20); h <= c {
		t.Fatalf("a kick should light more of the field: %d !> %d", h, c)
	}
}
