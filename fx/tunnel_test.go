package fx

import (
	"testing"
)

func builtTunnel(cols, rows int) *tunnel {
	t := newTunnel().(*tunnel)
	t.SetPalette(testPalette)
	t.Resize(cols, rows)
	return t
}

// tnlFlat is a playing frame with every band at v.
func tnlFlat(v float64) Audio {
	a := silence()
	a.Playing = true
	for i := range a.React {
		a.React[i] = v
	}
	return a
}

// litSpan is the topmost and bottommost drawn row in a column.
func litSpan(e Effect, col, rows int) (top, bottom int) {
	top, bottom = rows, -1
	for r := range rows {
		if _, _, ok := e.Cell(col, r); ok {
			top, bottom = min(top, r), max(bottom, r)
		}
	}
	return top, bottom
}

// The torus's rule on a ring: a bass spike bulges the nearest ring at the
// FLOOR (bass lives at the bottom) and leaves the ceiling (treble) alone.
func TestTunnelBassBulgesTheFloorNotTheCeiling(t *testing.T) {
	const w, h = 60, 30
	calm, hit := builtTunnel(w, h), builtTunnel(w, h)
	for range 60 {
		calm.Step(tnlFlat(0.3))
		hit.Step(tnlFlat(0.3))
	}
	a := tnlFlat(0.3)
	for i := range 4 {
		a.React[i] = 1
	}
	calm.Step(tnlFlat(0.3))
	hit.Step(a)
	ct, cb := litSpan(calm, w/2, h)
	ht, hb := litSpan(hit, w/2, h)
	if hb <= cb {
		t.Errorf("a bass spike should push the ring's floor down: bottom row %d vs %d", hb, cb)
	}
	if ht != ct {
		t.Errorf("a bass spike should leave the ceiling alone: top row %d vs %d", ht, ct)
	}
	// ...and the reverse for treble.
	a = tnlFlat(0.3)
	for i := 28; i < 32; i++ {
		a.React[i] = 1
	}
	treb := builtTunnel(w, h)
	for range 60 {
		treb.Step(tnlFlat(0.3))
	}
	treb.Step(a)
	tt, tb := litSpan(treb, w/2, h)
	if tt >= ct || tb != cb {
		t.Errorf("a treble spike should lift the ceiling only: top %d vs %d, bottom %d vs %d", tt, ct, tb, cb)
	}
}

// Bands are mirrored left/right: the picture is symmetric whatever plays.
func TestTunnelIsLeftRightSymmetric(t *testing.T) {
	for _, w := range []int{40, 41} {
		tn := builtTunnel(w, 16)
		for i := range 40 {
			tn.Step(loud(i))
		}
		for r := range 16 {
			for c := range w {
				ch1, c1, ok1 := tn.Cell(c, r)
				ch2, c2, ok2 := tn.Cell(w-1-c, r)
				if ch1 != ch2 || c1 != c2 || ok1 != ok2 {
					t.Fatalf("%d wide: (%d,%d) and its mirror differ", w, c, r)
				}
			}
		}
	}
}

// Depth is time: a spectrum shown on the nearest ring moves to smaller,
// farther rings as the music plays on.
func TestTunnelThePastRecedesIntoSmallerRings(t *testing.T) {
	tn := builtTunnel(60, 30)
	for range 30 {
		tn.Step(tnlFlat(0.2))
	}
	a := tnlFlat(0.2)
	a.React[0], a.React[1] = 1, 1
	for range 3 { // 0.1 s of bass
		tn.Step(a)
	}
	loudest := func() (ring int, radius float64) {
		for j := range tnlRings {
			if lv := tn.level[j][0]; lv > tn.level[ring][0] {
				ring = j
			}
		}
		return ring, tn.outline[ring][0]
	}
	prevRing, prevR := loudest()
	if prevRing > 1 {
		t.Fatalf("the spike should start on the nearest rings, got ring %d", prevRing)
	}
	for step := range 4 {
		for range 6 { // 0.2 s
			tn.Step(tnlFlat(0.2))
		}
		ring, r := loudest()
		if ring <= prevRing || r >= prevR {
			t.Fatalf("step %d: the spike should recede (ring %d -> %d, radius %.1f -> %.1f)", step, prevRing, ring, prevR, r)
		}
		prevRing, prevR = ring, r
	}
}

// The rings travel on music time: the same seconds in different frame
// sizes give the same picture.
func TestTunnelIsPacedByTimeNotFrameCount(t *testing.T) {
	fast, slow := builtTunnel(30, 12), builtTunnel(30, 12)
	for i := range 50 {
		a := loud(i / 5)
		a.Kick, a.DT = 0, 0.02
		fast.Step(a)
		if i%5 == 4 {
			a.DT = 0.1
			slow.Step(a)
		}
	}
	if frame(fast, 30, 12) != frame(slow, 30, 12) {
		t.Fatal("the same second of music drew different tunnels at different frame rates")
	}
}

// Kick is a global pulse: the rings flare and brighten.
func TestTunnelKickFlares(t *testing.T) {
	calm, hit := builtTunnel(60, 20), builtTunnel(60, 20)
	for range 40 {
		calm.Step(tnlFlat(0.5))
		hit.Step(tnlFlat(0.5))
	}
	a := tnlFlat(0.5)
	a.Kick = 1
	for range 3 {
		calm.Step(tnlFlat(0.5))
		hit.Step(a)
	}
	if lc, lh := litWeight(calm, 60, 20), litWeight(hit, 60, 20); lh <= lc*1.1 {
		t.Fatalf("a kick should flare the rings: weight %.0f vs %.0f", lh, lc)
	}
}

// Silence is small faint resting circles, not blank.
func TestTunnelSilenceIsSmallFaintCircles(t *testing.T) {
	const w, h = 60, 20
	hush, full := builtTunnel(w, h), builtTunnel(w, h)
	for range 60 {
		hush.Step(tnlFlat(0))
		full.Step(tnlFlat(1))
	}
	lit := litWeight(hush, w, h)
	if lit == 0 {
		t.Fatal("silence should leave resting circles, not a blank panel")
	}
	_, hb := litSpan(hush, w/2, h)
	_, fb := litSpan(full, w/2, h)
	if hb >= fb {
		t.Fatalf("resting circles should be smaller than full ones: bottom row %d vs %d", hb, fb)
	}
	if lit >= litWeight(full, w, h)/2 {
		t.Fatalf("resting circles should be faint: weight %.0f vs %.0f", lit, litWeight(full, w, h))
	}
}

func TestTunnelStepDoesNotAllocate(t *testing.T) {
	tn := builtTunnel(80, 24)
	i := 0
	if n := testing.AllocsPerRun(50, func() { tn.Step(loud(i)); i++ }); n != 0 {
		t.Fatalf("Step allocates %v times", n)
	}
}
