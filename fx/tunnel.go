package fx

// Tunnel: the song's recent past as rings rushing away from you.
//
// Seen head-on, the picture is a set of concentric rings round the centre.
// Every ring is ONE spectrum, drawn with the torus's own formula: where the
// torus makes its tube radius 0.55+0.85*band at each slice of the ring, a
// ring here makes its OUTLINE radius
//
//	r(θ) = base(depth) × (0.55 + 0.85 × level(θ)) / 1.4
//
// so a ring whose bands are all up is a full circle at its base size, one at
// rest is a small one, and a band that jumps pushes out a bulge where it lives.
// The 32 bands wrap round the circle mirrored left/right: bass at the bottom,
// treble at the top, the same on both sides (θ -> s = acos(dy/r)/π).
//
// Depth is time. The rings sit at fixed depths in perspective (base =
// R / (1 + j·tnlPersp)); ring j shows the spectrum from j·tnlStep seconds of
// music ago, read from a History and interpolated between its entries, so the
// nearest (largest) ring is the live spectrum and the song's last ~1.6 s
// recede into the distance: a bass hit bulges the rim, then its bulge travels
// down the tunnel ring by ring, shrinking into the centre. Time runs on DT,
// which amtui already scales to the music's pace (4% when silent), so the
// travel follows the music. That flow of history is the only free motion
// (FreeMotion "scroll"): a steady spectrum gives identical rings, a steady
// nested set, and nothing moves.
//
// The geometry never moves, only what the rings carry. (Rings that slid
// inward themselves were tried: a nested set zooming through a grid of cells
// re-draws most of its cells every frame and its weight saws with every new
// ring, even on a steady or silent spectrum.)
//
// Cells are ~1:2, so y is doubled before the radius is taken; Step rasterises
// every outline into a cell buffer, and Cell is a lookup.
//
// How it hears the music:
//   - React (the per-band reactive level, slider-scaled) is the shape: each
//     band owns a slice of every ring's outline, sets its radius there, and
//     brightens it.
//   - Depth is the song's recent past: the History advances every tnlStep of
//     DT, so the rings' contents travel at the music's pace.
//   - Kick is a global pulse: every ring flares outward a little and
//     brightens, then settles with the kick.
//   - Silence: React decays to 0, so every ring becomes a small faint resting
//     circle (the thin torus); once the last of the music has receded (at
//     the silent pace) the picture holds still.

import "math"

const (
	tnlRings = 20   // the live spectrum plus 19 from the past
	tnlStep  = 0.08 // seconds of music between rings
	tnlBins  = 64   // outline samples from floor (s=0) to ceiling (s=1)
	tnlPersp = 0.3  // ring j sits at depth 1 + j·tnlPersp: the far ring is R/6.7
	tnlFit   = 0.96 // R: the nearest ring at full level just fits the panel
	tnlFog   = 0.7  // brightness falls as nearness^tnlFog
	tnlFlare = 0.08 // how far a kick pushes every ring out
	tnlGlow  = 0.3  // how much a kick brightens every ring
	tnlDim   = 0.04 // brightness below which a ring is not drawn
)

// tnlRamp is the stroke glyph by brightness, dimmest first.
var tnlRamp = []rune(".:-=+*#%@")

// tnlColourSteps is how many distinct colours the theme ramp is sampled into:
// Render emits one escape per colour run, so a small table keeps runs long.
const tnlColourSteps = 16

func tnlLUT(p Palette) (lut [tnlColourSteps]RGB) {
	for i := range lut {
		lut[i] = p.At(float64(i) / float64(tnlColourSteps-1))
	}
	return lut
}

func tnlLUTAt(lut *[tnlColourSteps]RGB, t float64) RGB {
	i := int(t*float64(tnlColourSteps-1) + 0.5)
	return lut[min(max(i, 0), tnlColourSteps-1)]
}

// tnlGlide eases cur toward target with time constant tau seconds.
func tnlGlide(cur, target, dt, tau float64) float64 {
	return cur + (target-cur)*(1-math.Exp(-dt/tau))
}

// tnlDT is the frame's elapsed time, clamped so a stalled frame cannot fling
// the rings forward.
func tnlDT(a Audio) float64 {
	if !(a.DT > 0) {
		return 0
	}
	return min(a.DT, 0.25)
}

// tnlBand is Audio.BandAt(s, false) for a stored spectrum.
func tnlBand(sp *[32]float64, s float64) float64 {
	x := min(max(s, 0), 1) * 31
	i := int(x)
	if i >= 31 {
		return sp[31]
	}
	f := x - float64(i)
	return sp[i]*(1-f) + sp[i+1]*f
}

type tnlCell struct {
	ch rune
	c  RGB
	ok bool
}

type tunnel struct {
	cols, rows int

	hist *History
	acc  float64 // seconds of music since the last push
	kick float64 // smoothed
	lut  [tnlColourSteps]RGB

	// Per-cell geometry, fixed per size: the cell's radial extent [lo, hi]
	// about the centre (x units, y doubled) and the outline bins it spans.
	lo, hi []float64
	b0, b1 []uint8

	// Per-frame: each ring's outline radius and level per bin, the range of
	// its radius (a quick reject), and its brightness by nearness.
	outline    [tnlRings][tnlBins]float64
	level      [tnlRings][tnlBins]float64
	rlo, rhi   [tnlRings]float64
	brightness [tnlRings]float64

	cells []tnlCell
}

func newTunnel() Effect { return &tunnel{hist: NewHistory(tnlRings)} }

func init() { Register("tunnel", 31, newTunnel) }

func (t *tunnel) Name() string { return "tunnel" }

