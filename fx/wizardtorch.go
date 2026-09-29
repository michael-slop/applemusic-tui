package fx

// wizardtorch, tgevil, wzfire, raalien: four still ANSI-art pieces, each
// fitted to the panel and rippled like `waves`. Ported from panefx's
// wizardtorch.rs, which animates all four through one module; the art and the
// defaults each drawing needs are data (see wtPieces).
//
// The drawing is fixed -- see wizardtorch_art.go and friends. What moves is
// the SAMPLING. Each frame, every panel cell asks the art "what is at this
// point?" through a slowly swirling displacement, so the whole piece breathes
// and ripples without a single glyph being redrawn or invented.
//
// Fit. The art is 80 columns wide and 128-508 rows tall -- a portrait shape.
// A landscape panel is nothing like that, so the art is MAPPED onto whatever
// grid it is given rather than pasted into the middle of it: contain keeps the
// artist's proportions and letterboxes, stretch fills the panel and lets the
// wizard get wide, cover fills it while keeping proportions and crops.
//
// Detail. Sampling an 80x128 drawing onto a different grid means several art
// cells per panel cell (or the reverse), and nearest-neighbour blocks look like
// a mistake. So the sample is BILINEAR over shade levels -- the art's own 0..4
// shades interpolate to a continuous value which the ramp re-quantises.
//
// The ripple. A smooth per-cell noise field drives a sin/cos displacement
// whose phase advances with time, and the art is sampled at the displaced
// coordinate. Displacing the SAMPLE rather than the image is what keeps every
// feature intact -- the wizard ripples, he does not smear. Amplitude is small
// and measured in ART cells, not panel cells, so the ripple is the same size
// on the drawing whatever grid it is fitted to. The art has a face in it; a
// large warp turns a face into soup.
//
// Colour. panefx lit each piece with one ink colour times the sampled shade
// (quantised to 6 levels per channel). Here the shade instead indexes the
// theme ramp (Palette.At), quantised to wtColourSteps levels: the same
// dark-to-bright structure, recoloured by amtui's colour controller. The
// per-piece ink (bone, amber, pale teal) is therefore dropped.
//
// How it hears the music (all at rest at reactivity 0, so the picture is the
// panefx original):
//   - Bass drive (Drive(a.Bass), smoothed over ~0.2 s) speeds the ripple up
//     to 2.5x and deepens it by up to 60% -- a bass line makes the drawing
//     breathe harder. Speed is integrated into the phase, so it never jumps.
//   - Kick swells the ripple (+50% depth) and lifts the HIGHLIGHTS: a cell's
//     shade v becomes v + 0.3*kick*v^2, so the torch, flames and bright
//     strokes flare toward the top of the ramp (and fuller glyphs) while the
//     shadows barely move. The dark cutoff is judged on the unlifted shade,
//     so the silhouette itself stays put -- it glows, it does not pop.
//   - Paused: the ripple keeps its idle pace.

import "math"

// wtRamp is the shade ramp, dimmest first. The art's own blocks, so a sampled
// value re-quantises into the vocabulary it was drawn in.
var wtRamp = [4]rune{'░', '▒', '▓', '█'}

// wtColourSteps is how many colour levels the shade is quantised to. Few
// enough that neighbouring cells share a colour (the renderer groups runs),
// enough that the bilinear shading still reads as shading.
const wtColourSteps = 10

// wtFit is how the art is mapped onto a panel whose shape is not the art's.
type wtFit int

const (
	// wtContain keeps the art's proportions and letterboxes the rest.
	wtContain wtFit = iota
	// wtStretch fills the panel exactly; the drawing takes the panel's shape.
	wtStretch
	// wtCover keeps proportions and fills the panel, cropping the overflow.
	wtCover
)

// wtPiece is one still art piece and the defaults it wants. The DEFAULTS
// travel with the art because they are properties of the drawing, not
// preferences: raalien is negative art over six times taller than it is wide,
// and the darkcut that suits wizardtorch erases it.
type wtPiece struct {
	name       string
	cols, rows int
	art        []string
	// darkcut: cells at or below this fraction of full shade are not drawn.
	darkcut float64
	fit     wtFit
}

