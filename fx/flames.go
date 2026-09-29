package fx

import "math"

// "Flames": a faithful port of panefx's flames.rs, itself a port of
// msimpson's gist (https://gist.github.com/msimpson/1096950).
//
// This is a DIFFERENT algorithm from fire.go, not a retuning of it, and the
// differences are the whole point:
//
//   - Heat is an INTEGER 0..65 and indexes the glyph table directly
//     (char[min(b[i], 9)]). There is no float-to-bucket quantisation step,
//     which is exactly where fire produced horizontal banding.
//   - Seeding is SPARSE: only width/9 randomly chosen cells per frame are set
//     hot, rather than the whole bottom row. That is what gives discrete
//     rising sources instead of a solid sheet of flame.
//   - The kernel is asymmetric: (self + right + below + below_right) / 4, with
//     integer division doing all the cooling. There is no separate decay term.
//   - The update is IN-PLACE over a flat array, so each cell reads
//     already-updated neighbours. This is load-bearing: a double-buffered
//     version of the same kernel looks different.
//
// The original's colours are four curses pairs keyed to value thresholds
// (>15, >9, >4, else), not one colour per glyph. panefx drew them as the
// necronomicon's pale / green / teal / purple; here they are the theme's
// AccentHi / Accent / AccentLo and a dim blend of the darkest ramp stop with
// Dim, so the colour controller recolours the fire.
//
// Timing: panefx stepped flames once per panel frame at its default 10 fps.
// amtui steps at 30 fps, so the simulation runs on a 100 ms accumulator.
//
// Cells: panefx drew on the terminal's own 10x15 text grid, close enough to a
// terminal cell's 1:2 that the kernel needs no adaptation. The gist's seed
// value of 65 reaches ~20 rows whatever the panel height; on amtui's shorter
// visualizer panel that is most of the panel, which is the look.
//
// How it hears the music:
//   - Kick (the beat pulse) raises the seed value, i.e. the flame height: each
//     bass hit throws the fire up by up to +35, then it settles back to 65.
//   - Drive(Bass) raises the seeding density: a busy bass line lights up to
//     twice as many sources along the bottom row.
//   - Paused (Playing false) or reactivity 0: exactly the gist.

// flamesChars is the gist's ten-character ramp, coldest first.
var flamesChars = [10]rune{' ', '.', ':', '^', '*', 'x', 's', 'S', '#', '$'}

const (
	// flamesSeedValue is written into a seeded cell; 65 is straight from the
	// gist. It is effectively the FLAME HEIGHT control: the kernel's /4
	// integer division cools aggressively, so the fire reaches only ~20 rows.
	flamesSeedValue = 65
	flamesDensity   = 9 // 1 in N bottom cells seeded per frame (gist: width/9)
	// Colour thresholds from the original's color=(4 if b[i]>15 else ...).
	flamesTHot  = 15
	flamesTWarm = 9
	flamesTCool = 4
	// flamesTick is one simulation frame: panefx's default 10 fps.
	flamesTick = 0.1
	// Music: how far a full kick raises the seed value, and how much a full
	// bass drive multiplies the number of sources.
	flamesKickLift  = 35
	flamesBassDense = 1.0
)

type flames struct {
	width, height int
	// Flat heat array. The gist allocates size + width + 1 so the kernel can
	// read b[i+width+1] on the last row without bounds-checking; we keep that
	// same slack for the same reason.
	b   []int32
	rng *Rand
	acc float64

	// The breath: how far the flame height swings either side of the seed
	// value, and the seconds for one full breath. osc 0 (the default)
	// disables it: the steady fire is the look panefx shipped with. Kept
	// because it is part of the original's behaviour; amtui's music plays the
	// same role live.
	osc     int
	oscSecs float64
	// Frames elapsed, for the breath's phase. Counted rather than read off a
	// clock so the sweep is deterministic under test.
	tick uint64

	cHot, cWarm, cCool, cDim RGB
}

