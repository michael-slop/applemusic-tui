package fx

// Plasma: a radial spectrum glow. The name and the texture come from panefx's
// plasma.rs; the picture is the spectrum, built on the torus blueprint (see
// the package doc).
//
// Radius is the axis. Every cell is converted to a radius about the panel's
// centre, normalised so the largest circle that fits is 1, and that radius
// picks a band: bass at the centre, treble at the rim (Audio.BandAt(r)). The
// ring's BRIGHTNESS is its band's level, so the disc is a round spectrum
// analyser: a loud bass line lights the core, hi-hats light the rim, and a
// quiet band leaves a dark ring.
//
// The plasma survives only as texture. The classic sum-of-sines field is
// evaluated in polar coordinates at an angle that turns slowly and steadily
// (the torus's spin), and it only shades a ring by up to 30% and nudges its
// colour along the theme ramp. It never churns and it cannot light a ring its
// band has not lit.
//
// Cell aspect: a terminal cell is about twice as tall as it is wide, so the y
// distance is doubled before the radius is taken; the rings are round.
//
// Everything is rasterised in Step into a glyph/colour grid; Cell is a
// lookup.
//
// plasma-square is the same effect filling the whole panel. Both axes are
// folded about the centre the way Spectrum folds one (u = |2x-1|, v = |2y-1|),
// so all four quadrants are mirror images, and the folded pair becomes a
// radius through a rounded square (a superellipse, scaled so the corners reach
// the rim): bass at the centre, treble in the corners, no blank cells. The
// texture angle is folded too, so the slow spin turns as a kaleidoscope.
//
// How it hears the music: each ring glows with its band's reactive level
// (React, slider-scaled; 0.5 = normal for this song). A kick breathes the disc
// outward -- every ring is pushed out by up to 18%, like the sphere's latitudes
// -- and flashes it up to 25% brighter, fading with the kick. In silence React
// decays to 0 and every ring goes dark, leaving a faint resting disc at the
// centre that only the slow texture spin touches.

import "math"

// plasmaRamp is the glyph ramp, sparsest first; index 0 is never drawn.
var plasmaRamp = []rune(" .:-=+*#%@")

const (
	plasmaSpin   = 0.25 // texture rotation, rad/s (at the music's pace)
	plasmaBreath = 0.18 // how far a full kick pushes the rings out
	plasmaFlash  = 0.25 // how much brighter a full kick makes them
	plasmaTex    = 0.30 // how much of a ring's brightness the texture can shade
	plasmaRest   = 0.16 // brightness at the centre of the resting disc
	plasmaRestR  = 0.35 // resting disc radius (fraction of the full disc)
	plasmaCut    = 0.07 // brightness below this is not drawn
	plasmaEdge   = 0.06 // the disc fades out over this much radius past 1
)

type plasma struct {
	square     bool // plasma-square: quadrant-folded rounded-square rings
	cols, rows int
	theta      float64 // texture rotation, wrapped to 0..2π

	// Per cell, fixed for a panel size: normalised radius and angle.
	rad, ang []float64
	// Per cell, rasterised in Step: ramp index (0 = blank) and LUT index.
	glyph, shade []uint8

	lut  [ptsColourSteps]RGB
	kick float64 // smoothed
}

func newPlasma() Effect { return &plasma{} }

func newPlasmaSquare() Effect { return &plasma{square: true} }

func init() {
	Register("plasma", 30, newPlasma)
	Register("plasma-square", 30, newPlasmaSquare)
}

func (p *plasma) Name() string {
	if p.square {
		return "plasma-square"
	}
	return "plasma"
}

// FreeMotion: the texture turns rigidly even on a steady spectrum.
func (p *plasma) FreeMotion() string { return "spin" }

func (p *plasma) SetPalette(pal Palette) { p.lut = ptsLUT(pal, 0, 1) }

func (p *plasma) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == p.cols && rows == p.rows && p.rad != nil {
		return
	}
	p.cols, p.rows = cols, rows
	n := cols * rows
	p.rad, p.ang = make([]float64, n), make([]float64, n)
	p.glyph, p.shade = make([]uint8, n), make([]uint8, n)
	cx, cy := float64(cols)/2, float64(rows)/2
	// In cell widths: the widest circle that fits is cols/2 across and rows
	// (half-height doubled) tall.
	rmax := max(min(float64(cols)/2, float64(rows)), 0.5)
	for row := range rows {
		for col := range cols {
			i := row*cols + col
			dx := float64(col) + 0.5 - cx
			dy := (float64(row) + 0.5 - cy) * 2 // cells are ~1:2
			if p.square {
				p.rad[i] = plasmaSquareRadius(math.Abs(dx)/cx, math.Abs(dy)/(2*cy))
				p.ang[i] = math.Atan2(math.Abs(dy), math.Abs(dx))
				continue
			}
			p.rad[i] = math.Hypot(dx, dy) / rmax
			p.ang[i] = math.Atan2(dy, dx)
		}
	}
}

// plasmaSquareRadius maps folded offsets u, v (0 at the centre, 1 at the
// panel's edges) to plasma-square's radius: 0 at the centre, 1 in the corners,
// 2^-1/4 (~0.84) at the middle of each edge.
func plasmaSquareRadius(u, v float64) float64 {
	return math.Sqrt(math.Sqrt(u*u*u*u+v*v*v*v)) / math.Sqrt(math.Sqrt(2))
}

// texture is the plasma field in polar form at radius r and angle phi, 0..1.
// Rotating phi rotates the whole pattern rigidly.
func plasmaTexture(r, phi float64) float64 {
	v := math.Sin(3*phi+5*r) + math.Sin(5*phi-9*r+1.3) + math.Sin(2*phi+13*r)
	return v/6 + 0.5
}

func (p *plasma) Step(a Audio) {
	dt := ptsDT(a)
	var kick float64
	if a.Playing {
		kick = a.Kick
	}
	// Kick already decays over ~0.3 s; follow it quickly so the breath lands
	// on the beat.
	p.kick = ptsGlide(p.kick, kick, dt, 0.05)
	p.theta = math.Mod(p.theta+dt*plasmaSpin, 2*math.Pi)

	breath := 1 / (1 + plasmaBreath*p.kick)
	flash := 1 + plasmaFlash*p.kick
	n := len(plasmaRamp)
	for i, r0 := range p.rad {
		// Breathing out: the ring drawn at r0 is the one that sat further in.
		r := r0 * breath
		if r > 1+plasmaEdge {
			p.glyph[i] = 0
			continue
		}
		edge := min(max((1+plasmaEdge-r)/plasmaEdge, 0), 1)
		tex := plasmaTexture(r, p.ang[i]+p.theta)
		b := a.BandAt(r, false) * (1 - plasmaTex + plasmaTex*tex) * edge * flash
		if r < plasmaRestR {
			b = max(b, plasmaRest*(1-r/plasmaRestR))
		}
		b = min(b, 1)
		if b < plasmaCut {
			p.glyph[i] = 0
			continue
		}
		p.glyph[i] = uint8(min(max(int(b*float64(n)), 1), n-1))
		c := b*0.8 + 0.2*tex
		p.shade[i] = uint8(min(max(int(c*float64(ptsColourSteps-1)+0.5), 0), ptsColourSteps-1))
	}
}

func (p *plasma) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= p.cols || row >= p.rows {
		return 0, RGB{}, false
	}
	i := row*p.cols + col
	g := p.glyph[i]
	if g == 0 {
		return 0, RGB{}, false
	}
	return plasmaRamp[g], p.lut[p.shade[i]], true
}
