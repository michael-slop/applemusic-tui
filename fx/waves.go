package fx

// Waves: the spectrum as a landscape (the Unknown Pleasures plot, a 3D
// spectrum seen from above the ground).
//
// Columns are frequency, bass at the left to treble at the right, not
// mirrored, full detail. Rows are time. The picture is a stack of ridge
// lines: the nearest one along the bottom is the live spectrum, and each line
// further up (farther away) is the spectrum from one more wavStep seconds of
// music ago, read from a History and interpolated between its entries. A
// ridge's height at a column is the band there, the torus's rule on a line:
//
//	height(x) = amplitude(depth) × BandAt(x)
//
// so silence is a stack of flat lines, a steady spectrum the same ridge
// repeated into the distance, and a band that jumps raises a peak that then
// travels up the stack, shrinking, as it ages. Depth is perspective: farther
// ridges sit higher, are shorter, narrower and dimmer. They are drawn near
// to far with a per-column horizon, so a nearer ridge hides whatever lies
// behind and below its line, as in the classic plot.
//
// The ridges sit at fixed depths; only what they carry flows (FreeMotion
// "scroll"). Time runs on DT, which amtui scales to the music's pace (4%
// when silent), so the landscape rolls back at the music's pace; a steady
// spectrum draws a steady picture. Step rasterises into a cell buffer; Cell
// is a lookup. Cells are ~1:2; a line's height within a cell picks between
// '¯', '-' and '_', and steep flanks are drawn with '/', '\' and '|'.
//
// How it hears the music:
//   - React (the per-band reactive level, slider-scaled) is the height and
//     brightness of every ridge at its band's columns.
//   - Depth is the song's recent past: the History advances every wavStep of
//     DT, so the ridges recede at the music's pace.
//   - Kick flares the nearest ridge: its peaks jump up to 35% taller and it
//     brightens, settling with the kick.
//   - Silence: React decays to 0, so every ridge lies flat (the resting
//     silhouette); once the last of the music has rolled back (at the silent
//     pace) nothing moves.

import "math"

const (
	wavStep  = 0.05 // seconds of music between ridges (half of 0.1: twice the lines over the same stretch of song)
	wavMax   = 48   // most ridges drawn (one per row up to 48)
	wavPersp = 0.15 // ridge j sits at depth 1 + j·wavPersp
	wavAmp   = 0.45 // the nearest ridge at full level rises this much of the panel (0.55 hid too many of the denser ridges)
	wavHoriz = 0.28 // the farthest baseline, as a fraction of the panel from the top
	wavFlare = 0.35 // how much taller a kick makes the nearest ridge
	wavGlow  = 0.4  // how much a kick brightens it
)

// wavColourSteps is how many distinct colours the theme ramp is sampled into:
// Render emits one escape per colour run, so a small table keeps runs long.
const wavColourSteps = 64 // fine enough that 48 ridges each get their own shade

func wavLUT(p Palette) (lut [wavColourSteps]RGB) {
	for i := range lut {
		lut[i] = p.At(float64(i) / float64(wavColourSteps-1))
	}
	return lut
}

func wavLUTAt(lut *[wavColourSteps]RGB, t float64) RGB {
	i := int(t*float64(wavColourSteps-1) + 0.5)
	return lut[min(max(i, 0), wavColourSteps-1)]
}

// wavGlide eases cur toward target with time constant tau seconds.
func wavGlide(cur, target, dt, tau float64) float64 {
	return cur + (target-cur)*(1-math.Exp(-dt/tau))
}

// wavDT is the frame's elapsed time, clamped so a stalled frame cannot fling
// the landscape forward.
func wavDT(a Audio) float64 {
	dt := a.DT
	if a.Wall > 0 {
		dt = a.Wall // the song's stream scrolls in real time (see Audio.Wall)
	}
	if !(dt > 0) {
		return 0
	}
	return min(dt, 0.25)
}

// wavBand is Audio.BandAt(s, false) for a stored spectrum.
func wavBand(sp *[32]float64, s float64) float64 {
	x := min(max(s, 0), 1) * 31
	i := int(x)
	if i >= 31 {
		return sp[31]
	}
	f := x - float64(i)
	return sp[i]*(1-f) + sp[i+1]*f
}

type wavCell struct {
	ch rune
	c  RGB
	ok bool
}

type waves struct {
	cols, rows int
	n          int // ridges at this size

	hist *History
	acc  float64 // seconds of music since the last push
	kick float64 // smoothed
	lut  [wavColourSteps]RGB

	// Per-frame, per ridge x column: the line's y (rows, down; NaN where the
	// ridge does not reach) and the band level there.
	ys, lv []float64
	top    []int // per column: the highest row a nearer ridge has covered
	cells  []wavCell
}

func newWaves() Effect { return &waves{hist: NewHistory(wavMax)} }

func init() { Register("waves", 21, newWaves) }

func (w *waves) Name() string { return "waves" }

// FreeMotion: the landscape rolls back as the song's history flows.
func (w *waves) FreeMotion() string { return "scroll" }

func (w *waves) SetPalette(p Palette) { w.lut = wavLUT(p) }