func newFlames() Effect {
	f := &flames{rng: NewRand(0xF1A3E5), oscSecs: 20}
	f.Resize(0, 0)
	return f
}

func init() { Register("flames", 10, newFlames) }

func (f *flames) Name() string { return "flames" }

func (f *flames) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == f.width && rows == f.height && f.b != nil {
		return
	}
	f.width, f.height = cols, rows
	f.b = make([]int32, cols*rows+cols+1)
}

func (f *flames) SetPalette(p Palette) {
	f.cHot = p.AccentHi
	f.cWarm = p.Accent
	f.cCool = p.AccentLo
	// panefx's dim band is a deep purple under the teal; the theme's Dim
	// pulled most of the way down to the darkest ramp stop plays that part.
	f.cDim = Lerp(p.Ramp[0], p.Dim, 0.45)
}

// seedNow is the seed value for this frame, breathing if osc is set. A sine,
// not a triangle: the fire should pause at the top and bottom of the breath
// rather than reverse sharply. Clamped to 1..255 so a large osc flattens
// against the ends instead of wrapping from tall to nothing in one frame.
func (f *flames) seedNow() int32 {
	if f.osc == 0 || f.oscSecs <= 0 {
		return flamesSeedValue
	}
	period := max(f.oscSecs/flamesTick, 1)
	phase := math.Mod(float64(f.tick), period) / period
	s := math.Sin(phase * 2 * math.Pi)
	return int32(min(max(flamesSeedValue+int(math.Round(s*float64(f.osc))), 1), 255))
}

func (f *flames) Step(a Audio) {
	// Never try to catch up more than a second after a stall.
	f.acc = min(f.acc+a.DT, 1)
	var kick, bass float64
	if a.Playing {
		kick, bass = min(max(a.Kick, 0), 1), Drive(a.Bass)
	}
	for f.acc >= flamesTick {
		f.acc -= flamesTick
		f.advance(kick, bass)
	}
}

// advance is one frame of the gist.
func (f *flames) advance(kick, bass float64) {
	if f.width == 0 || f.height == 0 {
		return
	}
	w := f.width
	size := w * f.height

	// for i in range(int(width/9)): b[int(random.random()*width + width*(height-1))] = 65
	// Sparse seeding along the bottom row, only ~1 cell in 9.
	seeds := w / flamesDensity
	if bass > 0 {
		seeds = int(float64(seeds) * (1 + flamesBassDense*bass))
	}
	base := w * (f.height - 1)
	// One value for the whole row: seeding a single frame at two different
	// heights would fray the base of the fire rather than raise it.
	seed := f.seedNow()
	if kick > 0 {
		seed = min(seed+int32(math.Round(flamesKickLift*kick)), 255)
	}
	for range seeds {
		off := min(int(f.rng.Float()*float64(w)), w-1)
		f.b[base+off] = seed
	}
	// Advance the breath AFTER seeding, so frame 0 uses the configured height.
	f.tick++

	// b[i] = int((b[i] + b[i+1] + b[i+width] + b[i+width+1]) / 4)
	// In-place and forward-walking, so later cells see updated earlier ones.
	// Integer division is the only cooling in the algorithm.
	b := f.b
	for i := 0; i < size; i++ {
		b[i] = (b[i] + b[i+1] + b[i+w] + b[i+w+1]) / 4
	}
}

func (f *flames) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= f.width || row >= f.height {
		return 0, RGB{}, false
	}
	v := f.b[row*f.width+col]
	// char[(9 if b[i]>9 else b[i])]
	idx := min(max(v, 0), 9)
	if idx == 0 {
		// Blank cells are skipped, not painted.
		return 0, RGB{}, false
	}
	var c RGB
	switch {
	case v > flamesTHot:
		c = f.cHot
	case v > flamesTWarm:
		c = f.cWarm
	case v > flamesTCool:
		c = f.cCool
	default:
		c = f.cDim
	}
	return flamesChars[idx], c, true
}
