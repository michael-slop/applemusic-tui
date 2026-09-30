package main

import (
	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/k1y0miiii/applemusic-tui/fx"
)

// Hosting the panefx animations (package fx) in the visualizer panel. The
// built-in modes (bars, torus, sphere) come first; every registered fx effect
// follows, in the registry's order, so `v` cycles through all of them and a
// newly ported effect appears without touching this file.

// vizModeList is every visualizer mode by name, in cycle order.
func vizModeList() []string {
	return append(append([]string{}, vizModeNames[:]...), fx.Names()...)
}

// fxHost owns the running effect. It lives behind a pointer so it survives
// Bubble Tea's model copies (the effect is stateful).
type fxHost struct {
	name       string
	eff        fx.Effect
	cols, rows int
	pal        fx.Palette
	havePal    bool
}

// fxName is the effect the current mode shows, or "" for a built-in mode.
func (m model) fxName() string {
	if m.vizMode < len(vizModeNames) {
		return ""
	}
	list := vizModeList()
	if m.vizMode >= len(list) {
		return ""
	}
	return list[m.vizMode]
}

// ensureFx makes sure the host holds the effect for the current mode.
func (m *model) ensureFx() *fxHost {
	name := m.fxName()
	if name == "" {
		return nil
	}
	if m.fx == nil {
		m.fx = &fxHost{}
	}
	if m.fx.name != name || m.fx.eff == nil {
		m.fx.name, m.fx.eff, m.fx.cols, m.fx.rows, m.fx.havePal = name, fx.New(name), 0, 0, false
	}
	if m.fx.eff == nil {
		return nil
	}
	if p := themePalette(); !m.fx.havePal || p != m.fx.pal {
		m.fx.eff.SetPalette(p)
		m.fx.pal, m.fx.havePal = p, true
	}
	return m.fx
}

// stepFx advances the effect by frames 30 fps frames. Effects clamp a Step to
// 0.25 s, so a long catch-up is fed in steps of at most six frames (0.2 s).
func (m *model) stepFx(frames int) {
	for frames > 0 {
		n := min(frames, 6)
		m.stepFxBy(n)
		frames -= n
	}
}

func (m *model) stepFxBy(frames int) {
	h := m.ensureFx()
	if h == nil || h.cols == 0 {
		return // not drawn yet: Resize happens on the first render
	}
	f := m.features()
	a := fx.Audio{
		Bands: f.bands, React: f.react, Level: f.level,
		Bass: f.bass, Mid: f.mid, Treble: f.treble, Kick: f.kick,
		Playing: f.playing, Reactivity: m.reactivity,
		// Time itself runs at the music's pace: nearly still when it is off.
		DT:   float64(frames) / 30 * m.motionSpeed(),
		Wall: float64(frames) / 30,
	}
	if !configBool(m.cfg, "visualizer.reactive", true) {
		a.Reactivity = 0
	}
	h.eff.Step(a)
}

// fxPanel renders the effect into the panel body.
func (m model) fxPanel(w, h int) string {
	host := m.fx
	if host == nil || host.eff == nil || host.name != m.fxName() {
		return blankRows(w, h)
	}
	if host.cols != w || host.rows != h {
		host.eff.Resize(w, h)
		host.cols, host.rows = w, h
	}
	return fx.Render(host.eff, w, h, lipgloss.ColorProfile())
}

func blankRows(w, h int) string {
	row := make([]byte, max(0, w))
	for i := range row {
		row[i] = ' '
	}
	out := make([]byte, 0, (w+1)*max(0, h))
	for r := 0; r < h; r++ {
		if r > 0 {
			out = append(out, '\n')
		}
		out = append(out, row...)
	}
	return string(out)
}

// themePalette expresses the current theme (the eight globals the colour
// controller edits) for the effects.
func themePalette() fx.Palette {
	c := func(l lipgloss.Color) fx.RGB {
		cc, err := colorful.Hex(string(l))
		if err != nil {
			return fx.RGB{}
		}
		r, g, b := cc.Clamped().RGB255()
		return fx.RGB{R: r, G: g, B: b}
	}
	bg := c(selBg)
	return fx.Palette{
		Background: bg.Scale(0.6),
		Ramp:       [5]fx.RGB{bg, c(accentLo), c(accent), c(accentHi), c(fgBright)},
		Accent:     c(accent), AccentHi: c(accentHi), AccentLo: c(accentLo),
		Text: c(fgBright), Dim: c(fgDim), Faint: c(fgFaint),
	}
}
