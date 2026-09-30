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

func builtPlasmaSquare(cols, rows int) *plasma {
	p := newPlasmaSquare().(*plasma)
	p.SetPalette(testPalette)
	p.Resize(cols, rows)
	return p
}

func TestPlasmaSquareQuadrantsMirror(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {61, 21}, {7, 3}} {
		w, h := size[0], size[1]
		p := builtPlasmaSquare(w, h)
		a := ptsBands(0.3, 4, 12, 0.9)
		a.React[27] = 1
		a.Kick = 0.6
		for range 40 { // turn the texture well away from its start
			p.Step(a)
		}
		for r := range h {
			for c := range w {
				ch, col, ok := p.Cell(c, r)
				for _, m := range [][2]int{{w - 1 - c, r}, {c, h - 1 - r}, {w - 1 - c, h - 1 - r}} {
					mch, mcol, mok := p.Cell(m[0], m[1])
					if mch != ch || mcol != col || mok != ok {
						t.Fatalf("%dx%d: cell (%d,%d) differs from its mirror (%d,%d)", w, h, c, r, m[0], m[1])
					}
				}
			}
		}
	}
}

func TestPlasmaSquareFillsThePanelTheCircleDoesNot(t *testing.T) {
	const w, h = 60, 20
	loud := ptsBands(1, 0, 0, 1)
	sq, disc := builtPlasmaSquare(w, h), builtPlasma(w, h)
	sq.Step(loud)
	disc.Step(loud)
	for r := range h {
		for c := range w {
			if _, _, ok := sq.Cell(c, r); !ok {
				t.Fatalf("plasma-square left (%d,%d) blank with every band loud", c, r)
			}
		}
	}
	for _, c := range [][2]int{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}} {
		if _, _, ok := disc.Cell(c[0], c[1]); ok {
			t.Fatalf("circular plasma drew corner %v; it should stay round", c)
		}
	}
}

func TestPlasmaSquareBassCentreTrebleCorners(t *testing.T) {
	const w, h = 60, 20
	p := builtPlasmaSquare(w, h)
	centre := func(c, r int) bool { return p.rad[r*w+c] < 0.2 }
	corners := func(c, r int) bool { return p.rad[r*w+c] > 0.9 }

	// Per-cell means: the corner region is far smaller than the centre, so
	// sums would compare areas, not brightness.
	mean := func(in func(c, r int) bool) float64 {
		n := 0
		for r := range h {
			for c := range w {
				if in(c, r) {
					n++
				}
			}
		}
		return ptsWeightWhere(p, w, h, in) / float64(n)
	}
	p.Step(ptsBands(0, 0, 5, 1))
	bc, bk := mean(centre), mean(corners)
	if bc == 0 || bk > bc*0.1 {
		t.Fatalf("a bass spike should light the centre, not the corners: centre %.3f corners %.3f", bc, bk)
	}
	p.Step(ptsBands(0, 26, 31, 1))
	tc, tk := mean(centre), mean(corners)
	if tk == 0 || tk <= tc*1.5 {
		t.Fatalf("a treble spike should light the corners well above the centre: centre %.3f corners %.3f", tc, tk)
	}
}

func TestPlasmaSquareRadiusRunsCentreToCorner(t *testing.T) {
	if r := plasmaSquareRadius(0, 0); r != 0 {
		t.Fatalf("centre radius = %v, want 0", r)
	}
	if r := plasmaSquareRadius(1, 1); r < 0.999999 || r > 1.000001 {
		t.Fatalf("corner radius = %v, want 1", r)
	}
	if a, b := plasmaSquareRadius(1, 0), plasmaSquareRadius(0, 1); a != b || a > 0.85 || a < 0.83 {
		t.Fatalf("edge-middle radii = %v, %v, want equal ~0.84", a, b)
	}
}

func TestPlasmaHatFlashesTheRimNotTheCentre(t *testing.T) {
	const w, h = 60, 20
	quiet, hat := builtPlasma(w, h), builtPlasma(w, h)
	// Every band lit, mid-level: the flash has headroom below the clamp at 1.
	a := ptsBands(0.5, 0, 31, 0.5)
	for range 10 {
		quiet.Step(a)
		withHat := a
		withHat.Hat = 1
		hat.Step(withHat)
	}
	centre := func(c, r int) bool { return quiet.rad[r*w+c] < 0.4 }
	rim := func(c, r int) bool { return quiet.rad[r*w+c] > 0.8 && quiet.rad[r*w+c] < 1 }
	if qc, hc := ptsWeightWhere(quiet, w, h, centre), ptsWeightWhere(hat, w, h, centre); qc != hc {
		t.Fatalf("the hi-hat changed the centre: %.2f -> %.2f", qc, hc)
	}
	if qr, hr := ptsWeightWhere(quiet, w, h, rim), ptsWeightWhere(hat, w, h, rim); hr <= qr*1.10 {
		t.Fatalf("the hi-hat did not brighten the rim: %.2f -> %.2f", qr, hr)
	}
}
