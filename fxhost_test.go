package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/k1y0miiii/applemusic-tui/fx"
)

// probeFx is a stand-in effect that records what the host does to it.
type probeFx struct {
	cols, rows int
	steps      int
	pal        fx.Palette
	lastKick   float64
}

func (p *probeFx) Name() string             { return "zz-probe" }
func (p *probeFx) Resize(c, r int)          { p.cols, p.rows = c, r }
func (p *probeFx) SetPalette(pl fx.Palette) { p.pal = pl }
func (p *probeFx) Step(a fx.Audio)          { p.steps++; p.lastKick = a.Kick }
func (p *probeFx) Cell(c, r int) (rune, fx.RGB, bool) {
	if c < 0 || r < 0 || c >= p.cols || r >= p.rows {
		return 0, fx.RGB{}, false
	}
	return '@', p.pal.Accent, true
}

var lastProbe *probeFx

func init() {
	fx.Register("zz-probe", 1_000_000, func() fx.Effect { lastProbe = &probeFx{}; return lastProbe })
}

func TestVCyclesIntoFxEffectsAndRendersThem(t *testing.T) {
	t.Setenv("AMTUI_CONFIG_DIR", t.TempDir())
	applyTheme(themes[0])
	m := model{w: 120, h: 35, phase: phaseReady, st: demoState(), vizLive: true, vizSource: "PIPEWIRE", reactivity: 0.5}
	list := vizModeList()
	if list[len(list)-1] != "zz-probe" {
		t.Fatalf("registered effects should follow the built-ins: %v", list)
	}
	for m.fxName() != "zz-probe" {
		m = press(t, m, "v")
		if m.vizMode == 0 {
			t.Fatal("cycled round without reaching the effect")
		}
	}
	if loadVizMode() != len(list)-1 {
		t.Fatal("the effect should be saved by name and reload")
	}
	// The app's order: a tick creates the effect, the next draw sizes it,
	// later ticks step it.
	m.stepFrame(m.lastFrame)
	open := m.View()
	m.stepFrame(m.lastFrame)
	if lastProbe == nil || lastProbe.cols == 0 || lastProbe.steps < 1 {
		t.Fatalf("effect should be resized and stepped: %+v", lastProbe)
	}
	if !strings.Contains(m.View(), "@") || !strings.Contains(open, "ZZ-PROBE") {
		t.Fatal("the effect should draw in the panel, named in the title")
	}
	if got, want := len(strings.Split(m.View(), "\n")), len(strings.Split(open, "\n")); got != want {
		t.Fatalf("frame height changed: %d vs %d", got, want)
	}
	// A theme change reaches the effect.
	before := lastProbe.pal.Accent
	applyTheme(*themeByName("slop"))
	m.stepFrame(m.lastFrame)
	if lastProbe.pal.Accent == before {
		t.Fatal("the effect should be repainted with the new theme")
	}
	// V goes back.
	m = press(t, m, "V")
	if m.fxName() == "zz-probe" {
		t.Fatal("V should step backwards")
	}
	_ = lipgloss.Width
}
