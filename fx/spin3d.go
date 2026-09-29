package fx

// Rotating 3D solids with a z-buffer, ported from panefx's spin3d.rs. panefx's
// module also drew the donut and the sphere; amtui already has its own torus
// and sphere visualizers, so only the cube is ported here (the galaxy was dropped 2026-09-29).
//
// The method is Andy Sloane's donut.c (2006), which panefx reimplemented:
//
//   - walk the surface by two parameters, which gives points AND surface
//     normals for free;
//   - rotate by two more angles that advance with time;
//   - project with 1/z, and keep 1/z as the depth key: larger is nearer, so
//     the test is a single > and there is no division in the inner loop;
//   - shade by the dot product of the normal with a fixed light direction,
//     and index a ramp with it.
//
// A z-buffer is what makes it a solid rather than a wireframe: the far side
// is computed, then rejected because the near side already claimed the cells.
// The shape only swaps the parametric surface; the machinery (rotate,
// project, depth-test, shade) is shared, which is why the shapes share a
// module. Everything is rasterized in Step, so Cell is a table lookup.
//
// Cell aspect: x is projected at twice the scale of y, and the fit is to
// min(cols, 2*rows), because a terminal cell is about twice as tall as it is
// wide. panefx used the same 2:1, so the solids stay square/round here.
//
// How it hears the music: a kick kicks the spin. Each kick sets a rotation
// impulse that decays over ~0.6 s and multiplies both spin rates by up to
// 3.5x, so the solid lurches round on the beat and coasts back to cruising
// speed. The bass (Drive(a.Bass), smoothed) swells the object by up to 18%,
// and the kick briefly lifts the shading a step or two up the ramp. Paused or
// at reactivity 0 it turns exactly like the original.

import "math"

// spinRamp is the luminance ramp, dimmest first: Sloane's original.
var spinRamp = [12]rune{'.', ',', '-', '~', ':', ';', '=', '!', '*', '#', '$', '@'}

// spinEmpty marks a cell no surface claimed.
const spinEmpty = math.MaxUint8

type spinShape int

const (
	spinCube spinShape = iota
)

type spinSample struct{ p, n [3]float64 }

type spin3d struct {
	name       string
	shape      spinShape
	cols, rows int

	a, b         float64 // the two rotation angles
	spinA, spinB float64 // rad/s about each axis; signed
	scale        float64 // object scale
	density      float64 // surface sampling density; higher = smoother, dearer

	// The surface, sampled once per (shape, sample count): the point and
	// normal at each (u, v) never change, only their rotation does.
	surf   []spinSample
	surfNU int

	// Depth key per cell (1/z; larger is nearer) and ramp index per cell,
	// spinEmpty for none. Reused every frame.
	zbuf []float64
	lum  []uint8

	lut [len(spinRamp)]RGB

	// Music.
	impulse, bass, kick float64
}

func newSpin3d(name string, shape spinShape) func() Effect {
	return func() Effect {
		return &spin3d{name: name, shape: shape, spinA: 1, spinB: 0.5, scale: 1, density: 1}
	}
}

func init() {
	Register("cube", 40, newSpin3d("cube", spinCube))
}

func (s *spin3d) Name() string { return s.name }

// SetPalette: panefx mixed a dark shadow colour to a bright lit colour by the
// ramp index. The shadow end starts a little above the theme's darkest stop so
// faces turned away from the light still read against the background.
func (s *spin3d) SetPalette(p Palette) {
	for i := range s.lut {
		s.lut[i] = p.At(0.1 + 0.9*float64(i)/float64(len(s.lut)-1))
	}
}

func (s *spin3d) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == s.cols && rows == s.rows && s.zbuf != nil {
		return
	}
	s.cols, s.rows = cols, rows
	s.zbuf = make([]float64, cols*rows)
	s.lum = make([]uint8, cols*rows)
	for i := range s.lum {
		s.lum[i] = spinEmpty
	}
}

// steps is how many samples to take along each parameter, scaled to the
// panel: a big panel needs more points to look solid, a small one should not
// pay for them.
func (s *spin3d) steps() int {
	d := min(max(s.density, 0.1), 4)
	return min(max(int(float64(max(s.cols, s.rows))*0.9*d), 24), 720)
}

// pointAt is the surface point and normal at parameters (u, v), each in
// 0..2π. Returning the normal alongside the point is what makes shading
// cheap: for every surface here it is available analytically.
func (s *spin3d) pointAt(u, v float64) (p, n [3]float64) {
	const tau = 2 * math.Pi
	switch s.shape {
	default:
		// Six faces, chosen by where v falls; u walks one axis and v's
		// fraction the other. A cube has no smooth parameterisation, so this
		// is a deliberate patchwork.
		fv := v / tau * 6
		face := int(fv) % 6
		a := (u/tau*2 - 1) * 1.6
		b := ((fv-math.Floor(fv))*2 - 1) * 1.6
		switch face {
		case 0:
			return [3]float64{1.6, a, b}, [3]float64{1, 0, 0}
		case 1:
			return [3]float64{-1.6, a, b}, [3]float64{-1, 0, 0}
		case 2:
			return [3]float64{a, 1.6, b}, [3]float64{0, 1, 0}
		case 3:
			return [3]float64{a, -1.6, b}, [3]float64{0, -1, 0}
		case 4:
			return [3]float64{a, b, 1.6}, [3]float64{0, 0, 1}
		default:
			return [3]float64{a, b, -1.6}, [3]float64{0, 0, -1}
		}
	}
}

