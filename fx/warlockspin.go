package fx

// warlockspin: the warlock, rocking. Ported from panefx src/warlockspin.rs.
//
// A hooded skeleton in sunglasses holding a wand (warlock_art.go). Same
// rotation maths as skullspin and as michaelslop.org's own spinner: the screen
// column is inverse-mapped back through the cosine, rounded rather than
// interpolated so the edges stay crisp at these sizes.
//
// What is deliberately different from skullspin: the skull is a symmetric
// front-facing object, so it can turn through edge-on without ever looking
// wrong. The warlock faces RIGHT. A side-on figure rotated past edge-on shows
// its own back, and a wand that sweeps behind the body reads as a glitch
// rather than as a turn. So this does not spin through 360 degrees. It ROCKS:
// the cosine is driven over a limited arc, so the figure turns toward the
// viewer and back without ever presenting a face the sprite does not have.
//
// Four colours rather than the skull's two, because the sprite has four parts
// worth telling apart while it moves: the skull, the sunglasses (the joke of
// the piece), the robe, and the wand. From the theme: bone is Text, the
// sunglasses Accent, the robe a dark AccentLo, the wand Faint, and the outline
// the darkest ramp tone, falling off toward Background.
//
// How it hears the music:
//   - Tempo follows the bass: the rock's own clock runs up to 1.8x faster on a
//     driving bass line (Drive(Bass)), so it sways with the groove.
//   - Amplitude follows the bass too: sway goes from panefx's 0.85 toward 1.0
//     (edge-on) with Drive(Bass). Never past edge-on, so it still never shows
//     its back.
//   - The sunglasses glint on a Kick: a bright highlight sweeps across the
//     lenses as the kick pulse decays, and the lenses brighten toward AccentHi.
//   - At Reactivity 0 the rock clock is wall time, sway is 0.85 and there is
//     no glint: the panefx original. Time keeps running while paused.

import "math"

const (
	warlockCols = 21
	warlockRows = 29
	// Two glyph columns per source cell. A character cell is roughly twice as
	// tall as it is wide, so 1:1 would render the warlock stretched into a
	// lamppost.
	warlockXScale = 2
	warlockRate   = 0.25 // rock cycles per second (panefx spin=250)
	warlockSway   = 0.85 // how far it turns, of a quarter turn (sway=850)
)

// warlockRock is the rock angle at time t, as a cosine: what the inverse
// column map needs. sway is how far the figure turns, 0..1 of a quarter turn:
// at 0 it faces straight ahead and never moves; at 1 it reaches edge-on at the
// extremes. It never goes PAST edge-on.
func warlockRock(t, rate, sway float64) float64 {
	sway = min(max(sway, 0), 1)
	// cos walks between 1.0 (facing the viewer) and 1.0 - sway. With sway = 1
	// the far end is 0.0, which is edge-on: the sprite's own profile, and as
	// far as a side-on figure can honestly turn.
	phase := math.Sin(t * rate * 2 * math.Pi)
	return 1 - sway*(0.5-0.5*phase)
}

// The sunglasses' extent in the sprite, for the glint sweep.
var warlockLensLo, warlockLensHi = func() (lo, hi int) {
	lo, hi = warlockCols, -1
	for _, row := range warlockArt {
		for x := 0; x < len(row); x++ {
			if row[x] == '=' {
				lo, hi = min(lo, x), max(hi, x)
			}
		}
	}
	return
}()

type warlockSpin struct {
	cols, rows int
	// rt is the rock's own clock: wall time, sped up by the bass.
	rt   float64
	sway float64
	kick float64

	pal  Palette
	grid []spinCell
}

func newWarlockSpin() Effect { return &warlockSpin{sway: warlockSway} }

func init() { Register("warlockspin", 51, newWarlockSpin) }

func (w *warlockSpin) Name() string { return "warlockspin" }

func (w *warlockSpin) Resize(cols, rows int) {
	w.cols, w.rows = max(cols, 0), max(rows, 0)
	w.grid = make([]spinCell, w.cols*w.rows)
	w.raster()
}

