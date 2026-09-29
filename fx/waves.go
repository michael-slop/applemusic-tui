package fx

// "Waves": blackwaves, a procedural ASCII wave field, ported from panefx's
// waves.rs (itself a port of blackwaves.py). Its comments record
// measurements taken off a real reference clip, and those numbers ARE the
// design; they are carried over verbatim rather than re-derived:
//
//   - Reference distribution (695x1230 crop, greyscale):
//     p25=4 p50=14 p75=28 p90=42 p99=71 max~89: an almost entirely black
//     field carrying its detail in the top decile. A stock dense-to-sparse
//     ramp looks EMPTY on this material, which is why the luminance remap
//     exists at all.
//   - The field does NOT advect. Best whole-frame translation between
//     reference frames is (0,0), but only ~30% of crest edges coincide
//     between frames, so it is not static-under-moving-light either. It
//     deforms in place, in large coherent patches. Hence the swirl warp.
//   - Warp amplitude 0.012 * min(h,w). 0.045 moved crests about twice too far
//     and left the geometry visibly sliding.
//
// What makes it affordable in real time: the base field is five octaves of
// ridged, sheared value noise, computed ONCE per resize and cached. Each frame
// only bilinearly resamples that cache through a cheap low-frequency warp.
//
// The arithmetic is float32 like the Rust, so the calibrated constants
// (headroom, dark cut) land where they were tuned.
//
// Colours: panefx drew greyscale ink at 12 quantised brightnesses (25%..100%
// of a cool white). Here the same 12 steps climb the theme ramp, so troughs
// sit in the accent's dark end and crests in the theme's text colour.
//
// Cells: panefx ran waves on chunky 15x23 px cells (preferred_cell) so each
// glyph reads as a mark. The noise is sampled across the panel's extent
// whatever the grid, so only one thing assumes a cell shape: the shear, which
// is in cells per row. A terminal cell is ~1:2 rather than 15:23, so the tilt
// is scaled by 2*15/23 to keep the crests at the same angle on screen.
//
// Timing: the phase advances by real seconds (speed 1.0 x 0.15 per second, a
// seamless loop with period 1), so it moves at the original rate whatever
// amtui's frame rate.
//
// How it hears the music:
//   - Drive(Mid), smoothed over ~1/3 s, deepens the swirl (up to 1.8x): the
//     crests breathe harder while the band is busy, but stay put.
//   - Kick surges the phase: each beat pushes the swell forward (up to 3x
//     speed for the ~0.3 s the pulse lasts), a slow heave rather than a flash.
//   - Paused (Playing false) or reactivity 0: exactly the original.

import "math"

// wavesRamp is ordered by apparent ink coverage; short and low-contrast at
// the dark end, since the reference spends most of its area below
// luminance 16, so the first few rungs do the bulk of the work.
const wavesRamp = " .:-=+*#%@"

var wavesRunes = []rune(wavesRamp)

// Reference distribution: percentile -> luminance, from the measured crop.
var (
	wavesRefP = [7]float32{0, 25, 50, 75, 90, 99, 100}
	// The luminance side, normalised by its max (89). Kept as the measured
	// numbers so the knots stay explainable.
	wavesRefX = [7]float32{0, 4.0 / 89, 14.0 / 89, 28.0 / 89, 42.0 / 89, 71.0 / 89, 1}
)

const (
	// Lookup-table resolution for the shading and remap curves: far finer
	// than the 10-glyph ramp can resolve.
	wavesLUTN    = 257
	wavesLUTLast = float32(wavesLUTN - 1)
	// Distinct ink brightnesses (the Python's --ink-steps default of 12).
	wavesInkSteps = 12
	wavesOctaves  = 5
	wavesTilt     = 0.62
	wavesSwirl    = 0.55
	// Gamma < 1 lifts the crowded dark end apart; without it ~80% of cells
	// collapse onto one glyph.
	wavesGamma = 0.62
	// 1.35, not the 0.95 that matched the Python's histogram exactly: Michael
	// wanted the waves darker, and more blank cells are cheaper to draw.
	wavesHeadroom = 1.35
	// Cells at or below this luminance are not drawn at all. Tuned live by
	// Michael against the real thing (500 -> 450 -> 405 -> 345 -> 276). The
	// remap's floor would otherwise light EVERY cell with a near-invisible
	// glyph.
	wavesDarkCut = 0.276
	// The swirl's phase advance per second at speed 1.0.
	wavesRate = 0.15
	// Terminal cell (1:2) vs panefx's chosen 15x23 cell: see the header.
	wavesCellAspect = 2.0 * 15 / 23
	// Music.
	wavesMidSwirl  = 0.8
	wavesKickSpeed = 2.0
)

