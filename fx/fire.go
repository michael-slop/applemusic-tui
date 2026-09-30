package fx

import "math"

// "Fire": the burning waterfall -- the song's recent spectrum history, rising
// like heat off a fire.
//
// The columns are the frequency axis, NOT mirrored: bass on the left, treble
// on the right, at full detail. The bottom row is the spectrum right now;
// each row above it is an older spectrum (a History, one spectrum pushed per
// fireStep seconds of music time), so what the song did a moment ago drifts
// upward and cools. The heat at (col, row) is
//
//	BandAt(spectrum aged rows-from-bottom, col) x fade(age)
//
// and picks both the glyph (panefx's asciifire ramp " .:*sS#$") and the
// colour (up the theme ramp, so the colour controller recolours it). A band
// that is up now is a hot column at the base; as it rises it cools into a
// tongue of flame. A steady spectrum fills every history row alike, so the
// picture holds still: the only motion is the song's own history scrolling
// upward (declared as FreeMotion "scroll"), and that scroll runs on Audio.DT,
// which amtui already paces to the music (4% speed in silence).
//
// Cells are ~1:2, so a row of rise covers as much distance as two columns: at
// 20 rows a second the fire climbs briskly without smearing.
//
// How it hears the music:
//   - Each column is its band (Audio.BandAt(col, not mirrored)); every row is
//     that band at an earlier moment, fading with age toward the top.
//   - Kick is a global pulse: the whole fire flashes hotter while the kick
//     decays.
//   - Silence: the history fills with zeros and the fire goes dark, except a
//     faint resting ember line along the bottom row. Nothing moves.

// fireRamp is the asciifire character ramp, coldest first.
var fireRamp = [8]rune{' ', '.', ':', '*', 's', 'S', '#', '$'}

const (
	fireStep     = 0.05 // seconds of music time per history row (the scroll speed)
	fireGain     = 1.35 // a band at its normal level (0.5) burns ~2/3 hot at the base
	fireFadePow  = 1.3  // how quickly heat cools with age (1 = linear to the top)
	fireKickHeat = 0.35 // a full kick heats everything by this fraction
	fireEmber    = 0.08 // the resting ember line's heat: a dim '.' along the bottom
	fireColours  = 64   // colour table resolution
)

type fire struct {
	cols, rows int
	hist       *History
	acc        float64
	heat       []float64 // per cell before the kick, row-major, row 0 = top
	fade       []float64 // per age (rows from the bottom)
	kick       float64
	colours    [fireColours]RGB
}

func newFire() Effect {
	f := &fire{}
	f.Resize(0, 0)
	return f
}

func init() { Register("fire", 11, newFire) }

func (f *fire) Name() string { return "fire" }

// FreeMotion: the history scrolls upward, at the music's pace.
func (f *fire) FreeMotion() string { return "scroll" }

func (f *fire) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == f.cols && rows == f.rows && f.hist != nil {
		return
	}
	f.cols, f.rows = cols, rows
	f.hist = NewHistory(max(rows, 1))
	f.acc = 0
	f.heat = make([]float64, cols*rows)
	f.fade = make([]float64, rows)
	for age := range rows {
		f.fade[age] = math.Pow(1-float64(age)/float64(rows), fireFadePow)
	}
	f.fill([32]float64{})
}

func (f *fire) SetPalette(p Palette) {
	for i := range f.colours {
		f.colours[i] = p.At(0.1 + 0.9*float64(i)/float64(len(f.colours)-1))
	}
}

func (f *fire) Step(a Audio) {
	f.kick = min(max(a.Kick, 0), 1)
	// One spectrum per fireStep of music time; after a stall, never push
	// more than the history can hold.
	f.acc += max(a.DT, 0)
	for n := 0; f.acc >= fireStep; n++ {
		f.acc -= fireStep
		if n < f.hist.Depth() {
			f.hist.Push(a.React)
		}
	}
	f.fill(a.React)
}

// fill recomputes every cell's heat: the bottom row from the live spectrum,
// each row above it from the history, faded with age.
func (f *fire) fill(now [32]float64) {
	for age := range f.rows {
		src := Audio{React: now}
		if age > 0 {
			src.React = f.hist.At(age - 1)
		}
		row := (f.rows - 1 - age) * f.cols
		for c := range f.cols {
			v := src.BandAt((float64(c)+0.5)/float64(f.cols), false) * fireGain * f.fade[age]
			if age == 0 {
				v = max(v, fireEmber)
			}
			f.heat[row+c] = v
		}
	}
}

func (f *fire) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= f.cols || row >= f.rows {
		return 0, RGB{}, false
	}
	h := min(max(f.heat[row*f.cols+col]*(1+fireKickHeat*f.kick), 0), 1)
	// Rounded, not ceiled: the faint tail of a cooled row goes dark instead of
	// dusting the whole panel with dots.
	idx := min(int(h*float64(len(fireRamp)-1)+0.5), len(fireRamp)-1)
	if idx <= 0 {
		return 0, RGB{}, false
	}
	return fireRamp[idx], f.colours[int(h*float64(len(f.colours)-1)+0.5)], true
}
