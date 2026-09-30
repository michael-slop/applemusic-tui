package fx

// Cube: a spinning box whose six faces bulge with the spectrum. The render is
// panefx's spin3d.rs (Andy Sloane's donut.c method); the shape is the
// spectrum, built on the torus blueprint (see the package doc).
//
// Faces are the axis. The 32 bands are split into six groups and each group
// owns one face; the face sits out from the centre by
//
//	base x (0.55 + 0.85 x groupLevel)
//
// exactly like the torus's tube radius, so the solid is a box with six
// independent half-extents. Bass owns the floor, treble the lid and the mids
// the four sides:
//
//	0-4 bottom   5-9 right   10-15 front   16-21 left   22-26 back   27-31 top
//
// Every face is also shaded by its group's level (a quiet group's face is
// dim), so the spectrum shows as brightness as well as shape from any angle.
//
// The method: each face is a flat rectangle, so it is rotated once per frame
// as a corner plus two edge vectors and then walked on an n x n grid; every
// sample is projected with 1/z and depth-tested on 1/z (larger is nearer, one
// compare, no division). Faces turned away from the viewer are skipped whole
// (a box is convex). Shading is the dot of the face normal with a fixed light.
// All of it is rasterised in Step into a reused buffer; Cell is a lookup.
//
// The only free motion is rigid: the box turns on a turntable (yaw) seen from
// slightly above, with a slow nod of the tilt, like the torus's spin and
// wobble. Seen from above, the lid always shows, and the turntable keeps the
// silhouette's size steady.
//
// Cell aspect: x is projected at twice the scale of y, and the fit is to
// min(cols, 2*rows), because a terminal cell is about twice as tall as it is
// wide, so the box stays square.
//
// How it hears the music: each face's distance and brightness follow its band
// group's reactive level (React, slider-scaled; 0.5 = normal for this song).
// A kick kicks the spin -- an impulse that multiplies the turn rate by up to
// 3.5x and coasts down over ~0.6 s -- breathes the whole box up to 12% bigger
// and lifts the shading a step or two up the ramp. In silence React decays to
// 0 and the box settles to a small, dim resting cube that only turns.

import "math"

// spinRamp is the luminance ramp, dimmest first: Sloane's original.
var spinRamp = [12]rune{'.', ',', '-', '~', ':', ';', '=', '!', '*', '#', '$', '@'}

// spinEmpty marks a cell no surface claimed.
const spinEmpty = math.MaxUint8

const (
	cubeBase   = 1.3  // a face's distance at band level 0.5 is ~0.98 of this
	cubeRest   = 0.55 // torus: tube radius factor at silence
	cubeSwell  = 0.85 // torus: extra radius per unit band level
	cubeBreath = 0.12 // how much bigger a full kick makes the box
	cubeTilt   = 0.5  // how far above the box the viewer sits, rad
	cubeNod    = 0.15 // how much the tilt nods, rad
	cubeDist   = 7.0  // camera distance: the near face never reaches the viewer
)

// cubeGroups are the six band groups, in face order (see cubeFaces).
var cubeGroups = [6][2]int{{0, 4}, {5, 9}, {10, 15}, {16, 21}, {22, 26}, {27, 31}}

// cubeFaces are the faces' outward normals in model space (y up), in group
// order: bottom (bass), right, front, left, back, top (treble).
var cubeFaces = [6][3]float64{{0, -1, 0}, {1, 0, 0}, {0, 0, -1}, {-1, 0, 0}, {0, 0, 1}, {0, 1, 0}}

type spin3d struct {
	name       string
	cols, rows int

	a, b         float64 // yaw and nod phases
	spinA, spinB float64 // rad/s of each; signed
	scale        float64 // object scale

	group [6]float64 // this frame's band level per face

	// Depth key per cell (1/z; larger is nearer) and ramp index per cell,
	// spinEmpty for none. Reused every frame.
	zbuf []float64
	lum  []uint8

	lut [len(spinRamp)]RGB

	// Music.
	impulse, kick float64
}

func newSpin3d(name string) func() Effect {
	return func() Effect {
		return &spin3d{name: name, spinA: 1, spinB: 0.5, scale: 1}
	}
}

func init() {
	Register("cube", 40, newSpin3d("cube"))
}

func (s *spin3d) Name() string { return s.name }

// FreeMotion: the box turns rigidly even on a steady spectrum.
func (s *spin3d) FreeMotion() string { return "spin" }

// SetPalette: the shadow end starts a little above the theme's darkest stop so
// dim faces still read against the background.
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

// faceOffset is the torus rule: how far a face sits from the centre at band
// level g.
func faceOffset(g float64) float64 { return cubeBase * (cubeRest + cubeSwell*g) }

// steps is how many samples to take along each edge of a face, scaled to the
// panel: enough that the nearest, biggest face shows no holes.
func (s *spin3d) steps() int {
	return min(max(int(float64(min(s.cols, 2*s.rows))*1.2), 8), 900)
}