// sampleSurface refreshes the cached surface if the sample count changed.
func (s *spin3d) sampleSurface() {
	nu := s.steps()
	if nu == s.surfNU && s.surf != nil {
		return
	}
	s.surfNU = nu
	nv := nu
	if s.shape == spinCube {
		// Deviation from panefx: the cube's v parameter is split six ways
		// (one slice per face), so each face got only n/6 samples across it
		// and the near face rendered with holes the back face showed through.
		// Four times the v samples gives every face ~2n/3 each way.
		nv = nu * 4
	}
	s.surf = s.surf[:0]
	du, dv := 2*math.Pi/float64(nu), 2*math.Pi/float64(nv)
	for j := range nv {
		v := float64(j) * dv
		for i := range nu {
			p, nrm := s.pointAt(float64(i)*du, v)
			s.surf = append(s.surf, spinSample{p, nrm})
		}
	}
}

func (s *spin3d) Step(a Audio) {
	dt := ptsDT(a)
	var bass, kick float64
	if a.Playing {
		bass, kick = Drive(a.Bass), a.Kick
	}
	// The impulse jumps with the kick and coasts down over ~0.6 s, longer
	// than the kick itself, so the solid carries its momentum.
	s.impulse = max(s.impulse*math.Exp(-dt/0.6), kick)
	s.bass = ptsGlide(s.bass, bass, dt, 0.2)
	s.kick = ptsGlide(s.kick, kick, dt, 0.05)

	boost := 1 + 2.5*s.impulse
	s.a = math.Mod(s.a+dt*s.spinA*boost, 2*math.Pi)
	s.b = math.Mod(s.b+dt*s.spinB*boost, 2*math.Pi)

	if s.cols == 0 || s.rows == 0 {
		return
	}
	clear(s.zbuf)
	for i := range s.lum {
		s.lum[i] = spinEmpty
	}
	s.sampleSurface()

	sa, ca := math.Sincos(s.a)
	sb, cb := math.Sincos(s.b)
	// Light from up and behind the viewer's shoulder, normalised.
	light := [3]float64{0, 0.7071, -0.7071}

	scale := max(s.scale, 0.05) * (1 + 0.18*s.bass)
	// Fit to the SHORT axis, counting a cell as half as wide as it is tall,
	// so the solid stays on-screen and undistorted at any panel shape.
	k := float64(max(min(s.cols, s.rows*2), 1)) * 0.32 * scale
	cx, cy := float64(s.cols)/2, float64(s.rows)/2
	// Camera distance: big enough that the near face never crosses the
	// viewer, which would invert the projection.
	const kd = 7.0
	top := float64(len(spinRamp) - 1)
	lift := 2 * s.kick
	fc, fr := float64(s.cols), float64(s.rows)

	// Rotate about x by a, then about z by b.
	rot := func(q [3]float64) (x, y, z float64) {
		y1 := q[1]*ca - q[2]*sa
		z1 := q[1]*sa + q[2]*ca
		return q[0]*cb - y1*sb, q[0]*sb + y1*cb, z1
	}

	for i := range s.surf {
		px, py, pz := rot(s.surf[i].p)
		z := pz + kd
		if z <= 0.1 {
			continue
		}
		ooz := 1 / z
		sx := cx + k*ooz*px*2 // doubled: cells are twice as tall as wide
		sy := cy + k*ooz*py
		if sx < 0 || sy < 0 || sx >= fc || sy >= fr {
			continue
		}
		idx := int(sy)*s.cols + int(sx)
		// Depth test on 1/z: larger is nearer; one compare, no division.
		if ooz <= s.zbuf[idx] {
			continue
		}
		s.zbuf[idx] = ooz
		nx, ny, nz := rot(s.surf[i].n)
		l := nx*light[0] + ny*light[1] + nz*light[2]
		if l <= 0 {
			// Facing away from the light. Still claim the depth slot: it is
			// genuinely the nearest surface, and leaving it free lets the FAR
			// side of the solid show through the near one.
			s.lum[idx] = 0
			continue
		}
		s.lum[idx] = uint8(min(l*top+lift, top))
	}
}

func (s *spin3d) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= s.cols || row >= s.rows {
		return 0, RGB{}, false
	}
	l := s.lum[row*s.cols+col]
	if l == spinEmpty {
		return 0, RGB{}, false
	}
	return spinRamp[l], s.lut[l], true
}
