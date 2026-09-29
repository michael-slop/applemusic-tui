package fx

// fishloop: a stored frame sequence played back and fitted to the panel.
// Ported from panefx's frameplay.rs; the frames are in fishloop_art.go.
//
// The procedural effects compute every cell; this one does not compute
// anything. It holds an artist's frames and its whole job is to put them on a
// grid that is not the shape they were drawn on, at a rate that is not the
// rate they were drawn at.
//
// Fit. Authored canvases are small and their own shape (Fish Loop is 60x30),
// so the frame is MAPPED onto the grid the same way wizardtorch maps its art:
// contain / stretch / cover, with the cell aspect (cols : rows*2) in the maths.
//
// Timing. A piece authored at 12fps must play at 12fps whether the panel
// ticks at 5 or 30. The frame index comes from elapsed TIME (a.DT), not from
// a tick counter, so the animation neither crawls nor sprints.
//
// Sparseness. The frames store only lit cells, and cells the artist left empty
// stay empty -- a painted space is a visible smudge behind a translucent
// terminal.
//
// Colour. The reel's own 12-colour palette is mapped onto the theme ramp by
// luminance: each reel colour's brightness, normalised across the reel, picks
// a point on Palette.At between fpRampLo and fpRampHi. The fish keeps its
// light/dark structure (bright eye and scales, dark-green shadow) and takes
// the theme's hues. panefx's `tint` knob is subsumed by this.
//
// How it hears the music (all at rest at reactivity 0, so playback is the
// panefx original at the authored 12fps):
//   - Overall drive -- Drive of the mean React across all 32 bands, smoothed
//     over ~0.4 s -- speeds playback by up to 75%, so the fish swims faster
//     when the music gets busier. Rate is integrated into a play position, so
//     the swim never stutters or jumps frames.
//   - Kick lifts every colour toward the top of the ramp (by up to 35% of the
//     remaining headroom): the fish brightens on the beat and settles as the
//     kick decays. Only the 12-entry colour table changes; no geometry moves.
//   - Paused: keeps swimming at the authored rate.

import "math"

// fpRampLo/fpRampHi bound where the reel's darkest and brightest colours land
// on the theme ramp. Not 0: the bottom of the ramp is the selection
// background, which would make the shadow cells vanish.
const (
	fpRampLo = 0.25
	fpRampHi = 0.92
)

// fpReel is a stored piece: its canvas, palette, frames and authored rate.
// Each frame is a packed string of 4-byte cells: col, row, glyph, palette
// index.
type fpReel struct {
	cols, rows int
	fps        float64
	palette    []RGB
	frames     []string
}

func fishloopReel() fpReel {
	return fpReel{fishloopCols, fishloopRows, fishloopFPS, fishloopPalette[:], fishloopFrames[:]}
}

func init() {
	Register("fishloop", 70, func() Effect { return newFramePlay("fishloop", fishloopReel()) })
}

// FramePlay plays a stored reel.
type FramePlay struct {
	name       string
	reel       fpReel
	cols, rows int

	// pos is the play position in frames (fractional), kept in [0, frames).
	pos float64

	speed  float64 // playback rate, x the authored fps
	detail float64 // zoom about the centre
	fit    wtFit

	// The current frame rasterised to cols*rows: glyph (0 = empty) and
	// palette index. cell() is asked one cell at a time and must answer in
	// O(1); scanning the sparse list per cell would be quadratic.
	glyph []byte
	ci    []uint8
	// gridFrame is which frame the grid holds (-1: none), so it is only
	// rebuilt when the frame changes.
	gridFrame int

	pal     Palette
	base    []float64 // ramp position per reel colour, from luminance
	colours []RGB     // per Step: base lifted by the kick
	drive   float64
}

func newFramePlay(name string, reel fpReel) *FramePlay {
	f := &FramePlay{
		name:   name,
		reel:   reel,
		speed:  1,
		detail: 1,
		// COVER, not contain. These canvases are small, and contained on a
		// wide grid the fish renders as a thumbnail adrift in black.
		//
		// KNOWN LIMITATION: fit centres the CANVAS, not the drawing. Fish
		// Loop's subject sits off-centre in a mostly empty canvas, so cover
		// enlarges it but also pushes it toward one edge. Auto-cropping to the
		// drawn bounds would fix it, but the bounds move every frame and a
		// per-frame crop makes the subject jitter as it swims. Left as-is
		// deliberately.
		fit:       wtCover,
		gridFrame: -1,
		colours:   make([]RGB, len(reel.palette)),
		base:      make([]float64, len(reel.palette)),
	}
	lum := func(c RGB) float64 { return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B) }
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, c := range reel.palette {
		lo, hi = min(lo, lum(c)), max(hi, lum(c))
	}
	for i, c := range reel.palette {
		t := 1.0
		if hi > lo {
			t = (lum(c) - lo) / (hi - lo)
		}
		f.base[i] = fpRampLo + (fpRampHi-fpRampLo)*t
	}
	return f
}