// FreeMotion: the recession of the song's history toward the centre.
func (t *tunnel) FreeMotion() string { return "scroll" }

func (t *tunnel) SetPalette(p Palette) { t.lut = tnlLUT(p) }

// tnlS is the position round a ring for an offset from the centre: 0 at the
// floor, 1 at the ceiling, the same left and right. dy grows downward.
func tnlS(dx, dy float64) float64 {
	r := math.Hypot(dx, dy)
	if r < 1e-9 {
		return 0
	}
	return math.Acos(min(max(dy/r, -1), 1)) / math.Pi
}

func (t *tunnel) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == t.cols && rows == t.rows && t.cells != nil {
		return
	}
	t.cols, t.rows = cols, rows
	n := cols * rows
	t.lo, t.hi = make([]float64, n), make([]float64, n)
	t.b0, t.b1 = make([]uint8, n), make([]uint8, n)
	t.cells = make([]tnlCell, n)
	cx, cy := float64(cols)/2, float64(rows)/2
	bin := func(s float64) uint8 { return uint8(min(int(s*tnlBins), tnlBins-1)) }
	for row := range rows {
		for col := range cols {
			i := row*cols + col
			// The cell's footprint in x units: 1 wide, 2 tall.
			x0, x1 := float64(col)-cx, float64(col)+1-cx
			y0, y1 := (float64(row)-cy)*2, (float64(row)+1-cy)*2
			nx := min(max(0, x0), x1) // nearest point to the centre
			ny := min(max(0, y0), y1)
			t.lo[i] = math.Hypot(nx, ny)
			t.hi[i] = math.Hypot(max(-x0, x1), max(-y0, y1))
			if x0 <= 0 && x1 >= 0 && y0 <= 0 && y1 >= 0 {
				t.b0[i], t.b1[i] = 0, tnlBins-1 // the centre cell sees every angle
				continue
			}
			smin, smax := 1.0, 0.0
			pts := [6][2]float64{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}, {0, y0}, {0, y1}}
			np := 4
			if x0 < 0 && x1 > 0 {
				np = 6 // straddles the vertical axis: the floor/ceiling line is inside
			}
			for _, p := range pts[:np] {
				s := tnlS(p[0], p[1])
				smin, smax = min(smin, s), max(smax, s)
			}
			t.b0[i], t.b1[i] = bin(smin), bin(smax)
		}
	}
}

// tnlNearness is ring j's base size relative to the nearest ring.
func tnlNearness(j int) float64 { return 1 / (1 + float64(j)*tnlPersp) }

// past fills sp with the spectrum from j·tnlStep seconds of music ago: the
// live one for j = 0, else interpolated between the two History entries
// either side (entry i was pushed acc + i·tnlStep ago).
func (t *tunnel) past(sp *[32]float64, live *[32]float64, j int) {
	if j == 0 {
		*sp = *live
		return
	}
	f := 1 - t.acc/tnlStep // 0: exactly entry j-1, 1: exactly entry j
	a, b := t.hist.At(j-1), t.hist.At(j)
	for i := range sp {
		sp[i] = a[i]*(1-f) + b[i]*f
	}
}

func (t *tunnel) Step(a Audio) {
	dt := tnlDT(a)
	t.kick = tnlGlide(t.kick, a.Kick, dt, 0.05)
	t.acc += dt
	for t.acc >= tnlStep {
		t.hist.Push(a.React)
		t.acc -= tnlStep
	}

	R := tnlFit * min(float64(t.cols)/2, float64(t.rows)) * (1 + tnlFlare*t.kick)
	var sp [32]float64
	for j := range tnlRings {
		t.past(&sp, &a.React, j)
		base := R * tnlNearness(j)
		t.brightness[j] = math.Pow(tnlNearness(j), tnlFog)
		t.rlo[j], t.rhi[j] = math.Inf(1), 0
		for b := range tnlBins {
			lvl := tnlBand(&sp, (float64(b)+0.5)/tnlBins)
			r := base * (0.55 + 0.85*lvl) / 1.4
			t.level[j][b], t.outline[j][b] = lvl, r
			t.rlo[j], t.rhi[j] = min(t.rlo[j], r), max(t.rhi[j], r)
		}
	}

	// Rasterise: a cell is on ring j's outline when its radial extent meets
	// the outline (a stroke ~1 cell wide) over the angles the cell spans.
	// The nearest ring wins. O(cells x rings) once per frame; Cell is a
	// lookup.
	n := len(tnlRamp)
	for i := range t.cells {
		t.cells[i] = tnlCell{}
		lo, hi, b0, b1 := t.lo[i], t.hi[i], int(t.b0[i]), int(t.b1[i])
		for j := range tnlRings {
			if hi < t.rlo[j] || lo > t.rhi[j] {
				continue
			}
			rmin, rmax, lvl := math.Inf(1), 0.0, 0.0
			for b := b0; b <= b1; b++ {
				r := t.outline[j][b]
				rmin, rmax = min(rmin, r), max(rmax, r)
				lvl = max(lvl, t.level[j][b])
			}
			if hi < rmin || lo > rmax {
				continue
			}
			v := min(1, t.brightness[j]*(0.2+0.8*lvl)+tnlGlow*t.kick)
			if v < tnlDim {
				continue // too faint to draw: let a ring behind show
			}
			t.cells[i] = tnlCell{tnlRamp[min(int(v*float64(n)), n-1)], tnlLUTAt(&t.lut, 0.15+0.85*v), true}
			break
		}
	}
}

func (t *tunnel) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= t.cols || row >= t.rows {
		return 0, RGB{}, false
	}
	c := t.cells[row*t.cols+col]
	return c.ch, c.c, c.ok
}