func wtPieces() []wtPiece {
	return []wtPiece{
		// A robed wizard holding a torch above a bank of skulls.
		{"wizardtorch", wizardtorchCols, wizardtorchRows, wizardtorchArt[:], 0.120, wtContain},
		// A horned demon skull in a stone arch, a third eye above, molten
		// drips below. Signed tgFiRE.
		{"tgevil", tgevilCols, tgevilRows, tgevilArt[:], 0.120, wtContain},
		// A winged dragon on the battlement of a stone tower, under
		// blackletter lettering. Densest of the four (79% of cells lit), so it
		// can afford to cull more of the low end without losing the drawing.
		{"wzfire", wzfireCols, wzfireRows, wzfireArt[:], 0.200, wtContain},
		// A Giger biomechanical xenomorph, drawn in NEGATIVE: the hatching is
		// the ink and the creature is the unlit space between it. darkcut is 0
		// because on negative art the background IS the subject, and the usual
		// 0.12 eats the creature. Fit stays CONTAIN: cover on a 6.3:1 scroll
		// crops so hard that a landscape panel shows a scatter of unreadable
		// hatching (panefx: rendered and looked at, not assumed). Contained it
		// is a narrow vertical strip, which is at least the drawing.
		{"raalien", raalienCols, raalienRows, raalienArt[:], 0, wtContain},
	}
}

func init() {
	for i, p := range wtPieces() {
		Register(p.name, 60+i, func() Effect { return newWizardTorch(p) })
	}
}

// wtShade is the shade level of one art byte, 0 (empty) to 4 (full block).
//
// Half blocks count as full: at any fit other than 1:1 a panel cell covers a
// fraction of a glyph anyway, so preserving which HALF was inked buys nothing
// the sampler could express.
var wtShade = func() (t [256]uint8) {
	t['.'], t[':'], t['*'] = 1, 2, 3
	for _, b := range "#TBLR" {
		t[b] = 4
	}
	return t
}()

// wtHash01 is a deterministic hash to [0,1). Same shape as the one in waves.
func wtHash01(seed uint32, x, y int32) float64 {
	h := seed*0x9E3779B9 + uint32(x)*0x85EBCA6B + uint32(y)*0xC2B2AE35
	h ^= h >> 15
	h *= 0x2545F491
	h ^= h >> 13
	return float64(h&0x00FFFFFF) / 16777216.0
}

// wtSmoothField is a smooth low-frequency field over the grid, in [0,1).
// Smoothstep-interpolated over a coarse lattice: cheap, and smooth enough that
// the displacement it drives has no visible grid in it. Built once per resize,
// never per frame.
func wtSmoothField(h, w int, freq float64, seed uint32) []float64 {
	g := max(int(freq), 1) + 2
	lattice := make([]float64, g*g)
	for i := range lattice {
		lattice[i] = wtHash01(seed, int32(i/g), int32(i%g))
	}
	at := func(gy, gx int) float64 { return lattice[(gy%g)*g+(gx%g)] }
	out := make([]float64, w*h)
	for y := range h {
		for x := range w {
			fy := float64(y) / float64(max(h, 1)) * freq
			fx := float64(x) / float64(max(w, 1)) * freq
			y0, x0 := int(math.Floor(fy)), int(math.Floor(fx))
			ty, tx := fy-float64(y0), fx-float64(x0)
			// Smoothstep, so the lattice edges do not show as creases.
			sy := ty * ty * (3 - 2*ty)
			sx := tx * tx * (3 - 2*tx)
			out[y*w+x] = at(y0, x0)*(1-sx)*(1-sy) +
				at(y0, x0+1)*sx*(1-sy) +
				at(y0+1, x0)*(1-sx)*sy +
				at(y0+1, x0+1)*sx*sy
		}
	}
	return out
}

// wtCell is one rendered cell; ch == 0 is empty.
type wtCell struct {
	ch rune
	c  RGB
}

