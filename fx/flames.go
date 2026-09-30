package fx

// "Flames": the spectrum as fire.
//
// The columns are the frequency axis, mirrored: bass burns in the centre and
// the treble at both edges, so the picture is symmetric left to right. Each
// column is one flame whose HEIGHT is the band beneath it -- a spectrum bar
// drawn as fire:
//
//	height = rest + (top - rest) * level
//
// the same shape as the torus's tube radius 0.55+0.85*band. Inside a column
// the glyph runs the gist's fire ramp (msimpson,
// https://gist.github.com/msimpson/1096950: " .:^*xsS#$") from hot at the
// base to cool at the tip, measured along THAT column's own height, so a
// short flame and a tall one are each a whole flame. The colour climbs the
// theme ramp the same way (p.At), so the colour controller recolours it.
//
// Texture: the tip of each flame is frayed by a noise pattern that is FIXED in
// space (a constant hash of column and row, mirrored like the bands), so the
// top edge reads as tongues of flame rather than bars. It never moves on its
// own: a steady spectrum draws a steady picture. The only motion is the music.
//
// Cells are ~1:2 (twice as tall as wide); heights are in rows, so each column
// is a narrow flame and nothing needs correcting for the aspect.
//
// How it hears the music:
//   - Band level (Audio.BandAt(col, mirrored)) is each column's flame height;
//     a full band nearly reaches the top of the panel. Heights ease lightly
//     toward the band (half the gap per frame) so the fire breathes rather
//     than jitters; the bands are already smoothed upstream.
//   - A louder band also burns hotter: brighter glyphs and colours at its base.
//   - Kick is a global pulse: every flame jumps a little taller and the whole
//     fire flashes hotter for the ~0.3 s the kick takes to decay.
//   - Silence: React decays to 0, every flame sinks to the resting ember line
//     along the bottom (about a tenth of the panel), and nothing moves.

// flamesChars is the gist's ten-character ramp, coldest first.
var flamesChars = [10]rune{' ', '.', ':', '^', '*', 'x', 's', 'S', '#', '$'}

const (
	flamesRest     = 0.10 // resting ember line, fraction of the panel height
	flamesTop      = 0.95 // a full band's flame, fraction of the panel height
	flamesEase     = 0.5  // fraction of the gap to the band closed per frame
	flamesKickLift = 0.15 // a full kick makes every flame this much taller
	flamesKickHeat = 0.30 // ...and this much hotter
	flamesFray     = 0.22 // tip fraying, +- fraction of the flame's height
	flamesFrayRows = 0.6  // ...plus this many rows, so short flames fray too
	flamesColours  = 64   // colour table resolution
)

type flames struct {
	width, height int
	hgt           []float64 // eased flame height per column, in rows
	lvl           []float64 // eased band level per column, 0..1
	noise         []float64 // fixed fray per cell, -0.5..0.5, index y*width+col, y = rows from bottom
	kick          float64
	colours       [flamesColours]RGB
}

func newFlames() Effect {
	f := &flames{}
	f.Resize(0, 0)
	return f
}

func init() { Register("flames", 10, newFlames) }

func (f *flames) Name() string { return "flames" }

// flamesHash is a constant-seeded hash of a cell, in [0,1).
func flamesHash(x, y int) float64 {
	h := uint32(x)*0x9E3779B9 ^ uint32(y)*0x85EBCA6B ^ 0xF1A3E5
	h ^= h >> 15
	h *= 0x2545F491
	h ^= h >> 13
	h *= 0x68E31DA5
	h ^= h >> 16
	return float64(h>>8) / float64(1<<24)
}

// rest is the resting ember line's height in rows.
func (f *flames) rest() float64 { return max(1, flamesRest*float64(f.height)) }

func (f *flames) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == f.width && rows == f.height && f.hgt != nil {
		return
	}
	f.width, f.height = cols, rows
	f.hgt = make([]float64, cols)
	f.lvl = make([]float64, cols)
	for c := range f.hgt {
		f.hgt[c] = f.rest()
	}
	f.noise = make([]float64, cols*rows)
	for y := range rows {
		for c := range cols {
			// Mirrored like the bands, so the picture stays symmetric.
			f.noise[y*cols+c] = flamesHash(min(c, cols-1-c), y) - 0.5
		}
	}
}

func (f *flames) SetPalette(p Palette) {
	for i := range f.colours {
		f.colours[i] = p.At(0.1 + 0.9*float64(i)/float64(len(f.colours)-1))
	}
}

func (f *flames) Step(a Audio) {
	f.kick = min(max(a.Kick, 0), 1)
	if f.width == 0 {
		return
	}
	rest := f.rest()
	top := max(rest, flamesTop*float64(f.height))
	for c := range f.hgt {
		lvl := min(max(a.BandAt((float64(c)+0.5)/float64(f.width), true), 0), 1)
		target := (rest + (top-rest)*lvl) * (1 + flamesKickLift*f.kick)
		f.hgt[c] = flamesEaseTo(f.hgt[c], target)
		f.lvl[c] = flamesEaseTo(f.lvl[c], lvl)
	}
}

// flamesEaseTo closes part of the gap per frame, snapping once it is closed
// so a steady spectrum settles on exactly steady values.
func flamesEaseTo(v, target float64) float64 {
	v += (target - v) * flamesEase
	if d := target - v; d < 1e-6 && d > -1e-6 {
		return target
	}
	return v
}

func (f *flames) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= f.width || row >= f.height {
		return 0, RGB{}, false
	}
	y := f.height - 1 - row // rows from the bottom
	h := f.hgt[col]
	h += f.noise[y*f.width+col] * (flamesFray*h + flamesFrayRows)
	frac := (float64(y) + 0.5) / max(h, 0.01)
	if frac >= 1 {
		return 0, RGB{}, false
	}
	heat := (1 - frac) * (0.6 + 0.4*f.lvl[col]) * (1 + flamesKickHeat*f.kick)
	heat = min(max(heat, 0), 1)
	idx := min(max(int(heat*9+0.999), 1), 9)
	return flamesChars[idx], f.colours[int(heat*float64(len(f.colours)-1)+0.5)], true
}
