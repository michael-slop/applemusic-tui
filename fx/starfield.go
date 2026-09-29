package fx

// Perspective starfield, flying forward through a cloud of points; ported
// from panefx's starfield.rs.
//
// Each star is a point in 3D that moves toward the viewer; the screen position
// is the usual pinhole projection:
//
//	sx = cx + x / z · k
//	sy = cy + y / z · k / 2
//
// and z shrinking each frame is the entire sense of motion. A star that passes
// the viewer (z <= 0) or leaves the screen is respawned at the far plane, so
// the field is never exhausted and never needs sorting.
//
// Brightness is 1 - z/FAR: near stars are bright and, at the brightest, drawn
// with a heavier glyph. That is what makes the field read as depth rather than
// as noise; a flat-brightness starfield looks like static.
//
// Cell aspect: the y projection is halved because a terminal cell is about
// twice as tall as it is wide (panefx used the same 2:1); unhalved, the field
// spreads twice as fast vertically as horizontally.
//
// Why this one keeps a buffer: a star's position is not a function of its
// screen cell, so Cell cannot compute it. Step projects every star into a
// cell grid once, and Cell reads that. The grid is reused between frames.
//
// How it hears the music: overall drive (the mean of Drive(Bass/Mid/Treble),
// smoothed) speeds the warp by up to 2.5x, and each kick adds a burst of up
// to 4x more that fades with the kick, a jump to lightspeed on the beat.
// Faster travel also brightens the field naturally, since more stars are
// near. Paused or at reactivity 0 it cruises like the original.

import "math"

// starRamp is the glyph by brightness, dimmest first.
var starRamp = [4]rune{'.', '+', '*', '@'}

// starFar is the far plane: stars spawn here and travel toward 0.
const starFar = 32.0

// starSeed is fixed so two instances animate identically (the conformance
// test compares them frame for frame).
const starSeed = 0x5EED

type star struct{ x, y, z float64 }

type starfield struct {
	cols, rows int

	stars []star
	rng   *Rand
	// Projected brightness per cell, 0 = empty. Reused every frame.
	grid []float64

	speed   float64 // travel speed
	density float64 // stars per thousand cells (x90), so density is size independent
	fov     float64 // field of view; higher = wider, faster-spreading field
	drift   float64 // sideways drift; signed, the field can bank either way

	lut [ptsColourSteps]RGB

	// Music, smoothed.
	drive, kick float64
}

func newStarfield() Effect {
	return &starfield{
		rng:     NewRand(starSeed),
		speed:   1,
		density: 0.9,
		fov:     1,
	}
}

func init() { Register("starfield", 32, newStarfield) }

func (s *starfield) Name() string { return "starfield" }

// SetPalette: panefx mixed a far colour (a visible mid blue, never black) to
// white by brightness. The far end here starts a fifth of the way up the
// theme ramp so the dimmest stars stay visible.
func (s *starfield) SetPalette(p Palette) { s.lut = ptsLUT(p, 0.2, 1) }

// targetCount is how many stars this panel should hold, proportional to area
// so a wide panel is not sparser than a small one. 90 per 1000 cells at
// density 1.0: panefx first tried 12, which measured ~30 visible stars on a
// 160x60 panel; correct perspective, but far too empty to read as a field.
func (s *starfield) targetCount() int {
	return int(float64(s.cols*s.rows) / 1000 * s.density * 90)
}

func (s *starfield) spawn() star {
	return star{
		x: s.rng.Float()*2 - 1,
		y: s.rng.Float()*2 - 1,
		// Spread across the whole depth on spawn, not all at the far plane:
		// otherwise the first seconds are an empty screen followed by a wall
		// of stars arriving together.
		z: s.rng.Float()*starFar + 0.1,
	}
}

func (s *starfield) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == s.cols && rows == s.rows && s.grid != nil {
		return
	}
	s.cols, s.rows = cols, rows
	s.stars = s.stars[:0]
	for range s.targetCount() {
		s.stars = append(s.stars, s.spawn())
	}
	s.grid = make([]float64, cols*rows)
}

func (s *starfield) Step(a Audio) {
	dt := ptsDT(a)
	var drive, kick float64
	if a.Playing {
		drive = (Drive(a.Bass) + Drive(a.Mid) + Drive(a.Treble)) / 3
		kick = a.Kick
	}
	s.drive = ptsGlide(s.drive, drive, dt, 0.3)
	s.kick = ptsGlide(s.kick, kick, dt, 0.06)

	if s.cols == 0 || s.rows == 0 {
		return
	}
	// The population tracks the density knob.
	want := s.targetCount()
	if len(s.stars) > want {
		s.stars = s.stars[:want]
	}
	for len(s.stars) < want {
		s.stars = append(s.stars, s.spawn())
	}
	clear(s.grid)

	// panefx: 6 depth units per second at speed 1.
	move := dt * s.speed * 6 * (1 + 1.5*s.drive + 4*s.kick)
	drift := s.drift * move * 0.05
	fov := max(s.fov, 0.05)
	cx, cy := float64(s.cols)/2, float64(s.rows)/2
	k := float64(max(min(s.cols, s.rows*2), 1)) * 0.5 * fov

	for i := range s.stars {
		st := &s.stars[i]
		st.z -= move
		st.x += drift
		if st.z <= 0.05 {
			*st = s.spawn()
			st.z = starFar
			continue
		}
		sx := cx + st.x/st.z*k
		// Halved: a cell is twice as tall as it is wide.
		sy := cy + st.y/st.z*k*0.5
		if sx < 0 || sy < 0 || sx >= float64(s.cols) || sy >= float64(s.rows) {
			// Off screen: only respawn once it is also PAST the viewer.
			// Culling on screen bounds alone kills stars still approaching
			// from a wide angle and thins the edges of the field.
			if st.z < 1 {
				*st = s.spawn()
				st.z = starFar
			}
			continue
		}
		b := min(max(1-st.z/starFar, 0), 1)
		idx := int(sy)*s.cols + int(sx)
		// Keep the NEAREST star in a shared cell, so a bright close star is
		// not hidden behind a dim far one.
		if b > s.grid[idx] {
			s.grid[idx] = b
		}
	}
}

func (s *starfield) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= s.cols || row >= s.rows {
		return 0, RGB{}, false
	}
	b := s.grid[row*s.cols+col]
	if b <= 0 {
		return 0, RGB{}, false
	}
	// Gamma: linear brightness puts nearly every star in the dimmest slot,
	// because most of the depth range is far away.
	g := math.Pow(b, 2.2)
	idx := min(int(g*float64(len(starRamp))), len(starRamp)-1)
	return starRamp[idx], ptsLUTAt(&s.lut, b), true
}
