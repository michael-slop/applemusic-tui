package fx

import "testing"

func newFlamesAt(cols, rows int) *flames {
	f := newFlames().(*flames)
	f.SetPalette(testPalette)
	f.Resize(cols, rows)
	return f
}

// flamesAudio is a playing frame with every band at base, except bands
// lo..hi at spike.
func flamesAudio(base float64, lo, hi int, spike float64) Audio {
	a := Audio{Playing: true, Reactivity: 1, DT: 1.0 / 30}
	for i := range a.React {
		a.React[i] = base
		if i >= lo && i <= hi {
			a.React[i] = spike
		}
	}
	return a
}

// flamesColHeight is how many cells of column c are lit.
func flamesColHeight(f *flames, c int) int {
	n := 0
	for r := range f.height {
		if _, _, ok := f.Cell(c, r); ok {
			n++
		}
	}
	return n
}

func TestFlamesRampIsTheGists(t *testing.T) {
	if string(flamesChars[:]) != " .:^*xsS#$" {
		t.Fatalf("ramp %q is not the gist's", string(flamesChars[:]))
	}
}

// A bass spike raises the centre columns, not the edges; a treble spike the
// reverse.
func TestFlamesBassBurnsInTheCentreTrebleAtTheEdges(t *testing.T) {
	const w, h = 64, 24
	bass, treble := newFlamesAt(w, h), newFlamesAt(w, h)
	for range 30 {
		bass.Step(flamesAudio(0.2, 0, 3, 1))
		treble.Step(flamesAudio(0.2, 28, 31, 1))
	}
	mid, edge := w/2, 0
	if bc, be := flamesColHeight(bass, mid), flamesColHeight(bass, edge); bc < be+h/3 {
		t.Fatalf("bass spike: centre %d rows, edge %d rows", bc, be)
	}
	if tc, te := flamesColHeight(treble, mid), flamesColHeight(treble, edge); te < tc+h/3 {
		t.Fatalf("treble spike: centre %d rows, edge %d rows", tc, te)
	}
	// A full band nearly reaches the top.
	if bc := flamesColHeight(bass, mid); bc < h*3/4 {
		t.Fatalf("a full band reaches only %d of %d rows", bc, h)
	}
}

func TestFlamesIsSymmetric(t *testing.T) {
	for _, w := range []int{1, 7, 40, 61} {
		f := newFlamesAt(w, 18)
		a := flamesAudio(0.3, 0, 31, 0.3)
		for i := range a.React {
			a.React[i] = float64(i) / 31
		}
		a.Kick = 0.6
		for range 20 {
			f.Step(a)
		}
		for r := range f.height {
			for c := range w {
				c1, col1, ok1 := f.Cell(c, r)
				c2, col2, ok2 := f.Cell(w-1-c, r)
				if c1 != c2 || col1 != col2 || ok1 != ok2 {
					t.Fatalf("w=%d: cell (%d,%d) differs from its mirror", w, c, r)
				}
			}
		}
	}
}

// Silence is a low ember line along the bottom, lit in every column.
func TestFlamesSilenceIsAnEmberLine(t *testing.T) {
	const w, h = 50, 30
	f := newFlamesAt(w, h)
	for range 60 {
		f.Step(flamesAudio(0.9, 0, -1, 0))
	}
	for range 60 {
		a := flamesAudio(0, 0, -1, 0)
		a.Playing, a.DT = false, a.DT*0.04
		f.Step(a)
	}
	for c := range w {
		if _, _, ok := f.Cell(c, h-1); !ok {
			t.Fatalf("column %d has no ember on the bottom row", c)
		}
		if n := flamesColHeight(f, c); n > h/5 {
			t.Fatalf("column %d still %d rows tall in silence", c, n)
		}
	}
}

// A steady spectrum gives identical frames: the texture is fixed in space.
func TestFlamesSteadySpectrumIsStill(t *testing.T) {
	f := newFlamesAt(40, 16)
	a := flamesAudio(0.4, 5, 9, 0.8)
	for range 40 {
		f.Step(a)
	}
	before := frame(f, 40, 16)
	for range 30 {
		f.Step(a)
	}
	if frame(f, 40, 16) != before {
		t.Fatal("a steady spectrum changed the picture")
	}
}

// The kick is a global pulse: taller and hotter everywhere.
func TestFlamesKickPulses(t *testing.T) {
	const w, h = 40, 20
	calm, hit := newFlamesAt(w, h), newFlamesAt(w, h)
	for range 30 {
		a := flamesAudio(0.5, 0, -1, 0)
		calm.Step(a)
		a.Kick = 1
		hit.Step(a)
	}
	if lc, lh := litWeight(calm, w, h), litWeight(hit, w, h); lh <= lc*1.1 {
		t.Fatalf("a kick should brighten the fire: %.0f vs %.0f", lh, lc)
	}
}

// The glyph runs hot at the base to cool at the tip of each flame.
func TestFlamesHotBaseCoolTip(t *testing.T) {
	f := newFlamesAt(20, 30)
	for range 30 {
		f.Step(flamesAudio(0.8, 0, -1, 0))
	}
	c := 10
	base, _, _ := f.Cell(c, 29)
	top := -1
	for r := range 30 {
		if _, _, ok := f.Cell(c, r); ok {
			top = r
			break
		}
	}
	tip, _, _ := f.Cell(c, top)
	idx := func(r rune) int {
		for i, g := range flamesChars {
			if g == r {
				return i
			}
		}
		return -1
	}
	if idx(base) <= idx(tip)+3 {
		t.Fatalf("base %q should be much hotter than tip %q", base, tip)
	}
}

func TestFlamesStepAndCellDoNotAllocate(t *testing.T) {
	f := newFlamesAt(80, 24)
	a := flamesAudio(0.6, 0, 3, 1)
	if n := testing.AllocsPerRun(20, func() {
		f.Step(a)
		for r := range 24 {
			for c := range 80 {
				f.Cell(c, r)
			}
		}
	}); n != 0 {
		t.Fatalf("%v allocations per frame", n)
	}
}
