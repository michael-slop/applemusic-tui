package fx

// "Fire": heat-dissipation fire, ported from panefx's fire.rs, which ports
// mhearse/asciifire (asciifire.py), itself a port of Thiemo Mattig's
// JavaScript at http://maettig.com/code/javascript/asciifire.html.
//
// The algorithm:
//   - seed an OFF-SCREEN bottom row with random heat each frame
//   - every other cell becomes a centre-weighted average of the three cells
//     below it, which both spreads heat sideways and carries it upward
//   - a jittered decay keeps the fire from saturating
//   - map the result to an index in an 8-glyph ramp, dithered per cell
//
// Colours: panefx interpolated the 7 drawn shades between two endpoints of
// its green ramp (dark green -> near-white green). Here the same 7 steps run
// up the theme ramp, from just above its darkest stop to AccentHi.
//
// Timing: panefx stepped fire once per panel frame at its default 10 fps;
// here it runs on a 100 ms accumulator so amtui's 30 fps calls do not triple
// its speed.
//
// Cells: fire already scales its cooling with the panel's row count (see
// fireDecayForRows), so the taller terminal cell needs no adaptation.
//
// How it hears the music:
//   - Drive(Bass) lowers the cooling (up to 35% less), so a bass line lets
//     the tongues climb higher.
//   - Kick makes the seed row burn hotter: fewer cold gaps in the source row
//     (the cold fraction drops from 25% to ~10% on a full hit), so each beat
//     sends up a denser wave of flame.
//   - The spectrum shapes it: each column cools at a rate set by the band
//     beneath it (mirrored: bass in the middle, treble at both edges), so the
//     tongues stand tallest over whatever is loudest for this song.
//   - Paused (Playing false) or reactivity 0: exactly the original.

// fireRamp is the character ramp from the original, coldest first.
var fireRamp = [8]rune{' ', '.', ':', '*', 's', 'S', '#', '$'}

// fireSpectrumCool: how much a column's cooling follows its band. A band at
// +0.5 cools that column 60% less (taller tongues); at -0.5, 60% more.
const fireSpectrumCool = 0.6

const (
	fireTick = 0.1 // one simulation frame: panefx's default 10 fps
	// How much the glyph threshold is dithered, in ramp buckets. ~1 bucket of
	// spread is enough to shatter the flat bands without visibly softening
	// the bright core of the fire.
	fireDither = 1.0
	// How many rows at the top of the grid are faded to nothing. Deliberately
	// generous: heat must already be near zero BEFORE it reaches row 0,
	// otherwise the boundary line reappears at the top of the fade instead.
	fireTopFadeRows = 6
	// Music.
	fireBassCool = 0.35 // cooling removed by a full bass drive
	fireKickCold = 0.6  // fraction of the cold seeds a full kick heats up
)

// fireDecayForRows is the per-row cooling, tuned so flames reach a similar
// FRACTION of the panel height whatever its size. A fixed decay burns out
// after a roughly fixed number of rows: tuned on 80x25 it filled only the
// bottom sixth of a full-height terminal. Calibrated so flames reach roughly
// 60-70% of the way up, which reads as fire rather than a stripe.
func fireDecayForRows(rows int) float32 {
	if rows == 0 {
		return 0.14
	}
	// ~15 rows of visible flame at the reference size, scaled by height.
	return min(max(2.6/float32(rows), 0.02), 0.30)
}

// fireCellDither is a stable per-cell value in [0,1), hashed from the
// coordinates. It must be a function of position only: a per-frame random
// would make every cell in the sparse tail flicker independently, which looks
// like TV static rather than fire.
func fireCellDither(col, row int) float32 {
	h := uint32(col)*0x9E3779B9 ^ uint32(row)*0x85EBCA6B
	h ^= h >> 15
	h *= 0x2545F491
	h ^= h >> 13
	return float32(h>>8) / float32(1<<24)
}

// fireTopFade ramps from 0 at row 0 to 1 by fireTopFadeRows.
func fireTopFade(row, rows int) float32 {
	// On a very short panel, fading a fixed 6 rows would erase most of the
	// fire, so scale the band down for small grids.
	band := max(min(fireTopFadeRows, rows/3), 1)
	if row >= band {
		return 1
	}
	// Squared so the last row or two go properly dark rather than merely dim.
	t := float32(row) / float32(band)
	return t * t
}

type fire struct {
	cols, rows int
	// Heat per cell in [0,1], row-major, row 0 is the TOP of the screen.
	//
	// Holds rows+1 rows: the extra final row is the OFF-SCREEN seed row.
	// Mattig's original is explicit that the random source row is off-screen;
	// drawing it would show a solid wall of hot glyphs pinned to the bottom
	// edge instead of flame roots.
	cells  []float32
	dither []float32 // fireCellDither per visible cell, precomputed
	idx    []uint8   // dithered ramp index per visible cell, for Cell
	// prev is the visible heat before the latest simulation step. The sim
	// keeps panefx's 10 Hz clock; drawing heat blended from prev to cells by
	// the fraction of the next tick already elapsed gives 30 distinct images a
	// second instead of 10, without touching the simulation itself.
	prev []float32
	// spec is this frame's spectrum deviation per column (0 at rest).
	spec []float64
	rng  *Rand
	// Cooling factor per row of rise; higher = shorter flames.
	decay float32
	acc   float64

	colours [len(fireRamp)]RGB
}

func newFire() Effect {
	f := &fire{rng: NewRand(0xF19E)}
	f.Resize(0, 0)
	return f
}

func init() { Register("fire", 11, newFire) }

func (f *fire) Name() string { return "fire" }

