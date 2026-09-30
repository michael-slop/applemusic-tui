package fx

import "testing"

func builtPlasma(cols, rows int) *plasma {
	p := newPlasma().(*plasma)
	p.SetPalette(testPalette)
	p.Resize(cols, rows)
	return p
}

// ptsBands is a playing frame with every band at base and bands lo..hi at v.
func ptsBands(base float64, lo, hi int, v float64) Audio {
	a := silence()
	a.Playing = true
	for i := range a.React {
		a.React[i] = base
		if i >= lo && i <= hi {
			a.React[i] = v
		}
	}
	return a
}

// ptsWeightWhere sums the drawn brightness of the cells for which in is true.
func ptsWeightWhere(e Effect, cols, rows int, in func(c, r int) bool) float64 {
	var s float64
	for r := range rows {
		for c := range cols {
			if !in(c, r) {
				continue
			}
			if _, col, ok := e.Cell(c, r); ok {
				s += (0.2126*float64(col.R) + 0.7152*float64(col.G) + 0.0722*float64(col.B)) / 255
			}
		}
	}
	return s
}

func TestPlasmaBassLightsTheCentreTrebleTheRim(t *testing.T) {
	const w, h = 60, 20
	p := builtPlasma(w, h)
	centre := func(c, r int) bool { return p.rad[r*w+c] < 0.2 }
	rim := func(c, r int) bool { return p.rad[r*w+c] > 0.8 && p.rad[r*w+c] < 1 }

	p.Step(ptsBands(0, 0, 5, 1))
	bc, br := ptsWeightWhere(p, w, h, centre), ptsWeightWhere(p, w, h, rim)
	if bc == 0 || br > bc*0.1 {
		t.Fatalf("a bass spike should light the centre, not the rim: centre %.1f rim %.1f", bc, br)
	}
	p.Step(ptsBands(0, 26, 31, 1))
	tc, tr := ptsWeightWhere(p, w, h, centre), ptsWeightWhere(p, w, h, rim)
	if tr == 0 || tr <= tc {
		t.Fatalf("a treble spike should light the rim more than the centre: centre %.1f rim %.1f", tc, tr)
	}
	// The centre keeps only the faint resting disc: its sparsest glyph.
	for r := range h {
		for c := range w {
			if ch, _, ok := p.Cell(c, r); ok && centre(c, r) && ch != plasmaRamp[1] {
				t.Fatalf("a treble spike lit the centre with %q", ch)
			}
		}
	}
}

func TestPlasmaRingsAreRound(t *testing.T) {
	// Cells are ~1:2, so a ring's horizontal radius in cells is twice its
	// vertical one.
	p := builtPlasma(80, 40)
	if got := p.rad[20*80+40+20]; got < 0.45 || got > 0.55 {
		t.Fatalf("20 cells right of centre is at radius %.2f, want ~0.5", got)
	}
	if got := p.rad[(20+10)*80+40]; got < 0.45 || got > 0.55 {
		t.Fatalf("10 rows below centre is at radius %.2f, want ~0.5", got)
	}
}

func TestPlasmaSilenceIsAFaintRestingDisc(t *testing.T) {
	p := builtPlasma(60, 20)
	a := silence()
	for i := range a.React {
		a.React[i] = 0
	}
	p.Step(a)
	lit := ptsLit(p, 60, 20)
	if lit == 0 || lit > 60*20/4 {
		t.Fatalf("silence lit %d of %d cells, want a small disc", lit, 60*20)
	}
	for r := range 20 {
		for c := range 60 {
			if ch, _, ok := p.Cell(c, r); ok && p.rad[r*60+c] >= plasmaRestR {
				t.Fatalf("silence drew %q outside the resting disc", ch)
			}
		}
	}
}

func TestPlasmaKickBreathesOutward(t *testing.T) {
	calm, hit := builtPlasma(60, 20), builtPlasma(60, 20)
	for range 10 {
		a := ptsBands(0, 0, 10, 1)
		calm.Step(a)
		a.Kick = 1
		hit.Step(a)
	}
	if h, c := ptsLit(hit, 60, 20), ptsLit(calm, 60, 20); h <= c {
		t.Fatalf("a kick should push the lit core outward: %d !> %d", h, c)
	}
}

func TestPlasmaStepDoesNotAllocate(t *testing.T) {
	p := builtPlasma(80, 24)
	a := loud(3)
	if n := testing.AllocsPerRun(20, func() { p.Step(a) }); n != 0 {
		t.Fatalf("Step allocated %v times", n)
	}
}
