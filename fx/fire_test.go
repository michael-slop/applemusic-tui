package fx

import "testing"

func newFireAt(cols, rows int) *fire {
	f := newFire().(*fire)
	f.SetPalette(testPalette)
	f.Resize(cols, rows)
	return f
}

// fireAudio is a playing frame with every band at base except band b at v.
func fireAudio(base float64, b int, v, dt float64) Audio {
	a := Audio{Playing: true, Reactivity: 1, DT: dt}
	for i := range a.React {
		a.React[i] = base
	}
	if b >= 0 {
		a.React[b] = v
	}
	return a
}

// fireBandCol is the column whose centre sits exactly on band b.
func fireBandCol(cols, b int) int { return int(float64(b) / 31 * float64(cols)) }

// fireHighestLit is the topmost lit row in column c, -1 if none.
func fireHighestLit(f *fire, c int) int {
	for r := range f.rows {
		if _, _, ok := f.Cell(c, r); ok {
			return r
		}
	}
	return -1
}

// The column of a spiked band lights; far-away columns do not.
func TestFireSpikedBandLightsItsColumnOnly(t *testing.T) {
	const w, h = 62, 20
	f := newFireAt(w, h)
	for range 60 {
		f.Step(fireAudio(0, 16, 1, 1.0/30))
	}
	c := fireBandCol(w, 16)
	if ch, _, ok := f.Cell(c, h-1); !ok || ch != fireRamp[len(fireRamp)-1] {
		t.Fatalf("spiked column's base is %q, want the hottest glyph", ch)
	}
	if fireHighestLit(f, c) > h/2 {
		t.Fatal("a sustained spike should burn up the column")
	}
	for _, far := range []int{0, 5, w - 6, w - 1} {
		if r := fireHighestLit(f, far); r != h-1 {
			t.Fatalf("column %d should show only the ember line, lit up to row %d", far, r)
		}
		if ch, _, _ := f.Cell(far, h-1); ch != fireRamp[1] {
			t.Fatalf("column %d ember is %q", far, ch)
		}
	}
	// Bass is on the left, treble on the right: not mirrored.
	g := newFireAt(w, h)
	for range 30 {
		g.Step(fireAudio(0, 0, 1, 1.0/30))
	}
	if fireHighestLit(g, 0) > h/2 || fireHighestLit(g, w-1) != h-1 {
		t.Fatal("a bass spike should burn on the left only")
	}
}

// A short burst appears at the bottom, then rises and fades.
func TestFireSpikeRises(t *testing.T) {
	const w, h = 32, 30
	f := newFireAt(w, h)
	c := fireBandCol(w, 10)
	for range 3 {
		f.Step(fireAudio(0, 10, 1, 1.0/30))
	}
	// Lit near the bottom right away.
	if r := fireHighestLit(f, c); r < h-4 {
		t.Fatalf("burst should start at the bottom, lit up to row %d", r)
	}
	lowest := func() int { // lowest lit row above the ember line
		for r := h - 2; r >= 0; r-- {
			if _, _, ok := f.Cell(c, r); ok {
				return r
			}
		}
		return -1
	}
	prev := h
	for step := range 4 {
		for range 6 {
			f.Step(fireAudio(0, -1, 0, 1.0/30))
		}
		r := lowest()
		if r < 0 {
			break // faded out: fine once it has risen
		}
		if r >= prev {
			t.Fatalf("step %d: burst at row %d, was %d -- not rising", step, r, prev)
		}
		prev = r
	}
	if prev > h-6 {
		t.Fatalf("burst never rose (last seen at row %d)", prev)
	}
}

// The scroll runs on DT, i.e. at the music's pace.
func TestFireScrollFollowsDT(t *testing.T) {
	const w, h = 32, 40
	run := func(dt float64) int {
		f := newFireAt(w, h)
		f.Step(fireAudio(0, 10, 1, fireStep))
		for range 15 {
			f.Step(fireAudio(0, -1, 0, dt))
		}
		c := fireBandCol(w, 10)
		for r := range h - 1 {
			if _, _, ok := f.Cell(c, r); ok {
				return h - 1 - r // rows risen
			}
		}
		return 0
	}
	fast, slow := run(fireStep), run(fireStep/3)
	if fast < 14 || fast > 17 || slow < 4 || slow > 7 {
		t.Fatalf("15 frames rose %d rows at full pace, %d at a third (want ~16 and ~6)", fast, slow)
	}
}

func TestFireSilenceIsAnEmberLine(t *testing.T) {
	const w, h = 40, 16
	f := newFireAt(w, h)
	for range 30 {
		f.Step(fireAudio(0.9, -1, 0, 1.0/30))
	}
	for range 60 {
		f.Step(fireAudio(0, -1, 0, 1.0/30))
	}
	for c := range w {
		if ch, _, ok := f.Cell(c, h-1); !ok || ch != fireRamp[1] {
			t.Fatalf("column %d bottom is %q, want the ember '.'", c, ch)
		}
		if r := fireHighestLit(f, c); r != h-1 {
			t.Fatalf("column %d lit up to row %d in silence", c, r)
		}
	}
}

func TestFireKickPulses(t *testing.T) {
	const w, h = 40, 16
	calm, hit := newFireAt(w, h), newFireAt(w, h)
	for range 30 {
		a := fireAudio(0.4, -1, 0, 1.0/30)
		calm.Step(a)
		a.Kick = 1
		hit.Step(a)
	}
	if lc, lh := litWeight(calm, w, h), litWeight(hit, w, h); lh <= lc*1.1 {
		t.Fatalf("a kick should brighten the fire: %.0f vs %.0f", lh, lc)
	}
}

func TestFireStepAndCellDoNotAllocate(t *testing.T) {
	f := newFireAt(80, 24)
	a := fireAudio(0.5, 3, 1, 1.0/30)
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