func (w *warlockSpin) SetPalette(p Palette) {
	w.pal = p
	w.raster()
}

func (w *warlockSpin) Step(a Audio) {
	dt := min(max(a.DT, 0), 0.25)
	bass := Drive(a.Bass)
	w.rt += dt * (1 + 0.8*bass)
	// Ease the amplitude rather than following the bass frame by frame, so
	// the sway breathes instead of twitching.
	target := warlockSway + (1-warlockSway)*bass
	w.sway += (target - w.sway) * min(1, dt*4)
	w.kick = a.Kick
	w.raster()
}

func (w *warlockSpin) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= w.cols || row >= w.rows {
		return 0, RGB{}, false
	}
	g := w.grid[row*w.cols+col]
	return g.ch, g.c, g.ch != 0
}

// layout is the origin and zoom: where the figure sits and how big it is.
func (w *warlockSpin) layout() (ox, oy, z float64) {
	byW := float64(w.cols) / float64(warlockCols*warlockXScale)
	// No jump, so no headroom reservation, unlike skullspin. A small margin
	// only, so the outline is not flush against the panel edge.
	byH := float64(w.rows) / (warlockRows * 1.08)
	z = min(byW, byH)
	wd := float64(warlockCols*warlockXScale) * z
	h := warlockRows * z
	return (float64(w.cols) - wd) / 2, max(float64(w.rows)-h, 0) / 2, z
}

func (w *warlockSpin) raster() {
	for i := range w.grid {
		w.grid[i] = spinCell{}
	}
	ox, oy, z := w.layout()
	if z <= 0 {
		return
	}
	c := spinClampEdgeOn(warlockRock(w.rt, warlockRate, w.sway))

	p := w.pal
	bone := p.Text
	shade := Lerp(p.Accent, p.AccentHi, 0.5*w.kick)
	glint := p.AccentHi
	robe := Lerp(p.Ramp[0], p.AccentLo, 0.6)
	wand := p.Faint
	var outline [3]RGB
	for i, b := range []byte{'*', '+', '.'} {
		_, k, _ := spinOutlineShade(b)
		outline[i] = Lerp(p.Background, p.Ramp[0], k)
	}
	// The glint: a two-column highlight sweeping left to right across the
	// lenses as the kick decays from 1 to 0.
	glintAt := -100.0
	if w.kick > 0.05 {
		glintAt = float64(warlockLensLo) + (1-w.kick)*float64(warlockLensHi-warlockLensLo)
	}

	cx := float64(warlockCols-1) / 2
	for row := 0; row < w.rows; row++ {
		fy := (float64(row) - oy) / z
		if fy < 0 || fy >= warlockRows {
			continue
		}
		sy := int(fy)
		for col := 0; col < w.cols; col++ {
			// Panel cell -> sprite cell, undoing the layout and the rotation.
			fx := (float64(col) - ox) / z
			u := int(math.Round((fx/warlockXScale-cx)/c + cx))
			if u < 0 || u >= warlockCols {
				continue
			}
			var out spinCell
			// Every part is FLAT: one glyph, one colour, at every angle.
			// skullspin learned this the hard way: shading by the lighting
			// angle made it look like it changed colour as it turned.
			switch b := warlockArt[sy][u]; b {
			case ' ':
				continue
			case '#':
				out = spinCell{'#', bone}
			case '=':
				out = spinCell{'=', shade}
				if math.Abs(float64(u)-glintAt) < 1 {
					out.c = Lerp(shade, glint, w.kick)
				}
			case '|':
				out = spinCell{'|', wand}
			case '%':
				out = spinCell{'%', robe}
			default:
				// The outline, graded over three cells: a fixed halo, not a
				// light that moves with the turn.
				g, _, ok := spinOutlineShade(b)
				if !ok {
					continue
				}
				out = spinCell{g, outline[spinOutlineIndex(b)]}
			}
			w.grid[row*w.cols+col] = out
		}
	}
}
