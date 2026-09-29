package main

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// go test -run XXX -bench Frame -benchmem : one full frame (step + view) at
// the user's 76x62 terminal, per visualizer mode.
func BenchmarkFrameTorus(b *testing.B) { benchFrame(b, vizTorus) }
func BenchmarkFrameBars(b *testing.B)  { benchFrame(b, vizBars) }

func benchFrame(b *testing.B, mode int) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	applyTheme(*themeByName("slop"))
	m := model{w: 76, h: 62, phase: phaseReady, st: demoState(), recent: demoRecent(7),
		vizLive: true, vizSource: "PIPEWIRE", reactivity: 0.5, vizMode: mode}
	now := time.Now()
	m.stepFrame(now)
	_ = m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.vizTargets[i%32] = float64(i%10) / 10
		m.stepFrame(now)
		_ = m.View()
	}
}
