package fx

// Starfield: 32 emitters, one per band. The name and the glyph/brightness
// feel come from panefx's starfield.rs; the picture is the spectrum, built on
// the torus blueprint (see the package doc).
//
// Direction is the axis. Every band owns a direction out of the centre, laid
// round the circle like the tunnel's walls: bass straight down, treble
// straight up, mirrored left/right, so each band has two emitters (64 lanes).
// A lane launches stars at a rate that grows with the square of its band's
// level (so a spike clearly outshouts a merely present band) and at a speed
// that grows with it; the stars fly straight out along the lane and fade
// with distance. So the picture is a spectrum made of motion: a thumping bass
// line sprays the bottom of the screen, cymbals spray the top, and a quiet
// band sends nothing at all.
//
// Launches are deterministic: each lane keeps an accumulator that fills at
// its band's rate and fires a star each time it passes 1, so a steady
// spectrum keeps a steady density. The only randomness is launch jitter (a
// star's exact angle within its lane, and +-15% speed), drawn from a
// constant-seeded generator, and only when a band launches.
//
// Cell aspect: a terminal cell is about twice as tall as it is wide, so
// positions are kept in cell widths and the y is halved on the way to a row;
// the lanes fan out evenly rather than squashed.
//
// A star's position is not a function of its cell, so Step projects every
// star into a reused brightness grid and Cell reads it. The star pool is
// sized once per panel size, so Step does not allocate.
//
// How it hears the music: each lane's launch rate and speed follow its band's
// reactive level (React, slider-scaled; 0.5 = normal for this song) and a
// star is born brighter the louder its band was. A kick warps every star
// outward up to 3x faster and swells the central ring. In silence React
// decays to 0, nothing launches, the field empties, and what remains is the
// resting silhouette: a faint ring at the centre, each point of which glows
// with its own lane's band while the music plays.

import "math"

// starRamp is the glyph by brightness, dimmest first.
var starRamp = [4]rune{'.', '+', '*', '@'}

const (
	// starSeed is fixed so two instances animate identically (the
	// conformance test compares them frame for frame).
	starSeed  = 0x5EED
	starBands = 32
	starLanes = 2 * starBands // each band emits both left and right

	starRate  = 6.0  // launches per second per lane at full level (per 20 cells of radius)
	starFloor = 0.08 // a band at or below this launches nothing
	starSpeed = 0.9  // launch speed per unit level, in panel radii per second
	starSlow  = 0.4  // launch speed at level 0, panel radii per second
	starWarp  = 2.0  // extra speed a full kick adds (x)
	starRest  = 0.14 // brightness of the resting ring
)

// star is one particle: distance travelled along its unit direction, in cell
// widths, and its launch properties.
type star struct {
	r, dx, dy, speed, b0 float64
}

type starfield struct {
	cols, rows int

	stars []star // live stars; capacity fixed per panel size
	acc   [starLanes]float64
	level [starLanes]float64 // this frame's band level per lane
	rng   *Rand

	// Projected brightness per cell, 0 = empty. Reused every frame.
	grid []float64

	lut  [ptsColourSteps]RGB
	kick float64 // smoothed
}

func newStarfield() Effect {
	s := &starfield{rng: NewRand(starSeed)}
	s.resetLanes()
	return s
}

func init() { Register("starfield", 32, newStarfield) }

func (s *starfield) Name() string { return "starfield" }

// FreeMotion: stars move on a steady spectrum, but only the ones bands launch.
func (s *starfield) FreeMotion() string { return "particles" }

// SetPalette: the far end starts a fifth of the way up the theme ramp so the
// dimmest stars stay visible.
func (s *starfield) SetPalette(p Palette) { s.lut = ptsLUT(p, 0.2, 1) }

// resetLanes staggers the accumulators (golden-ratio phases) so the lanes do
// not all fire on the same frame and the density does not pulse.
func (s *starfield) resetLanes() {
	for i := range s.acc {
		s.acc[i] = math.Mod(float64(i)*0.6180339887, 1)
	}
}

// radius is the fade distance in cell widths: stars are gone by then.
func (s *starfield) radius() float64 {
	return max(float64(s.cols)/2, float64(s.rows), 1)
}

// sizeRate scales launches with the panel, so a big panel is not sparse.
func (s *starfield) sizeRate() float64 { return max(1, s.radius()/20) }