func (s *spin3d) Step(a Audio) {
	dt := ptsDT(a)
	var kick float64
	if a.Playing {
		kick = a.Kick
	}
	// The impulse jumps with the kick and coasts down over ~0.6 s, longer
	// than the kick itself, so the box carries its momentum.
	s.impulse = max(s.impulse*math.Exp(-dt/0.6), kick)
	s.kick = ptsGlide(s.kick, kick, dt, 0.05)

	boost := 1 + 2.5*s.impulse
	s.a = math.Mod(s.a+dt*s.spinA*boost, 2*math.Pi)
	s.b = math.Mod(s.b+dt*s.spinB*boost, 2*math.Pi)

	for f, g := range cubeGroups {
		var sum float64
		for i := g[0]; i <= g[1]; i++ {
			sum += min(max(a.React[i], 0), 1)
		}
		s.group[f] = sum / float64(g[1]-g[0]+1)
	}

	if s.cols == 0 || s.rows == 0 {
		return
	}
	clear(s.zbuf)
	for i := range s.lum {
		s.lum[i] = spinEmpty
	}

	sy, cy0 := math.Sincos(s.a)
	st, ct := math.Sincos(-(cubeTilt + cubeNod*math.Sin(s.b)))
	// Yaw about y, then tilt about x (model y up, viewer at z = -cubeDist).
	rot := func(x, y, z float64) (float64, float64, float64) {
		x1 := x*cy0 + z*sy
		z1 := -x*sy + z*cy0
		return x1, y*ct - z1*st, y*st + z1*ct
	}
	// Light from up, left and behind the viewer's shoulder, normalised.
	const lx, ly, lz = -0.4, 0.75, -0.53
	ln := math.Sqrt(lx*lx + ly*ly + lz*lz)

	scale := max(s.scale, 0.05) * (1 + cubeBreath*s.kick)
	var e [6]float64 // half-extent per face
	for f := range e {
		e[f] = faceOffset(s.group[f]) * scale
	}
	// The box: x in [-left, right], y in [-bottom, top], z in [-front, back].
	lo := [3]float64{-e[3], -e[0], -e[2]}
	hi := [3]float64{e[1], e[5], e[4]}

	k := float64(max(min(s.cols, s.rows*2), 1)) * 0.46
	cx, cy := float64(s.cols)/2, float64(s.rows)/2
	fc, fr := float64(s.cols), float64(s.rows)
	top := float64(len(spinRamp) - 1)
	lift := 2 * s.kick
	n := s.steps()

	for f, nm := range cubeFaces {
		// The face's fixed axis and its two free ones.
		ax := 0
		switch {
		case nm[1] != 0:
			ax = 1
		case nm[2] != 0:
			ax = 2
		}
		u, v := (ax+1)%3, (ax+2)%3
		var o, du, dv [3]float64
		if nm[ax] > 0 {
			o[ax] = hi[ax]
		} else {
			o[ax] = lo[ax]
		}
		o[u], o[v] = lo[u], lo[v]
		du[u] = hi[u] - lo[u]
		dv[v] = hi[v] - lo[v]

		ox, oy, oz := rot(o[0], o[1], o[2])
		ux, uy, uz := rot(du[0], du[1], du[2])
		vx, vy, vz := rot(dv[0], dv[1], dv[2])
		nx, ny, nz := rot(nm[0], nm[1], nm[2])
		// Back-face cull against the face centre (perspective-correct).
		mx, my, mz := ox+(ux+vx)/2, oy+(uy+vy)/2, oz+(uz+vz)/2+cubeDist
		if nx*mx+ny*my+nz*mz >= 0 {
			continue
		}
		l := max((nx*lx+ny*ly+nz*lz)/ln, 0)
		// Brightness is the face's band first, the light second.
		shade := (0.3 + 0.7*l) * (0.25 + 0.75*s.group[f])
		li := uint8(min(shade*top+lift, top))

		for j := 0; j <= n; j++ {
			tv := float64(j) / float64(n)
			bx, by, bz := ox+vx*tv, oy+vy*tv, oz+vz*tv+cubeDist
			for i := 0; i <= n; i++ {
				tu := float64(i) / float64(n)
				z := bz + uz*tu
				if z <= 0.1 {
					continue
				}
				ooz := 1 / z
				px := cx + k*ooz*(bx+ux*tu)*2 // doubled: cells are ~1:2
				py := cy - k*ooz*(by+uy*tu)   // model y is up
				if px < 0 || py < 0 || px >= fc || py >= fr {
					continue
				}
				idx := int(py)*s.cols + int(px)
				if ooz <= s.zbuf[idx] {
					continue
				}
				s.zbuf[idx] = ooz
				s.lum[idx] = li
			}
		}
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
