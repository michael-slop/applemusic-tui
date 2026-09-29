package fx

// The classic demoscene tunnel, ported from panefx's tunnel.rs.
//
// Every cell is converted to polar coordinates about the centre, and those two
// numbers index a texture:
//
//	depth = k / radius        (perspective: far things are small)
//	angle = atan2(dy, dx)
//
// Scrolling depth with time pulls the texture toward the viewer; scrolling
// angle spins it. The illusion is entirely in k / radius: that division is
// what makes a flat checkerboard read as a receding shaft.
//
// The texture is a checkerboard computed on the fly rather than stored: it is
// two floors and an xor, and a table would be a cache miss per cell for no
// benefit.
//
// Cell aspect: terminal cells are about twice as tall as they are wide, so the
// y delta is doubled before the radius is taken (the same 2:1 panefx used).
// Without that the "circular" mouth of the tunnel is a tall ellipse.
//
// One deliberate change from panefx: travel and spin keep separate phases.
// panefx derived both from one clock wrapped at 8, which is a whole texture
// period for travel but not for spin (0.3 x 12 slices x 8 = 28.8 cells), so
// the texture jumped sideways every ~160 s. Separate phases, each wrapped on
// its own texture period, remove the hitch and let the music push travel
// without also spinning the walls.
//
// How it hears the music: the spectrum shapes the wall -- every band owns a
// strip round the circumference, bass at the floor and treble at the ceiling,
// mirrored left/right, and swells and brightens it (spec/around). Then the
// bass (Drive(a.Bass), smoothed) speeds forward travel by up to 2.5x, and each kick adds a short surge of up to 4x more
// that fades with the kick, so the shaft lunges on the beat; the kick also
// lifts the walls' brightness a little. The treble quickens the spin by up to
// half again. Paused or at reactivity 0 it travels like the original.

import "math"

var tunnelRamp = []rune(" .:-=+*#%@")

// Spectrum shaping constants: how many points round the wall the spectrum is
// sampled at, how far a band at +0.5 pulls the wall in (depth x 0.8), and how
// much brighter it draws it.
const (
	tunnelSpecSteps = 64
	tunnelSpecBulge = 0.4
	tunnelSpecGlow  = 0.8
)

type tunnel struct {
	cols, rows int

	// Phases in texture cells, each wrapped on the checkerboard's period (2).
	travel, spinPh float64

	speed   float64 // forward travel
	spin    float64 // rotation about the axis; signed
	slices  int     // texture cells around the circumference
	rings   float64 // texture density along the depth axis
	fog     float64 // how fast brightness falls with depth
	darkcut float64 // brightness at or below this is not drawn

	ramp []rune
	lut  [ptsColourSteps]RGB

	// Per-cell depth and angle, precomputed. Both come from sqrt and atan2 of
	// the cell's offset from the centre and NEITHER depends on time, only on
	// the grid; panefx measured computing them per frame at ~100% of a core.
	// hole marks the centre cell, where the radius is zero.
	depth, angle []float64
	hole         []bool
	// Spectrum shaping. around is each cell's position round the wall, 0 at
	// the floor to 1 at the ceiling, the same for left and right (so the
	// shape is mirrored); spec is this frame's band deviation sampled at
	// tunnelSpecSteps points along it.
	around []float64
	spec   [tunnelSpecSteps]float64

	// Music, smoothed.
	bass, treble, kick float64
}

func newTunnel() Effect {
	return &tunnel{
		speed:   1,
		spin:    0.3,
		slices:  12,
		rings:   1,
		fog:     1,
		darkcut: 0.42,
		ramp:    tunnelRamp,
	}
}

func init() { Register("tunnel", 31, newTunnel) }

func (t *tunnel) Name() string { return "tunnel" }

func (t *tunnel) SetPalette(p Palette) { t.lut = ptsLUT(p, 0, 1) }