func (s *starfield) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == s.cols && rows == s.rows && s.grid != nil {
		return
	}
	s.cols, s.rows = cols, rows
	s.grid = make([]float64, cols*rows)
	// Worst case: every lane at full rate, stars at the slowest speed.
	life := 1 / starSlow
	s.stars = make([]star, 0, int(starLanes*starRate*s.sizeRate()*life)+starLanes)
	s.resetLanes()
}

// laneBand is the band a lane emits for.
func laneBand(lane int) int { return lane / 2 }

func (s *starfield) Step(a Audio) {
	dt := ptsDT(a)
	var kick float64
	if a.Playing {
		kick = a.Kick
	}
	s.kick = ptsGlide(s.kick, kick, dt, 0.06)
	if s.cols == 0 || s.rows == 0 {
		return
	}
	R := s.radius()
	warp := 1 + starWarp*s.kick

	// Fly: every star moves out along its lane and dies past the fade
	// radius. Swap-remove keeps the pool allocation-free.
	for i := 0; i < len(s.stars); {
		st := &s.stars[i]
		st.r += st.speed * dt * warp
		if st.r >= R {
			s.stars[i] = s.stars[len(s.stars)-1]
			s.stars = s.stars[:len(s.stars)-1]
			continue
		}
		i++
	}

	// Launch: each lane's accumulator fills at its band's rate.
	ring := s.ringRadius()
	for lane := range s.acc {
		lv := min(max(a.React[laneBand(lane)], 0), 1)
		s.level[lane] = lv
		// Squared, so a spiked band clearly outshouts a merely present one.
		x := max(0, lv-starFloor) / (1 - starFloor)
		rate := starRate * s.sizeRate() * x * x
		if rate <= 0 {
			continue
		}
		s.acc[lane] += rate * dt
		for s.acc[lane] >= 1 {
			s.acc[lane]--
			// How long ago, within this frame, the star was due: start it
			// that far along so launches are evenly spaced in space too.
			age := s.acc[lane] / rate
			s.launch(lane, lv, ring, age, R)
		}
	}

	// Project.
	clear(s.grid)
	cx, cy := float64(s.cols)/2, float64(s.rows)/2
	for i := range s.stars {
		st := &s.stars[i]
		b := st.b0 * (1 - st.r/R)
		s.plot(cx+st.r*st.dx, cy+st.r*st.dy*0.5, b)
	}
	// The resting ring: one point per lane, brightening with its band.
	for lane := range starLanes {
		dx, dy := laneDir(lane, 0.5)
		s.plot(cx+ring*dx, cy+ring*dy*0.5, starRest+0.5*s.level[lane])
	}
}

// ringRadius is the central ring's radius in cell widths; it swells with the
// kick.
func (s *starfield) ringRadius() float64 {
	return max(1.5, 0.08*s.radius()) * (1 + 0.6*s.kick)
}

// laneDir is the unit direction (screen x right, y down, in cell widths) of a
// lane at fraction f (0..1) across its slice: bass straight down, treble
// straight up, even lanes on the right and odd on the left.
func laneDir(lane int, f float64) (dx, dy float64) {
	phi := math.Pi * (float64(laneBand(lane)) + f) / starBands
	sn, cs := math.Sincos(phi)
	if lane%2 == 1 {
		sn = -sn
	}
	return sn, cs
}

func (s *starfield) launch(lane int, level, ring, age, R float64) {
	if len(s.stars) == cap(s.stars) {
		return // pool full: drop the launch rather than allocate
	}
	dx, dy := laneDir(lane, s.rng.Float())
	speed := R * (starSlow + starSpeed*level) * (0.85 + 0.3*s.rng.Float())
	s.stars = append(s.stars, star{
		r:     ring + speed*age,
		dx:    dx,
		dy:    dy,
		speed: speed,
		b0:    0.45 + 0.55*level,
	})
}

func (s *starfield) plot(x, y, b float64) {
	if x < 0 || y < 0 || x >= float64(s.cols) || y >= float64(s.rows) || b <= 0 {
		return
	}
	idx := int(y)*s.cols + int(x)
	// Keep the brightest star in a shared cell.
	if b > s.grid[idx] {
		s.grid[idx] = min(b, 1)
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
	idx := min(int(b*b*float64(len(starRamp))), len(starRamp)-1)
	return starRamp[idx], ptsLUTAt(&s.lut, b), true
}
