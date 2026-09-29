package fx

import (
	"math"
	"strings"
	"testing"
)

// wtFNV is FNV-1a 32, the checksum gen_wizardtorch_art.py prints from the
// Rust source text.
func wtFNV(b []byte) uint32 {
	h := uint32(0x811C9DC5)
	for _, c := range b {
		h ^= uint32(c)
		h *= 0x01000193
	}
	return h
}

func wtPieceByName(t *testing.T, name string) wtPiece {
	t.Helper()
	for _, p := range wtPieces() {
		if p.name == name {
			return p
		}
	}
	t.Fatalf("no piece %q", name)
	return wtPiece{}
}

// wtRest is a reactivity-0 frame at 30 fps.
func wtRest() Audio {
	a := Audio{DT: 1.0 / 30, Bass: 0.5, Mid: 0.5, Treble: 0.5}
	for i := range a.React {
		a.React[i] = 0.5
	}
	return a
}

// wtLandscape is a landscape grid, the case the fit logic exists for.
func wtLandscape() *WizardTorch {
	w := newWizardTorch(wtPiecesByIndex(0))
	w.SetPalette(testPalette)
	w.Resize(160, 60)
	return w
}

func wtPiecesByIndex(i int) wtPiece { return wtPieces()[i] }

func wtLit(e Effect, cols, rows int) int {
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

func TestWizardTorchArtMatchesTheRustSource(t *testing.T) {
	// Checksums of the ART arrays as they appear in panefx's *_art.rs,
	// computed from the Rust text by the generator: the Go data must be a
	// byte-for-byte copy.
	want := map[string]struct {
		cols, rows int
		sum        uint32
	}{
		"wizardtorch": {80, 128, 2546689485},
		"tgevil":      {80, 208, 1090072136},
		"wzfire":      {80, 160, 295858083},
		"raalien":     {80, 508, 3467714990},
	}
	for _, p := range wtPieces() {
		w := want[p.name]
		if p.cols != w.cols || p.rows != w.rows || len(p.art) != p.rows {
			t.Fatalf("%s: %dx%d (%d rows), want %dx%d", p.name, p.cols, p.rows, len(p.art), w.cols, w.rows)
		}
		if got := wtFNV([]byte(strings.Join(p.art, "\n"))); got != w.sum {
			t.Errorf("%s: checksum %d, want %d", p.name, got, w.sum)
		}
	}
}

func TestEveryShippedPieceIsRectangularAndDrawsSomething(t *testing.T) {
	// A ragged row or an unknown glyph renders as a hole in the drawing with
	// nothing in any log to say why.
	for _, p := range wtPieces() {
		for i, row := range p.art {
			if len(row) != p.cols {
				t.Fatalf("%s row %d is %d wide, want %d", p.name, i, len(row), p.cols)
			}
			for j := range len(row) {
				if b := row[j]; b != ' ' && wtShade[b] == 0 {
					t.Fatalf("%s row %d: unknown glyph %q", p.name, i, b)
				}
			}
		}
		w := newWizardTorch(p)
		w.SetPalette(testPalette)
		w.Resize(120, 44)
		w.Step(wtRest())
		if lit := wtLit(w, 120, 44); lit <= 100 {
			t.Errorf("%s drew only %d cells", p.name, lit)
		}
	}
}

func TestNegativeArtKeepsItsOwnDefaults(t *testing.T) {
	// raalien is negative art: a cutoff would eat the creature.
	p := wtPieceByName(t, "raalien")
	if p.darkcut != 0 || p.fit != wtContain {
		t.Fatalf("raalien defaults: darkcut %v fit %v", p.darkcut, p.fit)
	}
	if wtPieceByName(t, "wzfire").darkcut != 0.2 {
		t.Fatal("wzfire should cull more of its dense low end")
	}
}

func TestALandscapePanelDrawsTheArt(t *testing.T) {
	w := wtLandscape()
	w.Step(wtRest())
	if n := wtLit(w, 160, 60); n <= 500 {
		t.Fatalf("landscape panel drew only %d cells", n)
	}
}

func TestContainKeepsTheArtInsideThePanel(t *testing.T) {
	w := wtLandscape()
	ay, ax := w.toArt(0, 0)
	if !(ax <= 0.5 || ay <= 0.5) {
		t.Fatalf("contain cropped a corner: (%v, %v)", ay, ax)
	}
}

func TestStretchMapsPanelCornersToArtCorners(t *testing.T) {
	w := wtLandscape()
	w.fit = wtStretch
	ay, ax := w.toArt(0, 0)
	by, bx := w.toArt(float64(w.cols), float64(w.rows))
	if math.Abs(ay) > 1e-3 || math.Abs(ax) > 1e-3 ||
		math.Abs(by-wizardtorchRows) > 1e-3 || math.Abs(bx-wizardtorchCols) > 1e-3 {
		t.Fatalf("(%v,%v) (%v,%v)", ay, ax, by, bx)
	}
}

func TestDetailZoomsAboutTheCentre(t *testing.T) {
	w := wtLandscape()
	mc, mr := float64(w.cols)/2, float64(w.rows)/2
	y0, x0 := w.toArt(mc, mr)
	w.detail = 2.5
	y1, x1 := w.toArt(mc, mr)
	if math.Abs(y0-y1) > 1e-3 || math.Abs(x0-x1) > 1e-3 {
		t.Fatalf("centre moved: (%v,%v) -> (%v,%v)", y0, x0, y1, x1)
	}
	// Zooming in must sample less art across the same grid.
	w.detail = 0.4
	_, out := w.toArt(0, 0)
	w.detail = 3
	_, in := w.toArt(0, 0)
	if in <= out {
		t.Fatal("zooming in must narrow the window")
	}
}

func TestTheSampleIsBilinearAndEmptyOutside(t *testing.T) {
	w := wtLandscape()
	found := false
	for x := 0; x < wizardtorchCols-1; x++ {
		a, b := w.artSample(40, float64(x)), w.artSample(40, float64(x+1))
		if math.Abs(a-b) > 0.5 {
			found = true
			if mid := w.artSample(40, float64(x)+0.5); mid <= min(a, b) || mid >= max(a, b) {
				t.Fatalf("midpoint %v is not between %v and %v", mid, a, b)
			}
		}
	}
	if !found {
		t.Fatal("row 40 has no edge to test")
	}
	if w.artSample(-40, -40) != 0 || w.artSample(wizardtorchRows+40, wizardtorchCols+40) != 0 {
		t.Fatal("sampling outside the art must be empty, not wrapped")
	}
}

func TestTheRippleAdvancesWrapsAndIsPacedByTime(t *testing.T) {
	w := wtLandscape()
	for range 5 {
		w.Step(wtRest())
	}
	if w.t <= 0 {
		t.Fatal("the ripple never moved")
	}
	for range 2000 {
		w.Step(wtRest())
	}
	if w.t < 0 || w.t >= 1 {
		t.Fatalf("phase escaped 0..1: %v", w.t)
	}
	// The same elapsed time at two frame rates gives the same phase.
	fast, slow := wtLandscape(), wtLandscape()
	a := wtRest()
	a.DT = 0.02
	for range 50 {
		fast.Step(a)
	}
	a.DT = 0.1
	for range 10 {
		slow.Step(a)
	}
	if math.Abs(fast.t-slow.t) > 1e-6 {
		t.Fatalf("%v != %v", fast.t, slow.t)
	}
	// panefx pace: 0.10 of a cycle per second at speed 1.
	if math.Abs(slow.t-0.1) > 1e-6 {
		t.Fatalf("one second advanced the phase by %v, want 0.1", slow.t)
	}
}

func TestZeroSwirlIsAStillPicture(t *testing.T) {
	w := wtLandscape()
	w.swirl = 0
	w.Step(wtRest())
	ch, c, ok := w.Cell(80, 30)
	for range 20 {
		w.Step(wtRest())
	}
	if ch2, c2, ok2 := w.Cell(80, 30); ch != ch2 || c != c2 || ok != ok2 {
		t.Fatal("zero swirl still moved")
	}
}

func TestWizardTorchHearsTheMusic(t *testing.T) {
	// A kick lifts the highlights: the total brightness goes up, the set of
	// lit cells does not change (the silhouette is judged on raw shade).
	lum := func(w *WizardTorch) (sum float64, lit int) {
		for r := range w.rows {
			for c := range w.cols {
				if _, col, ok := w.Cell(c, r); ok {
					sum += float64(col.R) + float64(col.G) + float64(col.B)
					lit++
				}
			}
		}
		return
	}
	still := func() *WizardTorch { w := wtLandscape(); w.swirl = 0; return w }
	calm, hit := still(), still()
	calm.Step(wtRest())
	a := wtRest()
	a.Kick = 1
	hit.Step(a)
	lc, nc := lum(calm)
	lh, nh := lum(hit)
	if lh <= lc || nh != nc {
		t.Fatalf("kick: brightness %v -> %v, lit %d -> %d", lc, lh, nc, nh)
	}
	// Bass drive speeds the ripple.
	rest, bass := wtLandscape(), wtLandscape()
	b := wtRest()
	b.Bass = 1
	for range 30 {
		rest.Step(wtRest())
		bass.Step(b)
	}
	if bass.t <= rest.t {
		t.Fatalf("bass did not speed the ripple: %v vs %v", bass.t, rest.t)
	}
}

func TestWizardTorchDegenerateGrids(t *testing.T) {
	for _, p := range wtPieces() {
		w := newWizardTorch(p)
		w.Step(wtRest()) // before any Resize
		w.Resize(0, 0)
		w.Step(wtRest())
		if _, _, ok := w.Cell(0, 0); ok {
			t.Fatal("a 0x0 grid drew a cell")
		}
		w.Resize(-3, 5)
		w.Step(wtRest())
		w.Resize(7, 3)
		w.Step(wtRest())
		_, _, _ = w.Cell(6, 2)
	}
}

func BenchmarkWizardTorchStep(b *testing.B) {
	w := wtLandscape()
	w.Resize(80, 30)
	a := wtRest()
	for b.Loop() {
		w.Step(a)
	}
}
