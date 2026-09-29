package fx

// Classic demoscene plasma, ported from panefx's plasma.rs.
//
// The oldest trick in the book: sum a few sine waves of the cell's position
// and the clock, map the total through a ramp. No state, no buffers: every
// cell is a pure function of (x, y, t), which is why it is nearly free and
// why it resizes instantly.
//
// The version here is the standard four-term field:
//
//	v = sin(x·f)
//	  + sin(y·f/2 + t)
//	  + sin((x + y)·f/2 + t)
//	  + sin(√(x² + y²)·f + t)
//
// The last term is what stops it reading as a plaid: the first three are all
// axis-aligned, and the radial one breaks that up with a circular front moving
// through them.
//
// Cell aspect: a terminal cell is about twice as tall as it is wide, so the y
// coordinate is doubled before it enters the field (panefx made the same 2:1
// correction for its chunky cells, so the constant carries over unchanged).
// Without it every circular feature comes out as a vertical ellipse.
//
// Colour: panefx mixed a fixed low colour to a fixed high colour by the field
// value. Here the same field value walks the theme's dark -> bright ramp, so
// the plasma's hues cycle through the theme as the field moves.
//
// How it hears the music: the mid bands (Drive(a.Mid), smoothed) speed up the
// phase by up to 2.2x and raise the contrast by up to 50%, so busy passages
// churn harder and sharper; a kick lowers the dark cut-off (more of the field
// lights up) and lifts every lit cell a few steps up the colour ramp, a
// brightness pulse that fades with the kick. Paused or at reactivity 0 all of
// that rests at zero and the field drifts exactly like the original.

import "math"

// plasmaRamp is the default glyph ramp, sparsest first.
var plasmaRamp = []rune(" .:-=+*#%@")

// ptsColourSteps is how many distinct colours the theme ramp is sampled into.
// panefx quantised colours because its renderer paid one draw call per colour
// per row; amtui's Render likewise emits one escape per colour run, so the
// same budget applies. Shared by plasma, tunnel, starfield and spin3d.
const ptsColourSteps = 16

// ptsLUT samples the palette ramp from lo to hi into a fixed table.
func ptsLUT(p Palette, lo, hi float64) (lut [ptsColourSteps]RGB) {
	for i := range lut {
		lut[i] = p.At(lo + (hi-lo)*float64(i)/float64(ptsColourSteps-1))
	}
	return lut
}

// ptsLUTAt picks the LUT entry for t in 0..1.
func ptsLUTAt(lut *[ptsColourSteps]RGB, t float64) RGB {
	i := int(t*float64(ptsColourSteps-1) + 0.5)
	return lut[min(max(i, 0), ptsColourSteps-1)]
}

// ptsGlide moves cur toward target with time constant tau seconds, so audio
// reactions ease in and out instead of flickering frame to frame.
func ptsGlide(cur, target, dt, tau float64) float64 {
	return cur + (target-cur)*(1-math.Exp(-dt/tau))
}

// ptsDT is the frame's elapsed time, clamped so a stalled frame (a resize, a
// suspended laptop) cannot fling a simulation forward.
func ptsDT(a Audio) float64 {
	if !(a.DT > 0) {
		return 0
	}
	return min(a.DT, 0.25)
}

type plasma struct {
	cols, rows int
	t          float64 // phase, wrapped to 0..1

	speed      float64 // animation speed
	scale      float64 // feature size; higher = finer, busier field
	complexity int     // extra octaves of detail folded in, 1..4
	contrast   float64 // contrast applied to the normalised field
	// Field values at or below this are not drawn at all. In panefx it was
	// the biggest cost lever (every lit cell was a draw call); here it is
	// mostly the look: the dark half of the field stays empty, so the panel
	// is not a wall of glyphs.
	darkcut float64

	ramp []rune
	lut  [ptsColourSteps]RGB

	// Music, smoothed.
	mid  float64
	kick float64
}

func newPlasma() Effect {
	return &plasma{
		speed:      1,
		scale:      1,
		complexity: 3,
		contrast:   1,
		darkcut:    0.5,
		ramp:       plasmaRamp,
	}
}

func init() { Register("plasma", 30, newPlasma) }

func (p *plasma) Name() string { return "plasma" }

func (p *plasma) Resize(cols, rows int) { p.cols, p.rows = max(cols, 0), max(rows, 0) }

func (p *plasma) SetPalette(pal Palette) { p.lut = ptsLUT(pal, 0, 1) }

func (p *plasma) Step(a Audio) {
	dt := ptsDT(a)
	var mid, kick float64
	if a.Playing {
		mid, kick = Drive(a.Mid), a.Kick
	}
	p.mid = ptsGlide(p.mid, mid, dt, 0.25)
	// Kick already decays over ~0.3 s; follow it quickly so the pulse lands
	// on the beat.
	p.kick = ptsGlide(p.kick, kick, dt, 0.05)

	// Wall-clock paced, so the field drifts at the same rate whatever the
	// call rate: 0.08 of a cycle per second at speed 1, as in panefx.
	rate := p.speed * 0.08 * (1 + 1.2*p.mid)
	p.t = math.Mod(p.t+dt*rate, 1)
}

// field is the plasma at a cell, normalised to 0..1.
func (p *plasma) field(col, row int) float64 {
	scale := max(p.scale, 0.01)
	// Normalise to the SHORT axis so the feature size is the same on an
	// ultrawide as on a square panel; dividing each axis by its own length
	// stretches the pattern to the screen instead.
	short := float64(max(min(p.cols, p.rows), 1))
	// Measured FROM THE CENTRE, in units of the short axis. The radial term
	// needs a centre, and the panel's middle is the only one that makes sense
	// (a corner puts the rings off-screen on a wide panel). Measuring from
	// the centre also keeps the feature SIZE fixed: an offset that grows with
	// cols stretches every wave on a wide panel.
	x := (float64(col) - float64(p.cols)/2) / short * 8 * scale
	// Doubled: a cell is about twice as tall as it is wide.
	y := (float64(row) - float64(p.rows)/2) / short * 8 * scale * 2
	t := p.t * 2 * math.Pi

	v := math.Sin(x) + math.Sin(y*0.5+t) + math.Sin((x+y)*0.5+t)
	// Radial term: breaks up the axis-aligned plaid the three above would
	// make on their own. Centred, so it reads as rings from the middle.
	v += math.Sin(math.Sqrt(x*x+y*y) - t*2)
	terms := 4.0

	// Extra octaves: same field at higher frequency and lower weight.
	for o := 1; o < min(max(p.complexity, 1), 4); o++ {
		f := 1 + float64(o)
		w := 1 / f
		v += (math.Sin(x*f) + math.Sin(y*f*0.5+t*f)) * w
		terms += 2 * w
	}

	// Mean of the terms, mapped from [-1,1] to [0,1].
	mean := v / terms
	contrast := p.contrast * (1 + 0.5*p.mid)
	return min(max(mean*contrast*0.5+0.5, 0), 1)
}

func (p *plasma) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= p.cols || row >= p.rows {
		return 0, RGB{}, false
	}
	v := p.field(col, row)
	if v <= p.darkcut-0.1*p.kick {
		return 0, RGB{}, false
	}
	n := len(p.ramp)
	ch := p.ramp[min(int(v*float64(n)), n-1)]
	if ch == ' ' {
		// The dimmest ramp slot draws nothing rather than a space.
		return 0, RGB{}, false
	}
	return ch, ptsLUTAt(&p.lut, v+0.2*p.kick), true
}
