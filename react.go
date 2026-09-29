package main

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Why the shapes barely moved: the analyzer maps each band onto a FIXED
// -75..-12 dBFS window (visualizer/analyzer.go). That is the right scale for
// the bars -- an honest level meter, quiet songs look quiet -- but on a loud
// modern master every band sits at 0.5-0.95 and wobbles by a standard
// deviation of 0.02-0.12 (measured 2026-09-29 with bench/bandprobe). The torus
// maps that onto a tube radius of 0.55-1.40, so the music moved the surface by
// a few percent: less than one character cell at terminal resolution. The low
// bass even pinned against the ceiling (0.96 mean at 60 Hz).
//
// reactive re-expresses each band against ITS OWN recent behaviour: how far it
// is from its running mean, in units of its running spread. A band that is
// loud but steady reads as calm; one that jumps reads as a jump, whatever the
// mastering. The shapes (torus, sphere, the panefx effects) use this; the bars
// keep the absolute scale.

const (
	reactMemory = 1.0 / 60 // mean/spread follow over ~2 s at 30 fps
	reactGain   = 2.5      // spreads to span from centre to edge
	reactFloor  = 0.015    // minimum spread: silence must not amplify noise
	reactQuiet  = 0.03     // below this band level the output relaxes to 0
)

type reactive struct {
	mean, dev [32]float64
	out       [32]float64
	primed    bool
}

// update folds one frame of absolute bands (0..1) in and returns the reactive
// bands (0..1, resting near 0.5 for a steady signal).
func (r *reactive) update(b [32]float64) [32]float64 {
	if !r.primed {
		r.mean, r.primed = b, true
		for i := range r.dev {
			r.dev[i] = reactFloor * 2
		}
	}
	for i, v := range b {
		d := v - r.mean[i]
		r.mean[i] += d * reactMemory
		r.dev[i] += (math.Abs(d) - r.dev[i]) * reactMemory
		if v < reactQuiet {
			r.out[i] *= 0.85 // silence: fall away rather than chase the noise floor
			continue
		}
		z := d / (reactGain * max(r.dev[i], reactFloor))
		r.out[i] = min(1, max(0, 0.5+z))
	}
	return r.out
}

// level is the overall loudness of absolute bands, 0..1.
func bandsMean(b [32]float64) float64 {
	var s float64
	for _, v := range b {
		s += v
	}
	return s / float64(len(b))
}

// audioFeatures is what an animated visualizer gets each frame.
type audioFeatures struct {
	bands   [32]float64 // absolute, 0..1 (the analyzer's scale)
	react   [32]float64 // reactive, 0..1, ~0.5 at rest
	level   float64     // mean absolute band level
	bass    float64     // reactive, bands 0-5 (~25-250 Hz)
	mid     float64     // reactive, bands 6-19
	treble  float64     // reactive, bands 20-31
	kick    float64     // beat pulse from bassKick, 0..1
	playing bool
}

func rangeMean(b [32]float64, lo, hi int) float64 {
	var s float64
	for i := lo; i <= hi; i++ {
		s += b[i]
	}
	return s / float64(hi-lo+1)
}

func (m *model) features() audioFeatures {
	return audioFeatures{
		bands:   m.vizBands,
		react:   m.shapeBands(),
		level:   bandsMean(m.vizBands),
		bass:    rangeMean(m.shapeBands(), 0, 5),
		mid:     rangeMean(m.shapeBands(), 6, 19),
		treble:  rangeMean(m.shapeBands(), 20, 31),
		kick:    m.shapeKick(),
		playing: m.st.Playing,
	}
}

// --- the reactivity slider ----------------------------------------------
//
// One number scales how hard every animated visualizer answers the music:
// the per-band reactive swing AND the beat kick. 0 leaves only the idle
// motion, 0.5 is the measured default, 1 doubles the swing. The bars are an
// EQ and ignore it: they always show the analyzer's absolute levels.

const defaultReactivity = 0.5

// reactScale maps the slider to a swing multiplier: 0 -> 0, 0.5 -> 1, 1 -> 2.
func (m model) reactScale() float64 { return 2 * min(max(m.reactivity, 0), 1) }

// shapeBands is what the animated visualizers deform by: the reactive signal
// scaled by the slider, or (visualizer.reactive = false) the analyzer's
// absolute levels, which is the original behaviour.
func (m model) shapeBands() [32]float64 {
	if !configBool(m.cfg, "visualizer.reactive", true) {
		return m.vizBands
	}
	k := m.reactScale()
	var out [32]float64
	for i, v := range m.vizReact.out {
		out[i] = min(1, max(0, 0.5+(v-0.5)*k))
	}
	return out
}

// shapeKick is the beat pulse scaled by the slider.
func (m model) shapeKick() float64 { return min(1, m.orbKick*m.reactScale()) }

func reactivityFile() string {
	d := configDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "reactivity")
}

// loadReactivity reads the saved slider (0-100), then the config's
// visualizer.reactivity, then the default.
func loadReactivity(cfg map[string]string) float64 {
	pct := configInt(cfg, "visualizer.reactivity", int(defaultReactivity*100))
	if p := reactivityFile(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				pct = n
			}
		}
	}
	return float64(min(max(pct, 0), 100)) / 100
}

func saveReactivity(r float64) {
	p := reactivityFile()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(strconv.Itoa(int(math.Round(r*100)))+"\n"), 0o644)
}

// reactivitySlider renders the setting as ten cells, e.g. "▮▮▮▮▮▯▯▯▯▯ 50%".
func reactivitySlider(r float64) string {
	n := int(math.Round(r * 10))
	return strings.Repeat("▮", n) + strings.Repeat("▯", 10-n) + " " + strconv.Itoa(int(math.Round(r*100))) + "%"
}