// WizardTorch plays one still art piece.
type WizardTorch struct {
	piece      wtPiece
	cols, rows int

	// t is the ripple phase, 0..1.
	t float64

	speed   float64 // ripple speed, 1 = panefx default
	swirl   float64 // ripple depth
	detail  float64 // art scale about the centre; 1 = fitted
	darkcut float64
	fit     wtFit

	// Per-cell phase offsets of the two displacement fields, stored as
	// sin/cos so a frame needs no trig per cell: sin(p+a) = sin p cos a +
	// cos p sin a. Rebuilt on resize only.
	sinA, cosA, sinB, cosB []float64
	// The panel->art mapping is separable (x depends on col only, y on row
	// only), so it is two small tables rather than a per-cell computation.
	axOf, ayOf []float64

	colours [wtColourSteps + 1]RGB
	grid    []wtCell

	drive float64 // smoothed bass drive
}

func newWizardTorch(p wtPiece) *WizardTorch {
	w := &WizardTorch{
		piece:   p,
		speed:   1,
		swirl:   0.7,
		detail:  1,
		darkcut: p.darkcut,
		fit:     p.fit,
	}
	return w
}

// Name implements Effect.
func (w *WizardTorch) Name() string { return w.piece.name }

// SetPalette implements Effect.
func (w *WizardTorch) SetPalette(p Palette) {
	for i := range w.colours {
		w.colours[i] = p.At(float64(i) / wtColourSteps)
	}
}

// Resize implements Effect.
func (w *WizardTorch) Resize(cols, rows int) {
	cols, rows = max(cols, 0), max(rows, 0)
	if cols == w.cols && rows == w.rows && w.grid != nil {
		return
	}
	w.cols, w.rows = cols, rows
	w.rebuild()
}

func (w *WizardTorch) rebuild() {
	h, wd := max(w.rows, 1), max(w.cols, 1)
	a := wtSmoothField(h, wd, 2.0, 77)
	b := wtSmoothField(h, wd, 2.7, 91)
	n := len(a)
	w.sinA, w.cosA = make([]float64, n), make([]float64, n)
	w.sinB, w.cosB = make([]float64, n), make([]float64, n)
	for i := range n {
		w.sinA[i], w.cosA[i] = math.Sincos(a[i] * 2 * math.Pi)
		w.sinB[i], w.cosB[i] = math.Sincos(b[i] * 2 * math.Pi)
	}
	w.remap()
	w.grid = make([]wtCell, w.cols*w.rows)
}

// remap rebuilds the panel->art tables after a size, fit or detail change.
func (w *WizardTorch) remap() {
	w.axOf = make([]float64, w.cols)
	w.ayOf = make([]float64, w.rows)
	for c := range w.axOf {
		_, w.axOf[c] = w.toArt(float64(c), 0)
	}
	for r := range w.ayOf {
		w.ayOf[r], _ = w.toArt(0, float64(r))
	}
}

// toArt maps a panel cell to a point in art space (row, col), honouring fit
// and detail.
//
// Cells are about twice as tall as they are wide, so a grid's true aspect is
// cols : rows*2. Ignoring that letterboxes on the wrong axis and squashes the
// wizard to half his height.
func (w *WizardTorch) toArt(col, row float64) (float64, float64) {
	gw, gh := float64(max(w.cols, 1)), float64(max(w.rows, 1))
	detail := max(w.detail, 0.05)
	su, sv := col/gw, row/gh
	if w.fit != wtStretch {
		panelAspect := gw / (gh * 2)
		artAspect := float64(w.piece.cols) / (float64(w.piece.rows) * 2)
		wider := panelAspect > artAspect
		// Contain letterboxes on the long axis; cover crops it instead --
		// the same comparison with the branch reversed.
		scaleX, scaleY := 1.0, artAspect/panelAspect
		if (w.fit == wtContain) == wider {
			scaleX, scaleY = panelAspect/artAspect, 1
		}
		su = (su-0.5)*scaleX + 0.5
		sv = (sv-0.5)*scaleY + 0.5
	}
	// Detail zooms about the centre, so turning it resolves the drawing
	// rather than sliding it off the screen.
	su = (su-0.5)/detail + 0.5
	sv = (sv-0.5)/detail + 0.5
	return sv * float64(w.piece.rows), su * float64(w.piece.cols)
}

