package fx

// "Rain": ASCII rain, ported from panefx's rain.rs, which reproduces Michael's
// own createRain (the michaelslop.org boot screen).
//
// Kept from the original:
//   - the exact CP437 character set, block glyphs included. It is ASCII-not-
//     katakana on purpose: "ASCII rain" ought to be ascii.
//   - one head per column, each starting at a NEGATIVE row so columns are
//     staggered rather than falling in a sheet
//   - a pale head with a trail-coloured cell immediately above it
//   - respawn above the top once the head passes the bottom, at a random
//     negative offset
//   - the stepped ~55 ms tick. The original's comment is explicit that a CRT
//     reads better stepped than smooth, so this does NOT run at amtui's
//     30 fps; an accumulator steps it every 55 ms whatever the call rate.
//
// The original is a canvas that paints a translucent black rect each frame so
// old glyphs fade. A character grid is redrawn from scratch every frame, so
// the trail is modelled explicitly as a per-cell brightness that decays. Same
// look, different mechanism.
//
// Colours: the pale head (#c8ffc8) is the theme's AccentHi and the trail
// green (#22cc44) is the theme's Accent, dimmed toward the background as it
// decays, exactly as panefx dimmed it toward black.
//
// Cells: the rain was designed for 16px square-ish cells; on a 1:2 terminal
// cell a drop covers twice the height per tick, so it falls a little faster
// on screen. The stepped rhythm is the point and is kept as is.
//
// How it hears the music:
//   - Drive(Treble) quickens the tick: hi-hats and cymbals speed the rain up
//     to ~1.6x (55 ms down to 35 ms per step), easing back when they stop.
//   - Kick flashes the fresh trail toward the head colour for a moment, so
//     every beat lights the rain without strobing the whole panel.
//   - Paused (Playing false) or reactivity 0: exactly the original.

// rainChars is the site's exact set, block glyphs included.
const rainChars = "01ABCDEFGHIJKLMNOPQRSTUVWXYZ#$%&@*+=<>?!;:░▒▓"

const (
	// rainTick is seconds per rain step, from the original's TICK = 55:
	// "chunky on purpose - a CRT reads better stepped than smooth - but 66 was
	// far enough into slideshow territory to look broken rather than retro.
	// 55 is ~18fps." Do not smooth it out.
	rainTick = 0.055
	// rainDecay is how much a trail cell dims per step. Chosen so a trail
	// runs ~10 cells, which matches the canvas version's 0.22-alpha fade.
	rainDecay = 26
	// rainFastest is the share of the tick a full treble drive removes.
	rainFastest = 0.36
	// rainFlash is how far a full kick pulls the trail toward the head.
	rainFlash = 0.35
	// rainLevels is how many trail brightnesses get their own colour.
	rainLevels = 256
)

var rainRunes = []rune(rainChars)

type rain struct {
	cols, rows int
	// Head row per column. Negative means the head has not entered the top
	// of the panel yet, which is what staggers the columns.
	drops []int
	// Glyph currently shown in each cell.
	glyph []rune
	// Brightness 0..255 per cell; 255 is a fresh head. Decays each step.
	bright []uint8
	rng    *Rand
	acc    float64
	// speed eases toward the treble drive so the tempo never lurches.
	speed float64
	flash float64
	steps uint64 // rain steps taken, for tests

	pal         Palette
	head, trail RGB
	// shade[b] is the trail colour at brightness b (with the current flash).
	shade     [rainLevels]RGB
	shadeKey  float64
	shadeInit bool
}

func newRain() Effect {
	r := &rain{rng: NewRand(0x5A1D)}
	r.Resize(0, 0)
	return r
}

func init() { Register("rain", 20, newRain) }

func (r *rain) Name() string { return "rain" }

func (r *rain) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == r.cols && rows == r.rows && r.drops != nil {
		return
	}
	r.cols, r.rows = cols, rows
	r.glyph = make([]rune, cols*rows)
	r.bright = make([]uint8, cols*rows)
	r.resetDrops()
}

// resetDrops is drops = Array.from({length: cols}, () => -floor(random()*rows*1.5)).
func (r *rain) resetDrops() {
	span := max(int(float64(r.rows)*1.5), 1)
	r.drops = make([]int, r.cols)
	for c := range r.drops {
		r.drops[c] = -r.rng.Intn(span)
	}
}

func (r *rain) SetPalette(p Palette) {
	r.pal = p
	r.head, r.trail = p.AccentHi, p.Accent
	r.shadeInit = false
	r.buildShade()
}

// buildShade precomputes the trail colour per brightness, so Cell is a lookup.
// Rebuilt only when the palette or the (quantised) kick flash changes.
func (r *rain) buildShade() {
	key := float64(int(r.flash*20)) / 20
	if r.shadeInit && key == r.shadeKey {
		return
	}
	r.shadeKey, r.shadeInit = key, true
	trail := Lerp(r.trail, r.head, key*rainFlash)
	for b := range r.shade {
		// Fresh cells are the trail colour; decayed ones fade to the
		// background, as panefx faded them to its black background.
		r.shade[b] = Lerp(r.pal.Background, trail, float64(b)/255)
	}
}

func (r *rain) Step(a Audio) {
	var treble, kick float64
	if a.Playing {
		treble, kick = Drive(a.Treble), min(max(a.Kick, 0), 1)
	}
	// Ease the tempo over ~0.25 s; a flash follows the kick directly.
	k := min(a.DT*4, 1)
	r.speed += (treble - r.speed) * k
	r.flash = kick
	tick := rainTick * (1 - rainFastest*r.speed)

	r.acc = min(r.acc+a.DT, 1)
	for r.acc >= tick {
		r.acc -= tick
		r.advance()
		r.steps++
	}
	r.buildShade()
}

func (r *rain) randomGlyph() rune { return rainRunes[r.rng.Intn(len(rainRunes))] }

// advance is one rain step: the body of the original's tick.
func (r *rain) advance() {
	if r.cols == 0 || r.rows == 0 {
		return
	}
	// Fade everything. Stands in for the canvas' translucent black rect.
	for i, b := range r.bright {
		if b > rainDecay {
			r.bright[i] = b - rainDecay
		} else {
			r.bright[i] = 0
		}
	}
	for c := range r.cols {
		row := r.drops[c]
		if row >= 0 && row < r.rows {
			i := row*r.cols + c
			r.glyph[i] = r.randomGlyph()
			r.bright[i] = 255
			// if (r > 0): the trail cell one above the pale head.
			if row > 0 {
				r.glyph[i-r.cols] = r.randomGlyph()
				// Just under the head threshold so it renders as trail
				// rather than a second head.
				r.bright[i-r.cols] = 200
			}
		}
		// drops[c] = r > rows + random()*30 ? -((random()*20)|0) : r + 1
		limit := float64(r.rows) + r.rng.Float()*30
		if float64(row) > limit {
			r.drops[c] = -r.rng.Intn(20)
		} else {
			r.drops[c] = row + 1
		}
	}
}

func (r *rain) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= r.cols || row >= r.rows {
		return 0, RGB{}, false
	}
	i := row*r.cols + col
	b := r.bright[i]
	if b == 0 || r.glyph[i] == 0 {
		return 0, RGB{}, false
	}
	// A fresh head is pale; everything behind it is trail, dimmed by how far
	// it has decayed.
	if b >= 250 {
		return r.glyph[i], r.head, true
	}
	return r.glyph[i], r.shade[b], true
}