// wavesInterpRef is np.interp(x, REF_L/REF_L[-1], REF_P/100). Clamping, not
// extrapolating, exactly as numpy does. Getting this wrong sent every bright
// cell to the top rung.
func wavesInterpRef(x float32) float32 {
	xs := &wavesRefX
	if x <= xs[0] {
		return wavesRefP[0] / 100
	}
	if x >= xs[6] {
		return wavesRefP[6] / 100
	}
	for k := 1; k < len(xs); k++ {
		if x <= xs[k] {
			d := xs[k] - xs[k-1]
			var f float32
			if d > 1e-9 {
				f = (x - xs[k-1]) / d
			}
			return (wavesRefP[k-1] + (wavesRefP[k]-wavesRefP[k-1])*f) / 100
		}
	}
	return wavesRefP[6] / 100
}

func wavesFade(t float32) float32 { return t * t * t * (t*(t*6-15) + 10) }

// wavesHash01 is a deterministic hash -> [0,1). It stands in for numpy's
// seeded RNG grid: the same value for the same (seed, y, x) every time.
func wavesHash01(seed uint32, y, x int32) float32 {
	h := seed*0x9E3779B9 ^ uint32(y)*0x85EBCA6B ^ uint32(x)*0xC2B2AE35
	h ^= h >> 15
	h *= 0x2545F491
	h ^= h >> 13
	return float32(h>>8) / float32(1<<24)
}

// wavesValueNoise is smooth value noise on an h x w grid at the given cell
// frequency, bar-for-bar with the Python's _value_noise:
//
//   - The random grid is (freq+2) x (freq+2) and is INDEXED, not hashed at
//     arbitrary coordinates. Hashing per-sample gives noise with a different
//     character (measurably flatter gradients, a longer dark tail).
//   - Sample coordinates are i * freq / n (np.linspace, endpoint=False).
//   - y0 = int(ys) truncates; inputs are non-negative so that is a floor.
func wavesValueNoise(h, w int, freq float32, seed uint32) []float32 {
	g := int(freq) + 2
	grid := make([]float32, g*g)
	for i := range grid {
		grid[i] = wavesHash01(seed, int32(i/g), int32(i%g))
	}
	out := make([]float32, w*h)
	for y := range h {
		fy := float32(y) * freq / float32(h)
		y0 := int(fy)
		ty := wavesFade(fy - float32(y0))
		for x := range w {
			fx := float32(x) * freq / float32(w)
			x0 := int(fx)
			tx := wavesFade(fx - float32(x0))
			a := grid[y0*g+x0]
			b := grid[y0*g+x0+1]
			c := grid[(y0+1)*g+x0]
			d := grid[(y0+1)*g+x0+1]
			out[y*w+x] = (a*(1-tx)+b*tx)*(1-ty) + (c*(1-tx)+d*tx)*ty
		}
	}
	return out
}

// The shading and remap curves are pure functions of [0,1], so they are
// lookup tables rather than several powf per cell.
var wavesShadeLUT, wavesRemapLUT = func() (s, r [wavesLUTN]float32) {
	const floor = 0.055
	for i := range wavesLUTN {
		l := float64(i) / float64(wavesLUTLast)
		// ambient + 0.46*l^0.85 + 0.62*l^2.6 + 0.80*l^5.0
		s[i] = float32(0.10 + 0.46*math.Pow(l, 0.85) + 0.62*math.Pow(l, 2.6) + 0.80*math.Pow(l, 5))
		// interp_ref -> /headroom -> ^0.88 -> ^gamma
		p := float64(wavesInterpRef(float32(l)))
		lum := min(max(p/wavesHeadroom, 0), 1)
		v := floor + (1-floor)*math.Pow(lum, 0.88)
		r[i] = float32(math.Pow(min(max(v, 0), 1), wavesGamma))
	}
	return
}()

type waves struct {
	cols, rows int
	// The fixed wave geometry, computed once per resize.
	base []float32
	// Two low-frequency fields driving the warp, sampled in quadrature so the
	// warp sweeps through the frame instead of pulsing everywhere at once.
	warpA, warpB []float32
	// Per-frame scratch, so a frame allocates nothing.
	warped, lam []float32
	lum         []float32
	// What Cell draws, computed in Step: glyph index (0 = blank) and ink step.
	glyph []uint8
	ink   []uint8

	t    float32
	midS float64 // smoothed Drive(Mid)

	colours [wavesInkSteps]RGB
}

func newWaves() Effect {
	w := &waves{}
	w.Resize(0, 0)
	return w
}

func init() { Register("waves", 21, newWaves) }

func (w *waves) Name() string { return "waves" }