// Resize preserves nothing: a garbage-preserving copy would look worse than a
// clean restart.
func (f *fire) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == f.cols && rows == f.rows && f.cells != nil {
		return
	}
	f.cols, f.rows = cols, rows
	f.cells = make([]float32, cols*(rows+1))
	f.idx = make([]uint8, cols*rows)
	f.prev = make([]float32, cols*rows)
	f.spec = make([]float64, cols)
	f.dither = make([]float32, cols*rows)
	for r := range rows {
		for c := range cols {
			f.dither[r*cols+c] = fireCellDither(c, r)
		}
	}
	// Must re-tune: the old decay leaves flames the wrong height.
	f.decay = fireDecayForRows(rows)
}

func (f *fire) SetPalette(p Palette) {
	// Index 0 is the blank glyph and never drawn; 1..7 climb the theme ramp,
	// as panefx interpolated them between its two green endpoints.
	for i := 1; i < len(f.colours); i++ {
		t := float64(i-1) / float64(len(f.colours)-2)
		f.colours[i] = p.At(0.15 + 0.6*t)
	}
}

func (f *fire) Step(a Audio) {
	f.acc = min(f.acc+a.DT, 1)
	var kick, bass float64
	if a.Playing {
		kick, bass = min(max(a.Kick, 0), 1), Drive(a.Bass)
	}
	if len(f.spec) == f.cols {
		a.SpectrumRow(f.spec, true)
	}
	for f.acc >= fireTick {
		f.acc -= fireTick
		if len(f.prev) == f.cols*f.rows {
			copy(f.prev, f.cells[:f.cols*f.rows])
		}
		f.advance(float32(kick), float32(bass))
	}
	f.quantise(float32(f.acc / fireTick))
}

// advance is one frame of the original.
func (f *fire) advance(kick, bass float32) {
	if f.cols == 0 || f.rows == 0 {
		return
	}
	cols, rows := f.cols, f.rows

	// Seed the OFF-SCREEN row with fresh random heat. Some cells are seeded
	// cold, which is what carves the gaps between flame tongues: a uniformly
	// hot base gives an unbroken sheet of fire.
	cold := float32(0.25) * (1 - fireKickCold*kick)
	seed := f.cells[rows*cols:]
	for c := range cols {
		v := float32(f.rng.Float())
		if v < cold {
			seed[c] = v * 0.7
		} else {
			seed[c] = 0.75 + v*0.25
		}
	}

	decay := f.decay * (1 - fireBassCool*bass)
	// Propagate upward, walking top-down so each row reads the
	// not-yet-updated row beneath it.
	//
	// The kernel is CENTRE-WEIGHTED. An even left/centre/right average is a
	// strong horizontal blur applied every frame: it smears vertical
	// structure away, leaving flat horizontal bands. Weighting the cell
	// directly below far more heavily lets heat climb in columns, and the
	// lighter side terms let those columns lean and merge like real flames.
	for row := range rows {
		below := f.cells[(row+1)*cols:]
		// Extra cooling over the topmost rows so the fire fades out instead
		// of being sliced off at the grid boundary: row 0 has no neighbour
		// above it, so whatever heat reaches it would render as a hard flat
		// line. This is the mirror of the off-screen seed row.
		fade := fireTopFade(row, rows)
		out := f.cells[row*cols:]
		for col := range cols {
			left, right := col-1, col+1
			if col == 0 {
				left = cols - 1
			}
			if right == cols {
				right = 0
			}
			// Weights 6:1:1, dominated by straight-up rise.
			avg := (below[col]*6 + below[left] + below[right]) / 8
			// Random per-cell cooling. Uniform decay would let the small
			// sideways term equalise each row over time; the jitter keeps
			// neighbouring columns at genuinely different heights.
			d := decay
			if len(f.spec) == cols {
				// Spectrum shaping: a column over a band above its norm cools
				// slower, so its tongues climb higher (mirrored, bass centre).
				d *= 1 - fireSpectrumCool*float32(f.spec[col])*2
			}
			jitter := 1 - d*(0.2+float32(f.rng.Float())*1.6)
			out[col] = min(max(avg*jitter*fade, 0), 1)
		}
	}
}

// quantise maps heat to a ramp index for every visible cell.
//
// The mapping is DITHERED, and it has to be. In the sparse tail of the fire
// the heat field is smooth and nearly flat, so a plain floor puts a whole
// horizontal swathe of cells in the same bucket at once: long unbroken runs
// of '.' that read as horizontal lines through the dying flames. A stable
// per-cell offset before flooring breaks the tie, so cells either side of a
// threshold scatter instead of flipping in unison.
func (f *fire) quantise(t float32) {
	n := float32(len(fireRamp))
	blend := len(f.prev) == len(f.idx)
	for i := range f.idx {
		h := f.cells[i]
		if blend {
			h = f.prev[i] + (h-f.prev[i])*t
		}
		h = min(max(h, 0), 1)
		// Dither by up to one bucket, centred so mean brightness is unchanged.
		d := (f.dither[i] - 0.5) * fireDither
		f.idx[i] = uint8(min(int(max(h*n+d, 0)), len(fireRamp)-1))
	}
}

// glyphIndex is the dithered ramp index at a visible cell (tests).
func (f *fire) glyphIndex(col, row int) int { return int(f.idx[row*f.cols+col]) }

func (f *fire) heat(col, row int) float32 { return f.cells[row*f.cols+col] }

func (f *fire) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= f.cols || row >= f.rows {
		return 0, RGB{}, false
	}
	i := f.idx[row*f.cols+col]
	if i == 0 {
		return 0, RGB{}, false
	}
	return fireRamp[i], f.colours[i], true
}
