package fx

// skullspin: the michael.slop skull, spinning ghostty-style, and it jumps.
// Ported from panefx src/skullspin.rs, itself a port of the site's
// initSkullSpin: the site and this must show the same face turning the same
// way, so the crop, the pixel classification and the rotation all come from
// there rather than being re-invented. See skull_art.go for the traced sprite.
//
// # The spin
//
// Not a 3D projection: a horizontal squash. Sampling the source column at
// (x - centre) / cos(theta) + centre compresses the face as it turns away,
// which is exactly what a rotating billboard looks like and costs one divide
// per cell instead of a matrix.
//
// cos is clamped away from zero near edge-on. At exactly zero the divide is
// infinite and the skull vanishes for a frame; clamped, it thins to a sliver
// and turns through, which is what the eye expects.
//
// # The jump
//
// Deliberately IRREGULAR. A skull that acts on a strict four-second beat reads
// as a loading spinner; one whose timing you cannot predict reads as something
// alive. So it is driven by a hash of its own occurrence number:
// deterministic (no RNG state, same on every machine) but unpredictable to a
// viewer. The jump is a squash-and-stretch arc: the skull compresses before it
// leaves, stretches through the air, and compresses again on landing. Without
// the squash it is a picture being translated upward; with it, it has weight.
//
// # Colours
//
// panefx recolours the skull through params (bone, dark); here they come from
// the theme: bone is Text, the outline is the darkest ramp tone lifted a
// little toward AccentLo, and the outline's three-deep falloff fades toward
// the panel Background (panefx faded toward its near-black bg the same way).
//
// # How it hears the music
//
//   - Spin speed follows overall drive (Drive of the mean of Bass/Mid/Treble,
//     up to 2x) and gets an impulse on each Kick (the decaying pulse adds up to
//     +2x for ~0.3 s), so the skull lurches a little further round on a hit.
//     The extra angle only accumulates while it is SPINNING, so the settle and
//     the jump still happen square-on.
//   - The eye and nose sockets glow toward Accent/AccentHi with Kick, and the
//     halo brightens a touch with it.
//   - Nothing else: the jump sequence stays on its own irregular clock, and
//     time keeps running while paused so it idles exactly like panefx.
//   - At Reactivity 0 (drive 0, Kick 0) the extra angle is always 0 and every
//     colour is the plain theme colour: the panefx original.

import "math"

const (
	skullCols = 29
	skullRows = 25
	// The skull is drawn in ONE glyph, deliberately. It was an 8-step ramp
	// shaded by the lighting angle, which made the skull's apparent colour
	// shift as it spun: wrong for a solid white skull. The silhouette carries
	// the design, and shading competes with the outline. '#' is the densest
	// glyph that still reads as a filled block without the noise of '@'.
	skullBoneGlyph = '#'
	// Two glyph columns per source pixel. Character cells are about twice as
	// tall as they are wide, so doubling horizontally keeps the skull round
	// instead of squashed: the same 2:1 the site uses.
	skullXScale = 2

	// Durations are properties of the ACTION, not settings: a jump slower than
	// about three-quarters of a second stops reading as ballistic.
	skullJumpDur   = 0.72
	skullSettleDur = 0.22

	skullSpinRate = 1.5 // radians of theta per second of spinning (panefx spin=1500)
	skullActEvery = 6.0 // mean seconds of spinning between jumps (act_every=6000)
	skullSeed     = 0x5EED
)

// spinOutlineShade is the glyph an outline label draws and how strong it is (0..1
// of the outline colour). Graded rather than flat: a single tone makes the
// halo a slab, and the whole point of a three-deep edge is that it falls off.
// Shared with warlockspin.
func spinOutlineShade(b byte) (rune, float64, bool) {
	switch b {
	case '*':
		return '*', 1.0, true
	case '+':
		return '+', 0.72, true
	case '.':
		return '.', 0.45, true
	}
	return 0, 0, false
}

// spinOutlineIndex is the position of an outline label in the '*' '+' '.' ramp.
func spinOutlineIndex(b byte) int {
	switch b {
	case '*':
		return 0
	case '+':
		return 1
	}
	return 2
}

// spinHash01 hashes one integer to [0,1). A hash rather than an RNG so the
// animation is identical on every machine and across restarts: a spinner that
// desyncs between two monitors showing the same effect looks broken.
func spinHash01(n uint64) float64 {
	h := n * 0x9E3779B97F4A7C15
	h ^= h >> 30
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 27
	h *= 0x94D049BB133111EB
	h ^= h >> 31
	return float64(h>>40) / float64(uint64(1)<<24)
}

// skullAct is what the skull is doing right now.
//
// A sequence, not overlapping clocks. The first panefx version ran spin and
// acts on independent schedules, so the skull could act mid-turn, and a jump
// that starts mid-spin reads as a glitch rather than a hop. So it takes turns:
// spin, settle, do ONE thing facing the viewer, spin again.
type skullAct int