func (w *waves) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == w.cols && rows == w.rows && w.glyph != nil {
		return
	}
	w.cols, w.rows = cols, rows
	n := cols * rows
	w.lum = make([]float32, n)
	w.warped = make([]float32, n)
	w.lam = make([]float32, n)
	w.glyph = make([]uint8, n)
	w.ink = make([]uint8, n)
	w.rebuildBase()
	w.compute(wavesSwirl)
}

func (w *waves) SetPalette(p Palette) {
	// The original's 12 ink brightnesses, as 12 steps up the theme ramp.
	for q := range w.colours {
		w.colours[q] = p.At(float64(q) / (wavesInkSteps - 1))
	}
}

// rebuildBase builds five octaves of RIDGED value noise with a directional
// shear.
//
// Ridged (1 - |2n-1|, then squared) rather than plain value noise: the
// reference is creased everywhere, and plain fBm gives smooth blobs with flat
// faces between them.
//
// The shear is applied to the SAMPLING COORDINATES, not by rolling whole
// octaves. Rolling only slides crests sideways and leaves the structure
// axis-aligned, which reads as horizontal scan lines rather than water.
func (w *waves) rebuildBase() {
	h, wd := w.rows, w.cols
	if h == 0 || wd == 0 {
		w.base, w.warpA, w.warpB = nil, nil, nil
		return
	}
	tilt := float32(wavesTilt * wavesCellAspect)
	field := make([]float32, wd*h)
	amp, freq, norm := float32(1), float32(3), float32(0)
	fw := float32(wd)
	for o := range wavesOctaves {
		n := wavesValueNoise(h, wd, freq, 1000+uint32(o))
		shearMul := 0.4 + 0.3*float32(o)
		for y := range h {
			// sh = (arange(h) * tilt) % w, then idx = (cols + sh*mul) % w.
			sh := float32(math.Mod(float64(float32(y)*tilt), float64(fw)))
			for x := range wd {
				idx := float32(math.Mod(float64(float32(x)+sh*shearMul), float64(fw)))
				if idx < 0 {
					idx += fw
				}
				// LINEAR blend between the two straddling columns, not a cast
				// to int: integer snapping makes neighbouring rows land on the
				// same offset, which shows up as long flat horizontal stripes.
				fl := float32(math.Floor(float64(idx)))
				i0 := int(fl) % wd
				i1 := (i0 + 1) % wd
				fr := idx - fl
				v := n[y*wd+i0]*(1-fr) + n[y*wd+i1]*fr
				// Ridged, then SQUARED: field += amp * ridged**2.
				ridged := 1 - float32(math.Abs(float64(2*v-1)))
				field[y*wd+x] += amp * ridged * ridged
			}
		}
		norm += amp
		amp *= 0.58
		freq *= 2.03
	}
	for i := range field {
		field[i] /= max(norm, 1e-6)
	}
	w.base = field
	w.warpA = wavesValueNoise(h, wd, 2.0, 77)
	w.warpB = wavesValueNoise(h, wd, 2.7, 91)
}

func (w *waves) baseAt(y, x int) float32 {
	y %= w.rows
	if y < 0 {
		y += w.rows
	}
	x %= w.cols
	if x < 0 {
		x += w.cols
	}
	return w.base[y*w.cols+x]
}

func (w *waves) Step(a Audio) {
	var mid, kick float64
	if a.Playing {
		mid, kick = Drive(a.Mid), min(max(a.Kick, 0), 1)
	}
	w.midS += (mid - w.midS) * min(a.DT*3, 1)
	// The swirl is periodic in t with period 1, so the animation loops
	// seamlessly on its own.
	w.t = float32(math.Mod(float64(w.t)+a.DT*wavesRate*(1+wavesKickSpeed*kick), 1))
	w.compute(float32(wavesSwirl * (1 + wavesMidSwirl*w.midS)))
}

