package main

import (
	"testing"
	"time"

	"github.com/k1y0miiii/applemusic-tui/visualizer"
)

// flatBands is a loud, steady spectrum: bassKick reads no beat in it.
func flatBands() [32]float64 {
	var b [32]float64
	for i := range b {
		b[i] = 0.7
	}
	return b
}

func onsetModel(live bool, cfg map[string]string) model {
	m := model{phase: phaseReady, st: demoState(), reactivity: defaultReactivity, cfg: cfg,
		vizLive: live, vizBands: flatBands(), vizTargets: flatBands()}
	// bassKick settled on this spectrum, so on its own it reads no beat.
	m.orbKickBase = bassLevel(flatBands())
	m.vizOnsets = visualizer.Onsets{Kick: 1, Hat: 0.8}
	return m
}

func TestLiveKickAndHatComeFromTheOnsetDetector(t *testing.T) {
	m := onsetModel(true, nil)
	m.stepFrame(time.Now())
	if m.orbKick != 1 {
		t.Fatalf("live kick = %.2f, want the onset pulse 1 (the flat bands give bassKick nothing)", m.orbKick)
	}
	if got := m.features().hat; got < 0.79 || got > 0.81 {
		t.Fatalf("live hat = %.2f, want the onset pulse 0.8 at default reactivity", got)
	}
	m.reactivity = 0
	if m.shapeKick() != 0 || m.shapeHat() != 0 {
		t.Fatal("reactivity 0 must silence both pulses")
	}
}

func TestOnsetsOffOrSimulatedFallsBackToBassKick(t *testing.T) {
	for name, m := range map[string]model{
		"visualizer.onsets = false": onsetModel(true, map[string]string{"visualizer.onsets": "false"}),
		"simulated spectrum":        onsetModel(false, nil),
	} {
		m.stepFrame(time.Now())
		if m.orbKick > 0.5 {
			t.Fatalf("%s: kick %.2f followed the onset pulse, want bassKick's", name, m.orbKick)
		}
		if m.features().hat != 0 {
			t.Fatalf("%s: hat %.2f, want 0", name, m.features().hat)
		}
	}
}