const (
	actSpin   skullAct = iota // turning; the only state in which theta advances
	actSettle                 // come to rest facing the viewer, before acting
	actJump
)

func (a skullAct) String() string {
	return [...]string{"Spin", "Settle", "Jump"}[a]
}

// skullSequence says where the sequence is at time t, given the mean seconds
// between actions. It returns the act, phase 0..1 through it, and the total
// time spent spinning so far, which is what theta is derived from. Deriving
// the angle from spin time rather than wall time is what makes the skull HOLD
// its angle while it acts instead of drifting through the pause.
func skullSequence(t, every float64, seed uint64) (skullAct, float64, float64) {
	if every <= 0 {
		return actSpin, 0, t // nothing to interleave: spin forever
	}
	clock, spun := 0.0, 0.0
	// Bounded because each iteration consumes at least skullSettleDur, so this
	// cannot spin on a pathological input.
	for k := uint64(0); k < 10000; k++ {
		// Spin for a jittered stretch, so the rhythm is never countable.
		spinLen := max(every*(0.6+spinHash01(k^seed)*0.8), 0.3)
		if t < clock+spinLen {
			return actSpin, (t - clock) / spinLen, spun + (t - clock)
		}
		clock += spinLen
		spun += spinLen
		if t < clock+skullSettleDur {
			return actSettle, (t - clock) / skullSettleDur, spun
		}
		clock += skullSettleDur
		// The jump is the only act (a wink lived here until 2026-08-20; it
		// was removed, and it had painted the forehead rather than the eyes).
		if t < clock+skullJumpDur {
			return actJump, (t - clock) / skullJumpDur, spun
		}
		clock += skullJumpDur
	}
	return actSpin, 0, spun
}

// skullJumpShape is the vertical lift and squash for a jump at phase 0..1:
// anticipation, flight, landing.
func skullJumpShape(phase, height float64) (lift, squash float64) {
	const crouch, land = 0.18, 0.82
	p := min(max(phase, 0), 1)
	if p < crouch {
		return 0, 1 - 0.22*math.Sin(p/crouch*math.Pi)
	}
	if p > land {
		return 0, 1 - 0.18*math.Sin((p-land)/(1-land)*math.Pi)
	}
	q := math.Sin((p - crouch) / (land - crouch) * math.Pi)
	return q * height, 1 + 0.16*q
}

// spinClampEdgeOn keeps a cosine away from zero. At exactly edge-on the inverse
// column map divides by zero and the sprite vanishes for a frame; clamped, it
// thins to a sliver and turns through, which is what the eye expects.
func spinClampEdgeOn(c float64) float64 {
	if math.Abs(c) < 0.07 {
		if c < 0 {
			return -0.07
		}
		return 0.07
	}
	return c
}

// spinCell is one rasterized cell.
type spinCell struct {
	ch rune
	c  RGB
}

// skullSockets marks the outline cells ENCLOSED by bone (the eyes and nose):
// every non-bone cell not reachable from the edge of the sprite without
// crossing bone. Those are what glow on a kick.
var skullSockets = func() (m [skullRows][skullCols]bool) {
	var seen [skullRows][skullCols]bool
	var stack [][2]int
	push := func(x, y int) {
		if x < 0 || y < 0 || x >= skullCols || y >= skullRows || seen[y][x] || skullArt[y][x] == '#' {
			return
		}
		seen[y][x] = true
		stack = append(stack, [2]int{x, y})
	}
	for x := range skullCols {
		push(x, 0)
		push(x, skullRows-1)
	}
	for y := range skullRows {
		push(0, y)
		push(skullCols-1, y)
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		push(p[0]+1, p[1])
		push(p[0]-1, p[1])
		push(p[0], p[1]+1)
		push(p[0], p[1]-1)
	}
	for y := range skullRows {
		for x := range skullCols {
			m[y][x] = !seen[y][x] && skullArt[y][x] != '#'
		}
	}
	return
}()

type skullSpin struct {
	cols, rows int
	// t is seconds elapsed. Everything is derived from it, so the animation
	// is a pure function of time.
	t float64
	// extra is the angle the music has added on top of the time-derived
	// theta. Only advances while spinning, so acts stay square-on.
	extra float64
	kick  float64

	pal  Palette
	grid []spinCell
}

func newSkullSpin() Effect { return &skullSpin{} }

func init() { Register("skullspin", 50, newSkullSpin) }

func (s *skullSpin) Name() string { return "skullspin" }

func (s *skullSpin) Resize(cols, rows int) {
	s.cols, s.rows = max(cols, 0), max(rows, 0)
	s.grid = make([]spinCell, s.cols*s.rows)
	s.raster()
}

