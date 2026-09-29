package fx

import (
	"math"
	"testing"
)

func runWaves(cols, rows, frames int) *waves {
	w := newWaves().(*waves)
	w.SetPalette(testPalette)
	w.Resize(cols, rows)
	for range frames {
		w.Step(silence())
	}
	return w
}

// np.interp does NOT extrapolate. Getting this wrong sent every bright cell
// to the top rung.
func TestWavesInterpClampsLikeNumpy(t *testing.T) {
	for x, want := range map[float32]float32{-1: 0, 0: 0, 1: 1, 99: 1} {
		if got := wavesInterpRef(x); math.Abs(float64(got-want)) > 1e-6 {
			t.Errorf("interp(%v) = %v, want %v", x, got, want)
		}
	}
	prev := float32(-1)
	for i := range 21 {
		v := wavesInterpRef(float32(i) / 20)
		if v < prev-1e-6 {
			t.Fatalf("interp not monotonic at %d", i)
		}
		prev = v
	}
}

func TestWavesLuminanceStaysInRange(t *testing.T) {
	w := runWaves(80, 25, 0)
	for range 40 {
		w.Step(silence())
		for _, v := range w.lum {
			if !(v >= 0 && v <= 1) {
				t.Fatalf("luminance %v out of range", v)
			}
		}
	}
}

// The measured reference is an almost entirely black field with detail in
// the top decile. A port that lights up most cells has lost the look.
func TestWavesFieldIsMostlyDarkLikeTheReference(t *testing.T) {
	w := runWaves(120, 40, 30)
	dark := 0
	for _, v := range w.lum {
		if v < 0.35 {
			dark++
		}
	}
	if f := float64(dark) / float64(len(w.lum)); f <= 0.25 {
		t.Fatalf("field is too bright: only %.0f%% dark", f*100)
	}
}

// Measured on the Python original at 200x60: '@' 0.3%. The port once emitted
// ~10x that because the interp extrapolated instead of clamping.
func TestWavesTopRungStaysRare(t *testing.T) {
	w := runWaves(200, 60, 20)
	top, seen := 0, map[rune]bool{}
	for y := range 60 {
		for x := range 200 {
			if g, _, ok := w.Cell(x, y); ok {
				seen[g] = true
				if g == '@' {
					top++
				}
			}
		}
	}
	if pct := float64(top) / (200 * 60) * 100; pct >= 2 {
		t.Fatalf("top rung '@' at %.1f%%; reference is 0.3%%", pct)
	}
	if len(seen) < 4 {
		t.Fatalf("only %d distinct glyphs: the field is flat", len(seen))
	}
}

// The music: a busy mid band and kicks move the field further than silence.
func TestWavesMidAndKickStirTheField(t *testing.T) {
	drift := func(loud bool) float64 {
		w := runWaves(80, 25, 5)
		first := append([]float32(nil), w.lum...)
		for i := range 15 {
			a := silence()
			if loud {
				a.Playing, a.Mid = true, 1
				if i%8 == 0 {
					a.Kick = 1
				}
			}
			w.Step(a)
		}
		var d float64
		for i, v := range w.lum {
			d += math.Abs(float64(v - first[i]))
		}
		return d
	}
	if q, l := drift(false), drift(true); l <= q*1.2 {
		t.Fatalf("music should stir the waves: quiet %v, loud %v", q, l)
	}
}