func (w *waves) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == w.cols && rows == w.rows && w.cells != nil {
		return
	}
	w.cols, w.rows = cols, rows
	// About one ridge per row. Perspective packs the far ones toward the
	// horizon and occlusion keeps them apart, so the extra lines add shades
	// to the gradient without flattening the depth.
	w.n = min(max(rows, 1), wavMax)
	w.ys, w.lv = make([]float64, w.n*cols), make([]float64, w.n*cols)
	w.top = make([]int, cols)
	w.cells = make([]wavCell, cols*rows)
}

// wavNearness is ridge j's size relative to the nearest ridge.
func wavNearness(j int) float64 { return 1 / (1 + float64(j)*wavPersp) }

// past fills sp with the spectrum from j·wavStep seconds of music ago: the
// live one for j = 0, else interpolated between the two History entries
// either side (entry i was pushed acc + i·wavStep ago).
func (w *waves) past(sp *[32]float64, live *[32]float64, j int) {
	if j == 0 {
		*sp = *live
		return
	}
	f := 1 - w.acc/wavStep // 0: exactly entry j-1, 1: exactly entry j
	a, b := w.hist.At(j-1), w.hist.At(j)
	for i := range sp {
		sp[i] = a[i]*(1-f) + b[i]*f
	}
}

func (w *waves) Step(a Audio) {
	dt := wavDT(a)
	w.kick = wavGlide(w.kick, a.Kick, dt, 0.05)
	w.acc += dt
	for w.acc >= wavStep {
		w.hist.Push(a.React)
		w.acc -= wavStep
	}
	cols, rows, n := w.cols, w.rows, w.n
	if cols == 0 || rows == 0 {
		return
	}

	// Ridge geometry: baselines from the bottom row's centre (nearest) up to
	// the horizon (farthest), spaced in perspective.
	far := wavNearness(n - 1)
	y0, yh := float64(rows)-0.5, float64(rows)*wavHoriz
	var sp [32]float64
	for j := range n {
		w.past(&sp, &a.React, j)
		near := wavNearness(j)
		depth := 1.0 // 1 nearest .. 0 farthest
		if n > 1 {
			depth = (near - far) / (1 - far)
		}
		base := yh + (y0-yh)*depth
		amp := wavAmp * float64(rows) * near
		if j == 0 {
			amp *= 1 + wavFlare*w.kick
		}
		half := float64(cols) / 2 * (0.75 + 0.25*depth)
		left := float64(cols)/2 - half
		for c := range cols {
			i := j*cols + c
			u := (float64(c) + 0.5 - left) / (2 * half)
			if u < 0 || u > 1 {
				w.ys[i], w.lv[i] = math.NaN(), 0
				continue
			}
			l := wavBand(&sp, u)
			w.ys[i], w.lv[i] = base-amp*l, l
		}
	}

	// Rasterise near to far: a column's horizon is the highest row any
	// nearer ridge reached; everything at or below it is hidden.
	for i := range w.cells {
		w.cells[i] = wavCell{}
	}
	for c := range w.top {
		w.top[c] = rows
	}
	for j := range n {
		bright := math.Pow(wavNearness(j), 0.8)
		for c := range cols {
			i := j*cols + c
			y := w.ys[i]
			if math.IsNaN(y) {
				continue
			}
			yl, yr := y, y
			if c > 0 && !math.IsNaN(w.ys[i-1]) {
				yl = w.ys[i-1]
			}
			if c < cols-1 && !math.IsNaN(w.ys[i+1]) {
				yr = w.ys[i+1]
			}
			// The line through this column runs from halfway to its left
			// neighbour to halfway to its right one. It is drawn in the row
			// holding y, plus any row whose middle that segment crosses (a
			// steep flank); a gentle slope stays one glyph per column.
			lo, hi := min(y, (y+yl)/2, (y+yr)/2), max(y, (y+yl)/2, (y+yr)/2)
			ry := int(math.Floor(y))
			rt := min(ry, int(math.Floor(lo+0.5)))
			rb := max(ry, int(math.Floor(hi-0.5)))
			// Colour walks the ramp by depth first (far = dark end, near =
			// bright end), and the band level lifts it within that: every
			// ridge gets its own shade, so the gradient reads as depth.
			v := bright * (0.05 + 0.95*w.lv[i])
			if j == 0 {
				v += wavGlow * w.kick
			}
			col := wavLUTAt(&w.lut, 0.08+0.92*min(v, 1))
			for r := max(rt, 0); r <= rb && r < w.top[c]; r++ {
				var ch rune
				switch {
				case r == ry:
					switch f := y - math.Floor(y); {
					case f < 1.0/3:
						ch = '¯'
					case f < 2.0/3:
						ch = '-'
					default:
						ch = '_'
					}
				case (yl-y)*(yr-y) > 0: // a peak or a trough
					ch = '|'
				case yr < yl: // rising to the right
					ch = '/'
				default:
					ch = '\\'
				}
				w.cells[r*cols+c] = wavCell{ch, col, true}
			}
			w.top[c] = min(w.top[c], max(rt, 0))
		}
	}
}

func (w *waves) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= w.cols || row >= w.rows {
		return 0, RGB{}, false
	}
	c := w.cells[row*w.cols+col]
	return c.ch, c.c, c.ok
}