// compute warps the fixed geometry, shades it, and quantises it for Cell.
func (w *waves) compute(swirl float32) {
	h, wd := w.rows, w.cols
	if h == 0 || wd == 0 || w.base == nil {
		return
	}
	const tau = 2 * math.Pi
	phase := float64(tau * w.t)
	// "Small on purpose: the crests must stay put, only breathe."
	ampPx := 0.012 * float32(min(h, wd))

	// --- warp + sample ---
	warped, lam := w.warped, w.lam
	for y := range h {
		for x := range wd {
			i := y*wd + x
			wx := float32(math.Sin(phase+float64(w.warpA[i])*tau)) * swirl
			wy := float32(math.Cos(phase+float64(w.warpB[i])*tau)) * swirl
			sy := float32(y) + wy*ampPx
			sx := float32(x) + wx*ampPx
			y0f, x0f := float32(math.Floor(float64(sy))), float32(math.Floor(float64(sx)))
			fy, fx := sy-y0f, sx-x0f
			y0, x0 := int(y0f), int(x0f)
			warped[i] = w.baseAt(y0, x0)*(1-fx)*(1-fy) +
				w.baseAt(y0, x0+1)*fx*(1-fy) +
				w.baseAt(y0+1, x0)*(1-fx)*fy +
				w.baseAt(y0+1, x0+1)*fx*fy
		}
	}

	// --- shade ---
	//
	// Diffuse and specular are ADDED, not multiplied: multiplying drives the
	// result to zero wherever either factor is small and hollows the frame
	// out. Three lobes rather than two: diffuse + one narrow specular leaves
	// a gap in the middle of the histogram that no remap can fill.
	lx, ly := float32(-0.55), float32(-0.83)
	ln := float32(math.Sqrt(float64(lx*lx + ly*ly)))
	lx, ly = lx/ln, ly/ln

	lamMax := float32(1e-6)
	for y := range h {
		for x := range wd {
			// np.gradient: interior uses a CENTRAL difference over a spacing
			// of 2; the first and last rows/columns a one-sided difference
			// over spacing 1. Scaling the edges like the interior flattens
			// the lit region.
			var dy, dx float32
			switch {
			case h == 1:
			case y == 0:
				dy = warped[wd+x] - warped[x]
			case y == h-1:
				dy = warped[(h-1)*wd+x] - warped[(h-2)*wd+x]
			default:
				dy = (warped[(y+1)*wd+x] - warped[(y-1)*wd+x]) * 0.5
			}
			switch {
			case wd == 1:
			case x == 0:
				dx = warped[y*wd+1] - warped[y*wd]
			case x == wd-1:
				dx = warped[y*wd+wd-1] - warped[y*wd+wd-2]
			default:
				dx = (warped[y*wd+x+1] - warped[y*wd+x-1]) * 0.5
			}
			v := max(-(dx*lx + dy*ly), 0)
			lam[y*wd+x] = v
			lamMax = max(lamMax, v)
		}
	}

	// shade() tail, bar for bar:
	//   lam /= lam.max() + 1e-6
	//   v = ambient + 0.46*lam**0.85 + 0.62*lam**2.6 + 0.80*lam**5.0
	//   return v / (v.max() + 1e-6)
	// Both normalisations are load-bearing and NOT interchangeable with one
	// at the end.
	lamScale := wavesLUTLast / (lamMax + 1e-6)
	vmax := float32(0)
	for i, l := range lam {
		v := wavesLUT(&wavesShadeLUT, l*lamScale)
		lam[i] = v
		vmax = max(vmax, v)
	}

	// to_reference_levels(), bar for bar:
	//   x = x - x.min(); x /= x.max() + 1e-6; then the remap curve.
	// The second step divides by the max OF THE SHIFTED ARRAY, not by
	// (max - min) of the original: using a span lifted the dark half off its
	// floor (p50 0.27 against the Python's 0.17).
	lo := float32(math.MaxFloat32)
	for i := range lam {
		lam[i] /= vmax + 1e-6
		lo = min(lo, lam[i])
	}
	shiftedMax := float32(0)
	for i := range lam {
		lam[i] -= lo
		shiftedMax = max(shiftedMax, lam[i])
	}
	xScale := wavesLUTLast / (shiftedMax + 1e-6)
	n := len(wavesRunes)
	for i, l := range lam {
		v := wavesLUT(&wavesRemapLUT, l*xScale)
		w.lum[i] = v
		// Cells at or below the dark cut are not drawn at all.
		g := 0
		if v > wavesDarkCut {
			g = min(int(v*float32(n-1)+0.5), n-1)
		}
		w.glyph[i] = uint8(g)
		// Glyph choice carries the coarse steps; ink brightness carries
		// everything between them, QUANTISED to 12 levels so the renderer
		// can batch runs of one colour.
		w.ink[i] = uint8(min(int(v*wavesInkSteps), wavesInkSteps-1))
	}
}

// wavesLUT reads a table at fractional index fi with linear interpolation.
func wavesLUT(t *[wavesLUTN]float32, fi float32) float32 {
	fi = min(max(fi, 0), wavesLUTLast)
	i0 := int(fi)
	fr := fi - float32(i0)
	a, b := t[i0], t[min(i0+1, wavesLUTN-1)]
	return a + (b-a)*fr
}

func (w *waves) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= w.cols || row >= w.rows {
		return 0, RGB{}, false
	}
	i := row*w.cols + col
	g := w.glyph[i]
	if g == 0 {
		return 0, RGB{}, false
	}
	return wavesRunes[g], w.colours[w.ink[i]], true
}
