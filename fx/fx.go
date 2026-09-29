// Package fx holds the panefx ASCII animations, ported from the Rust panefx
// crate (C:\Users\micha\panefx on pHub) to run inside amtui's visualizer
// panel. The shape of the port follows panefx's own AsciiAnimation trait: a
// panel does not know it is showing fire; it owns an Effect, advances it one
// frame, and asks for a glyph + colour per cell.
//
// Two things are new relative to panefx:
//
//   - Audio. Step receives the music (see Audio). At Reactivity 0 every effect
//     must behave like its panefx original; the reactivity slider in amtui
//     scales how hard the music drives it.
//   - Palette. Colours come from amtui's current theme (see Palette), so the
//     colour controller recolours every effect. Effects keep their panefx
//     shapes and ramps' STRUCTURE (dark -> bright), not their hard-coded hues.
package fx

import (
	"math"
	"sort"
	"sync"
)

// RGB is a 24-bit colour.
type RGB struct{ R, G, B uint8 }

// Lerp blends from a to b by t in 0..1.
func Lerp(a, b RGB, t float64) RGB {
	t = min(max(t, 0), 1)
	f := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return RGB{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B)}
}

// Scale multiplies brightness by k (clamped).
func (c RGB) Scale(k float64) RGB {
	f := func(x uint8) uint8 { return uint8(min(255, max(0, math.Round(float64(x)*k)))) }
	return RGB{f(c.R), f(c.G), f(c.B)}
}

// Audio is what an effect hears each frame. Every field is already scaled by
// amtui's reactivity slider: at Reactivity 0, React is all 0.5 and Kick is 0.
type Audio struct {
	Bands      [32]float64 // absolute analyzer levels 0..1 (25 Hz .. 16 kHz, log-spaced)
	React      [32]float64 // per-band change vs its own recent average, 0..1, rests at 0.5
	Level      float64     // mean absolute level 0..1
	Bass       float64     // mean React of bands 0-5 (~25-250 Hz), rests at 0.5
	Mid        float64     // mean React of bands 6-19
	Treble     float64     // mean React of bands 20-31
	Kick       float64     // beat pulse 0..1: jumps on a bass hit, decays over ~0.3 s
	Playing    bool        // false when paused (effects should keep their idle motion)
	Reactivity float64     // the slider itself, 0..1 (0.5 = default)
	DT         float64     // seconds since the previous Step (1/30 at full rate)
}

// Drive is a convenience for effects: how far a rest-at-0.5 value sits above
// rest, 0..1. Drive(a.Bass) is 0 on a steady bass line and ~1 on a hit.
func Drive(v float64) float64 { return min(1, max(0, (v-0.5)*2)) }

// Palette is the current amtui theme, expressed for effects.
type Palette struct {
	Background RGB    // what an "off" cell would look like (effects usually just return ok=false)
	Ramp       [5]RGB // dark -> bright: selection bg, accent lo, accent, accent hi, text
	Accent     RGB
	AccentHi   RGB
	AccentLo   RGB
	Text       RGB
	Dim        RGB
	Faint      RGB
}

// At samples the ramp at t in 0..1 (0 = darkest, 1 = brightest).
func (p Palette) At(t float64) RGB {
	t = min(max(t, 0), 1) * float64(len(p.Ramp)-1)
	i := int(t)
	if i >= len(p.Ramp)-1 {
		return p.Ramp[len(p.Ramp)-1]
	}
	return Lerp(p.Ramp[i], p.Ramp[i+1], t-float64(i))
}

// Effect is one animation. It is the Go shape of panefx's AsciiAnimation.
type Effect interface {
	// Name is the registry name ("flames", "rain", ...).
	Name() string
	// Resize sets the grid in terminal cells. Called before the first Step
	// and whenever the panel changes size; state may be discarded.
	Resize(cols, rows int)
	// SetPalette is called whenever the theme changes (and before the first
	// frame).
	SetPalette(p Palette)
	// Step advances one frame. amtui calls it 30 times a second while the
	// effect is on screen.
	Step(a Audio)
	// Cell returns the glyph and colour at (col, row); ok=false leaves the
	// cell blank. col in [0, cols), row in [0, rows).
	Cell(col, row int) (ch rune, c RGB, ok bool)
}

var (
	regMu    sync.Mutex
	registry = map[string]func() Effect{}
	order    = map[string]int{}
)

// Register adds an effect constructor. rank orders the effect in Names (the
// order `v` cycles through); ties sort by name. Call from an init func.
func Register(name string, rank int, ctor func() Effect) {
	regMu.Lock()
	defer regMu.Unlock()
	registry[name] = ctor
	order[name] = rank
}

// New builds a registered effect, or nil.
func New(name string) Effect {
	regMu.Lock()
	ctor := registry[name]
	regMu.Unlock()
	if ctor == nil {
		return nil
	}
	return ctor()
}

// Names lists registered effects in cycle order.
func Names() []string {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if order[out[i]] != order[out[j]] {
			return order[out[i]] < order[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// Rand is a small deterministic PRNG (xorshift64*), so effects can be seeded
// and tested reproducibly. Not for anything that matters.
type Rand struct{ s uint64 }

// NewRand seeds a generator; seed 0 is replaced with a fixed non-zero value.
func NewRand(seed uint64) *Rand {
	if seed == 0 {
		seed = 0x9E3779B97F4A7C15
	}
	return &Rand{seed}
}

// Uint64 returns the next value.
func (r *Rand) Uint64() uint64 {
	r.s ^= r.s >> 12
	r.s ^= r.s << 25
	r.s ^= r.s >> 27
	return r.s * 2685821657736338717
}

// Intn returns 0..n-1 (n > 0).
func (r *Rand) Intn(n int) int { return int(r.Uint64() % uint64(n)) }

// Float returns 0..1.
func (r *Rand) Float() float64 { return float64(r.Uint64()>>11) / (1 << 53) }
