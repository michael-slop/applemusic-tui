package fx

import (
	"math"
	"testing"
)

func TestSpectrumRestsAtZero(t *testing.T) {
	a := silence()
	a.Playing = true
	for _, s := range []float64{0, 0.3, 0.5, 1} {
		if d := a.Spectrum(s, false); d != 0 {
			t.Fatalf("at rest the deviation must be 0, got %v at %v", d, s)
		}
	}
	a.React[0] = 1
	a.Playing = false
	if d := a.Spectrum(0, false); d != 0 {
		t.Fatal("paused must read 0 whatever the bands say")
	}
}

func TestSpectrumMapsBandsAlongTheAxis(t *testing.T) {
	a := silence()
	a.Playing = true
	a.React[0], a.React[31] = 1, 0
	if d := a.Spectrum(0, false); d != 0.5 {
		t.Fatalf("s=0 should read band 0: %v", d)
	}
	if d := a.Spectrum(1, false); d != -0.5 {
		t.Fatalf("s=1 should read band 31: %v", d)
	}
	// Mirrored: bass in the middle, treble at both ends.
	if d := a.Spectrum(0.5, true); d != 0.5 {
		t.Fatalf("mirrored centre should read band 0: %v", d)
	}
	if l, r := a.Spectrum(0, true), a.Spectrum(1, true); l != -0.5 || r != -0.5 {
		t.Fatalf("mirrored ends should read band 31: %v %v", l, r)
	}
	if l, r := a.Spectrum(0.2, true), a.Spectrum(0.8, true); math.Abs(l-r) > 1e-12 {
		t.Fatalf("mirrored must be symmetric: %v vs %v", l, r)
	}
}

func TestSpectrumBlendsNeighbours(t *testing.T) {
	a := silence()
	a.Playing = true
	a.React[10], a.React[11] = 1, 0
	mid := (10.5) / 31
	if d := a.Spectrum(mid, false); math.Abs(d) > 1e-9 {
		t.Fatalf("halfway between a high and a low band should read ~0, got %v", d)
	}
}