// Name implements Effect.
func (f *FramePlay) Name() string { return f.name }

// SetPalette implements Effect.
func (f *FramePlay) SetPalette(p Palette) {
	f.pal = p
	f.recolour(0)
}

func (f *FramePlay) recolour(kick float64) {
	for i, t := range f.base {
		f.colours[i] = f.pal.At(t + (1-t)*0.35*kick)
	}
}

// Resize implements Effect.
func (f *FramePlay) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == f.cols && rows == f.rows && f.glyph != nil {
		return
	}
	f.cols, f.rows = cols, rows
	// Force a rebuild: the grid is sized to the panel.
	f.gridFrame = -1
}

// frameIndex is which frame is showing now.
func (f *FramePlay) frameIndex() int {
	n := max(len(f.reel.frames), 1)
	return int(f.pos) % n
}

// Step implements Effect.
func (f *FramePlay) Step(a Audio) {
	dt := min(max(a.DT, 0), 0.25)
	var react float64
	for _, v := range a.React {
		react += v
	}
	f.drive += (Drive(react/float64(len(a.React))) - f.drive) * min(1, dt*2.5)

	n := float64(max(len(f.reel.frames), 1))
	f.pos = math.Mod(f.pos+dt*f.reel.fps*max(f.speed, 0.01)*(1+0.75*f.drive), n)

	if fr := f.frameIndex(); fr != f.gridFrame || len(f.glyph) != f.cols*f.rows {
		f.rasterise(fr)
	}
	f.recolour(min(max(a.Kick, 0), 1))
}

// rasterise draws frame into the grid for the current panel size.
//
// Art-cell-first rather than panel-cell-first: the art is sparse, so walking
// the lit cells and scattering them is proportional to what was drawn, while
// sampling per panel cell would be proportional to the screen.
func (f *FramePlay) rasterise(frame int) {
	n := f.cols * f.rows
	if len(f.glyph) != n {
		f.glyph, f.ci = make([]byte, n), make([]uint8, n)
	} else {
		clear(f.glyph)
	}
	f.gridFrame = frame
	if n == 0 || frame >= len(f.reel.frames) {
		return
	}
	cells := f.reel.frames[frame]
	for k := 0; k+4 <= len(cells); k += 4 {
		acol, arow, ch, ci := float64(cells[k]), float64(cells[k+1]), cells[k+2], cells[k+3]
		if int(ci) >= len(f.reel.palette) {
			continue
		}
		// The inverse of the sampling map, applied to the art cell's corners
		// so a scaled-up drawing FILLS the panel instead of leaving gaps
		// between scattered points.
		y0, x0 := f.artToPanel(arow, acol)
		y1, x1 := f.artToPanel(arow+1, acol+1)
		cs, ce := min(x0, x1), max(x0, x1)
		rs, re := min(y0, y1), max(y0, y1)
		// At least one cell, so a shrunk drawing does not vanish.
		cLo := int(max(math.Floor(cs), 0))
		cHi := min(max(int(math.Ceil(ce)), cLo+1), f.cols)
		rLo := int(max(math.Floor(rs), 0))
		rHi := min(max(int(math.Ceil(re)), rLo+1), f.rows)
		for rr := rLo; rr < rHi; rr++ {
			for cc := cLo; cc < cHi; cc++ {
				f.glyph[rr*f.cols+cc] = ch
				f.ci[rr*f.cols+cc] = ci
			}
		}
	}
}

// artToPanel maps an art coordinate (row, col) to a panel coordinate.
func (f *FramePlay) artToPanel(arow, acol float64) (float64, float64) {
	gw, gh := float64(max(f.cols, 1)), float64(max(f.rows, 1))
	detail := max(f.detail, 0.05)
	sv := arow / float64(max(f.reel.rows, 1))
	su := acol / float64(max(f.reel.cols, 1))
	su = (su-0.5)*detail + 0.5
	sv = (sv-0.5)*detail + 0.5
	if f.fit != wtStretch {
		panelAspect := gw / (gh * 2)
		artAspect := float64(f.reel.cols) / (float64(f.reel.rows) * 2)
		wider := panelAspect > artAspect
		sx, sy := 1.0, artAspect/panelAspect
		if (f.fit == wtContain) == wider {
			sx, sy = panelAspect/artAspect, 1
		}
		su = (su-0.5)/sx + 0.5
		sv = (sv-0.5)/sy + 0.5
	}
	return sv * gh, su * gw
}

// Cell implements Effect.
func (f *FramePlay) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= f.cols || row >= f.rows {
		return 0, RGB{}, false
	}
	i := row*f.cols + col
	if i >= len(f.glyph) || f.glyph[i] == 0 {
		return 0, RGB{}, false
	}
	return rune(f.glyph[i]), f.colours[f.ci[i]], true
}