func (s *skullSpin) SetPalette(p Palette) {
	s.pal = p
	s.raster()
}

func (s *skullSpin) Step(a Audio) {
	// Wall-clock, so the skull turns at the same rate whatever the frame
	// rate. Clamped so a stall does not fling it half a turn.
	dt := min(max(a.DT, 0), 0.25)
	act, _, _ := skullSequence(s.t, skullActEvery, skullSeed)
	if act == actSpin {
		energy := Drive((a.Bass + a.Mid + a.Treble) / 3)
		s.extra += dt * skullSpinRate * (1.0*energy + 2.0*a.Kick)
	}
	s.t += dt
	s.kick = a.Kick
	s.raster()
}

func (s *skullSpin) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= s.cols || row >= s.rows {
		return 0, RGB{}, false
	}
	g := s.grid[row*s.cols+col]
	return g.ch, g.c, g.ch != 0
}

// layout is the size of one sprite pixel in panel cells and where the skull
// sits. Fitted to whichever axis runs out first, so the skull is always whole
// and centred whatever the panel shape.
func (s *skullSpin) layout() (ox, oy, z float64) {
	byW := float64(s.cols) / float64(skullCols*skullXScale)
	// Leave room overhead for the jump, or the skull clips through the top.
	// 1.75, not 1.45: the arc lifts by 0.55 of the sprite height, so the
	// reservation has to cover 1.0 + 0.55 plus a margin.
	byH := float64(s.rows) / (skullRows * 1.75)
	z = min(byW, byH)
	w := float64(skullCols*skullXScale) * z
	h := skullRows * z
	// Rest LOW in the reserved space, so the headroom sits above the skull
	// where the jump needs it rather than being split evenly.
	free := max(float64(s.rows)-h, 0)
	return (float64(s.cols) - w) / 2, free * 0.78, z
}

// raster draws the current frame into the grid.
func (s *skullSpin) raster() {
	for i := range s.grid {
		s.grid[i] = spinCell{}
	}
	ox, oy, z := s.layout()
	if z <= 0 {
		return
	}
	act, phase, spun := skullSequence(s.t, skullActEvery, skullSeed)
	theta := spun*skullSpinRate + s.extra
	var raw float64
	switch act {
	case actSettle:
		// Settling and jumping happen SQUARE ON. Easing the last of the turn
		// out over the settle is what stops the stop reading as a dropped frame.
		e := min(max(phase, 0), 1)
		ease := 1 - (1-e)*(1-e)
		raw = math.Cos(theta) + (1-math.Cos(theta))*ease
	case actJump:
		raw = 1
	default:
		raw = math.Cos(theta)
	}
	c := spinClampEdgeOn(raw)
	lift, squash := 0.0, 1.0
	if act == actJump {
		lift, squash = skullJumpShape(phase, skullRows*0.55)
	}

	p := s.pal
	bone := p.Text
	dark := Lerp(p.Ramp[0], p.AccentLo, 0.35)
	// The halo brightens a touch on a kick; the sockets glow properly.
	dark = Lerp(dark, p.AccentLo, 0.5*s.kick)
	glow := Lerp(p.Accent, p.AccentHi, s.kick)
	var shades [3]RGB
	var socketShades [3]RGB
	for i, b := range []byte{'*', '+', '.'} {
		_, k, _ := spinOutlineShade(b)
		shades[i] = Lerp(p.Background, dark, k)
		socketShades[i] = Lerp(shades[i], glow, s.kick*(0.4+0.6*k))
	}

	cx := float64(skullCols-1) / 2
	for row := 0; row < s.rows; row++ {
		// Panel cell -> sprite cell, undoing the layout, the jump and the spin.
		fy := (float64(row) - oy + lift*z) / (z * squash)
		if fy < 0 || fy >= skullRows {
			continue
		}
		sy := int(fy)
		for col := 0; col < s.cols; col++ {
			fx := (float64(col) - ox) / z
			// The spin: sample the column the rotation brings here.
			u := int(math.Round((fx/skullXScale-cx)/c + cx))
			if u < 0 || u >= skullCols {
				continue
			}
			b := skullArt[sy][u]
			var out spinCell
			switch b {
			case ' ':
				continue
			case '#':
				// FLAT: one glyph, one colour, at every angle.
				out = spinCell{skullBoneGlyph, bone}
			default:
				// Dark cells stay dark at every angle, or the sockets fill in
				// as the skull turns and the face stops reading.
				g, _, ok := spinOutlineShade(b)
				if !ok {
					continue
				}
				i := spinOutlineIndex(b)
				cc := shades[i]
				if skullSockets[sy][u] {
					cc = socketShades[i]
				}
				out = spinCell{g, cc}
			}
			s.grid[row*s.cols+col] = out
		}
	}
}