// artSample is a bilinear shade sample (0..4) of the art at a continuous
// coordinate. Out of bounds reads as empty rather than wrapping: the art is a
// picture with edges, and wrapping puts the flame band right above the
// wizard's hat.
func (w *WizardTorch) artSample(ay, ax float64) float64 {
	y0f, x0f := math.Floor(ay), math.Floor(ax)
	fy, fx := ay-y0f, ax-x0f
	y0, x0 := int(y0f), int(x0f)
	at := func(y, x int) float64 {
		if y < 0 || x < 0 || y >= w.piece.rows || x >= w.piece.cols {
			return 0
		}
		line := w.piece.art[y]
		if x >= len(line) {
			return 0
		}
		return float64(wtShade[line[x]])
	}
	return at(y0, x0)*(1-fx)*(1-fy) +
		at(y0, x0+1)*fx*(1-fy) +
		at(y0+1, x0)*(1-fx)*fy +
		at(y0+1, x0+1)*fx*fy
}

// Step implements Effect.
func (w *WizardTorch) Step(a Audio) {
	dt := min(max(a.DT, 0), 0.25)
	// Smoothed so a bass line eases the ripple faster instead of jerking it.
	w.drive += (Drive(a.Bass) - w.drive) * min(1, dt*5)
	kick := min(max(a.Kick, 0), 1)

	// Time-based, so the ripple keeps its pace at any frame rate.
	w.t = math.Mod(w.t+dt*w.speed*0.10*(1+1.5*w.drive), 1)

	// Amplitude in ART cells, so the ripple is the same size on the drawing
	// whatever grid it happens to be fitted to.
	amp := 0.02 * float64(min(w.piece.cols, w.piece.rows)) * w.swirl * (1 + 0.6*w.drive + 0.5*kick)
	w.render(amp, 0.3*kick)
}

// render fills the grid for the current phase. lift brightens highlights.
func (w *WizardTorch) render(amp, lift float64) {
	if len(w.grid) != w.cols*w.rows {
		w.rebuild()
	}
	sinP, cosP := math.Sincos(2 * math.Pi * w.t)
	pr, pc := float64(w.piece.rows), float64(w.piece.cols)
	for r := range w.rows {
		ay := w.ayOf[r]
		base := r * w.cols
		for c := range w.cols {
			i := base + c
			w.grid[i] = wtCell{}
			// Same displacement as waves: a per-cell noise value sets the
			// phase offset, so neighbouring cells move together and the
			// field ripples instead of shimmering.
			dx := (sinP*w.cosA[i] + cosP*w.sinA[i]) * amp
			dy := (cosP*w.cosB[i] - sinP*w.sinB[i]) * amp
			sy, sx := ay+dy, w.axOf[c]+dx
			// Cheap reject BEFORE the bilinear sample: at contain on a wide
			// panel most cells fall in the letterbox.
			if sy < -1 || sx < -1 || sy >= pr || sx >= pc {
				continue
			}
			v := w.artSample(sy, sx) / 4
			if v <= w.darkcut {
				continue
			}
			v = min(v+lift*v*v, 1)
			idx := min(max(int(math.Ceil(v*float64(len(wtRamp)))), 1), len(wtRamp)) - 1
			w.grid[i] = wtCell{wtRamp[idx], w.colours[int(math.Round(v*wtColourSteps))]}
		}
	}
}

// Cell implements Effect.
func (w *WizardTorch) Cell(col, row int) (rune, RGB, bool) {
	if col < 0 || row < 0 || col >= w.cols || row >= w.rows {
		return 0, RGB{}, false
	}
	i := row*w.cols + col
	if i >= len(w.grid) {
		return 0, RGB{}, false
	}
	g := w.grid[i]
	return g.ch, g.c, g.ch != 0
}
