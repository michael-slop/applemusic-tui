package fx

import "testing"

func fpBuilt(cols, rows int) *FramePlay {
	f := newFramePlay("fishloop", fishloopReel())
	f.SetPalette(testPalette)
	f.Resize(cols, rows)
	return f
}

// fpTick is a reactivity-0 step of dt seconds.
func fpTick(dt float64) Audio {
	a := wtRest()
	a.DT = dt
	return a
}

func TestFishloopReelMatchesTheRustSource(t *testing.T) {
	// Counts and a checksum of every (col,row,glyph,index) cell, computed by
	// the generator from panefx's fishloop_art.rs.
	wantCells := []int{98, 93, 89, 97, 78, 72, 83, 92, 99, 100, 109, 117, 124, 103, 95, 106, 86, 95, 96}
	if fishloopCols != 60 || fishloopRows != 30 || fishloopFPS != 12 || len(fishloopFrames) != 19 {
		t.Fatalf("reel is %dx%d @%d, %d frames", fishloopCols, fishloopRows, fishloopFPS, len(fishloopFrames))
	}
	var all []byte
	for i, fr := range fishloopFrames {
		if len(fr)%4 != 0 || len(fr)/4 != wantCells[i] {
			t.Fatalf("frame %d: %d bytes, want %d cells", i, len(fr), wantCells[i])
		}
		for k := 0; k < len(fr); k += 4 {
			// Every index in range, or a cell silently renders wrong.
			if int(fr[k]) >= fishloopCols || int(fr[k+1]) >= fishloopRows || int(fr[k+3]) >= len(fishloopPalette) || fr[k+2] <= ' ' {
				t.Fatalf("frame %d: bad cell % x", i, fr[k:k+4])
			}
		}
		all = append(all, fr...)
	}
	if got := wtFNV(all); got != 1442532305 {
		t.Fatalf("checksum %d", got)
	}
	f0, f18 := fishloopFrames[0], fishloopFrames[18]
	if f0[:4] != "\x0e\x02.\x09" || f18[len(f18)-4:] != "\x08\x0f#\x06" {
		t.Fatalf("first/last cells: % x / % x", f0[:4], f18[len(f18)-4:])
	}
	if fishloopPalette[0] != (RGB{194, 242, 97}) || fishloopPalette[11] != (RGB{0, 128, 0}) {
		t.Fatal("palette mismatch")
	}
}

func TestFishloopDrawsOnALandscapePanel(t *testing.T) {
	f := fpBuilt(160, 50)
	f.Step(fpTick(0.1))
	if n := wtLit(f, 160, 50); n <= 20 {
		t.Fatalf("only %d cells drawn", n)
	}
}

func TestFishloopAdvancesAndLoops(t *testing.T) {
	f := fpBuilt(80, 30)
	seen := map[int]bool{}
	for range 200 {
		f.Step(fpTick(0.1))
		if fi := f.frameIndex(); fi < 0 || fi >= len(fishloopFrames) {
			t.Fatalf("frame index escaped the reel: %d", fi)
		}
		seen[f.frameIndex()] = true
	}
	if len(seen) != len(fishloopFrames) {
		t.Fatalf("played %d of %d frames", len(seen), len(fishloopFrames))
	}
}

func TestFishloopIsPacedByTimeNotTicks(t *testing.T) {
	slow, fast, amtui := fpBuilt(80, 30), fpBuilt(80, 30), fpBuilt(80, 30)
	for range 5 {
		slow.Step(fpTick(0.2))
	}
	for range 50 {
		fast.Step(fpTick(0.02))
	}
	for range 30 {
		amtui.Step(fpTick(1.0 / 30))
	}
	// One second at the authored 12 fps: frame 12 on every panel rate.
	for _, f := range []*FramePlay{slow, fast, amtui} {
		if f.frameIndex() != 12 {
			t.Fatalf("after 1s: frame %d, want 12", f.frameIndex())
		}
	}
}

func TestFishloopEmptyCellsStayEmptyAndStretchFillsMore(t *testing.T) {
	f := fpBuilt(120, 40)
	f.Step(fpTick(0.1))
	if wtLit(f, 120, 40) >= 120*40 {
		t.Fatal("every cell was painted")
	}
	c, s := fpBuilt(160, 40), fpBuilt(160, 40)
	c.fit, s.fit = wtContain, wtStretch
	c.Step(fpTick(0.1))
	s.Step(fpTick(0.1))
	if wtLit(s, 160, 40) <= wtLit(c, 160, 40) {
		t.Fatal("stretch should fill more than contain")
	}
}

func TestFishloopResizeAndDegenerateGrids(t *testing.T) {
	f := fpBuilt(0, 0)
	f.Step(fpTick(0.1))
	if _, _, ok := f.Cell(0, 0); ok {
		t.Fatal("0x0 drew a cell")
	}
	f.Resize(80, 30)
	f.Step(fpTick(0.01))
	f.Resize(40, 20)
	f.Step(fpTick(0.01))
	if len(f.glyph) != 40*20 {
		t.Fatalf("grid is %d cells after a resize to 40x20", len(f.glyph))
	}
	g := newFramePlay("fishloop", fishloopReel())
	g.Step(fpTick(0.1)) // before Resize and SetPalette
}

func TestFishloopColoursFollowTheThemeInOrder(t *testing.T) {
	f := fpBuilt(10, 10)
	// The brightest reel colour must map brighter than the darkest.
	lum := func(c RGB) float64 { return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B) }
	var bi, di int
	for i, c := range fishloopPalette {
		if lum(c) > lum(fishloopPalette[bi]) {
			bi = i
		}
		if lum(c) < lum(fishloopPalette[di]) {
			di = i
		}
	}
	if lum(f.colours[bi]) <= lum(f.colours[di]) {
		t.Fatal("luminance order lost in the theme mapping")
	}
	for i := range f.base {
		if f.colours[i] != testPalette.At(f.base[i]) {
			t.Fatal("colours are not on the theme ramp")
		}
	}
}

func TestFishloopHearsTheMusic(t *testing.T) {
	calm, busy := fpBuilt(80, 30), fpBuilt(80, 30)
	a := fpTick(1.0 / 30)
	for i := range a.React {
		a.React[i] = 1
	}
	// 0.67 s: under one loop of the reel even at the fastest rate.
	for range 20 {
		calm.Step(fpTick(1.0 / 30))
		busy.Step(a)
	}
	if busy.pos <= calm.pos {
		t.Fatalf("busy music did not speed the swim: %v vs %v", busy.pos, calm.pos)
	}
	k := fpTick(1.0 / 30)
	k.Kick = 1
	before := calm.colours[0]
	calm.Step(k)
	if calm.colours[0] == before {
		t.Fatal("a kick did not lift the colours")
	}
}