func (t *tunnel) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == t.cols && rows == t.rows && t.depth != nil {
		return
	}
	t.cols, t.rows = cols, rows
	n := cols * rows
	t.depth, t.angle, t.hole = make([]float64, n), make([]float64, n), make([]bool, n)
	t.around = make([]float64, n)
	cx, cy := float64(cols)/2, float64(rows)/2
	short := float64(max(min(cols, rows), 1))
	for row := range rows {
		for col := range cols {
			i := row*cols + col
			dx := float64(col) - cx
			// Doubled: cells are twice as tall as wide, so this makes the
			// tunnel mouth circular rather than a tall ellipse.
			dy := (float64(row) - cy) * 2
			r := math.Hypot(dx, dy)
			if r < 0.5 {
				t.hole[i] = true
				continue
			}
			t.depth[i] = short * 0.5 / r
			t.angle[i] = math.Atan2(dy, dx) / (2 * math.Pi)
			// dy grows downward: acos(dy/r) is 0 at the floor, pi at the ceiling.
			t.around[i] = math.Acos(min(max(dy/r, -1), 1)) / math.Pi
		}
	}
}

func (t *tunnel) Step(a Audio) {
	dt := ptsDT(a)
	var bass, treble, kick float64
	if a.Playing {
		bass, treble, kick = Drive(a.Bass), Drive(a.Treble), a.Kick
	}
	t.bass = ptsGlide(t.bass, bass, dt, 0.2)
	t.treble = ptsGlide(t.treble, treble, dt, 0.3)
	t.kick = ptsGlide(t.kick, kick, dt, 0.05)
	a.SpectrumRow(t.spec[:], false)

	// panefx: clock += 0.05 * speed per second; texture v = clock * 8 and
	// u = clock * spin * slices. Same rates, as two phases.
	clock := dt * t.speed * 0.05
	surge := 1 + 1.5*t.bass + 4*t.kick
	t.travel = math.Mod(t.travel+clock*8*surge, 2)
	t.spinPh = math.Mod(t.spinPh+clock*t.spin*float64(max(t.slices, 1))*(1+0.5*t.treble), 2)
	if t.spinPh < 0 { // spin is signed; keep the phase in [0, 2)
		t.spinPh += 2
	}
}

// sample is the brightness (0..1) and depth at a cell; ok=false at the exact
// centre, where the radius is zero and the depth would be infinite.
func (t *tunnel) sample(col, row int) (b, depth float64, ok bool) {
	i := row*t.cols + col
	if t.hole[i] {
		return 0, 0, false
	}
	depth = t.depth[i]
	// Spectrum shaping: where the band under this part of the wall is above
	// its norm the wall swells toward you (shallower depth, so the checker
	// ripples) and brightens. 0 at rest: the original tunnel.
	dev := t.spec[min(int(t.around[i]*tunnelSpecSteps), tunnelSpecSteps-1)]
	depth *= 1 - tunnelSpecBulge*dev
	u := t.angle[i]*float64(max(t.slices, 1)) + t.spinPh
	v := depth*4*max(t.rings, 0.05) + t.travel

	// Checkerboard: xor of the two parities.
	cell := (int64(math.Floor(u)) & 1) ^ (int64(math.Floor(v)) & 1)

	// Fog: far parts of the shaft fade out, which is most of the depth cue.
	lit := min(max(1/(1+depth*0.35*max(t.fog, 0)), 0), 1)
	if cell != 0 {
		lit *= 0.45
	}
	lit = min(lit*(1+tunnelSpecGlow*dev), 1)
	return lit, depth, true
}

func (t *tunnel) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= t.cols || row >= t.rows {
		return 0, RGB{}, false
	}
	b, _, ok := t.sample(col, row)
	if !ok {
		return 0, RGB{}, false
	}
	b = min(b*(1+0.3*t.kick), 1)
	if b <= t.darkcut {
		return 0, RGB{}, false
	}
	n := len(t.ramp)
	ch := t.ramp[min(int(b*float64(n)), n-1)]
	if ch == ' ' {
		return 0, RGB{}, false
	}
	// panefx mixed far -> near by brightness; here brightness walks the theme.
	return ch, ptsLUTAt(&t.lut, b), true
}
