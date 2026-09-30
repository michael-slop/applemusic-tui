package fx

// Spectrum shaping: the torus's trick, for any effect.
//
// The torus gives every one of the 32 bands its own slice of the ring and
// makes the tube there swell with that band. The general form: pick the axis
// along which an effect naturally shows position (columns, an angle), and at
// each position s in 0..1 read
//
//	deviation(s) = React[band at s] - 0.5
//
// which is 0 when that band is at its normal level for this song, up to +0.5
// when it is well above, down to -0.5 when well below. An effect adds
// strength x deviation(s) to whatever it draws there (flame height, wall
// radius, swirl depth). React is already scaled by amtui's reactivity slider,
// and every band rests at 0.5 when paused or at reactivity 0, so the effect
// is exactly its panefx original at rest.
//
// Mirroring folds the axis about its centre: bass in the middle, treble at
// both ends, so the picture is symmetric.

// Spectrum returns the deviation of the band under position s (0..1) along
// an effect's axis: 0 at rest, -0.5..+0.5 with the music. Neighbouring bands
// are blended so the shape has no steps. Paused, it is always 0.
func (a Audio) Spectrum(s float64, mirror bool) float64 {
	if !a.Playing {
		return 0
	}
	if mirror {
		s = 2*s - 1
		if s < 0 {
			s = -s
		}
	}
	x := min(max(s, 0), 1) * float64(len(a.React)-1)
	i := int(x)
	if i >= len(a.React)-1 {
		return a.React[len(a.React)-1] - 0.5
	}
	f := x - float64(i)
	return a.React[i]*(1-f) + a.React[i+1]*f - 0.5
}

// SpectrumRow fills dst[i] with Spectrum at the centre of cell i of len(dst)
// cells along an axis — the per-frame precompute an effect keeps so Cell and
// its simulation stay O(1).
func (a Audio) SpectrumRow(dst []float64, mirror bool) {
	n := len(dst)
	for i := range dst {
		dst[i] = a.Spectrum((float64(i)+0.5)/float64(n), mirror)
	}
}

// BandAt is the torus's input at position s (0..1) along an effect's axis: the
// reactive level of the band there, 0..1 (0.5 = normal for this song),
// neighbour-blended. Unlike Spectrum it is not gated on Playing: the host
// already lets React decay to 0 when the music stops, which is what makes
// silence a resting silhouette.
func (a Audio) BandAt(s float64, mirror bool) float64 {
	if mirror {
		s = 2*s - 1
		if s < 0 {
			s = -s
		}
	}
	x := min(max(s, 0), 1) * float64(len(a.React)-1)
	i := int(x)
	if i >= len(a.React)-1 {
		return a.React[len(a.React)-1]
	}
	f := x - float64(i)
	return a.React[i]*(1-f) + a.React[i+1]*f
}

// History keeps the last N spectra (32 reactive levels each), newest first,
// for effects that show the song's recent past: a waterfall, receding rings.
type History struct {
	rows [][32]float64
	head int
	n    int
}

// NewHistory holds up to depth spectra.
func NewHistory(depth int) *History {
	return &History{rows: make([][32]float64, max(depth, 1))}
}

// Push adds the newest spectrum.
func (h *History) Push(s [32]float64) {
	h.head = (h.head + 1) % len(h.rows)
	h.rows[h.head] = s
	h.n = min(h.n+1, len(h.rows))
}

// At returns the spectrum age steps ago (0 = newest); older than recorded
// reads as silence.
func (h *History) At(age int) [32]float64 {
	if age < 0 || age >= h.n {
		return [32]float64{}
	}
	return h.rows[(h.head-age+len(h.rows))%len(h.rows)]
}

// Depth is how many spectra it can hold.
func (h *History) Depth() int { return len(h.rows) }

// BandAt is History.At(age) sampled like Audio.BandAt.
func (h *History) BandAt(age int, s float64, mirror bool) float64 {
	a := Audio{React: h.At(age)}
	return a.BandAt(s, mirror)
}
