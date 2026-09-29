package main

import "testing"

func fill(v float64) (b [32]float64) {
	for i := range b {
		b[i] = v
	}
	return
}

func TestReactiveRestsNearMiddleForSteadyLoudSignal(t *testing.T) {
	var r reactive
	var out [32]float64
	for range 300 { // 10 s of a loud, flat master
		out = r.update(fill(0.9))
	}
	if out[0] < 0.4 || out[0] > 0.6 {
		t.Fatalf("steady loud band should rest near 0.5, got %.2f", out[0])
	}
}

func TestReactiveMagnifiesAJumpTheAbsoluteScaleHides(t *testing.T) {
	var r reactive
	for i := range 300 {
		v := 0.85
		if i%2 == 0 {
			v = 0.87 // the mix breathing: a 0.02 wobble
		}
		r.update(fill(v))
	}
	out := r.update(fill(0.95)) // a hit only 0.08 above the level
	if out[0] < 0.9 {
		t.Fatalf("a hit above a steady level should read near the top, got %.2f", out[0])
	}
}

func TestReactiveFallsQuietInSilence(t *testing.T) {
	var r reactive
	for range 100 {
		r.update(fill(0.8))
	}
	var out [32]float64
	for range 60 {
		out = r.update(fill(0))
	}
	if out[0] > 0.05 {
		t.Fatalf("silence should settle near 0, got %.2f", out[0])
	}
}

func TestReactivitySliderScalesTheSwing(t *testing.T) {
	m := model{}
	m.vizReact.out = fill(0.9) // a band well above its running mean
	m.orbKick = 0.4
	m.reactivity = 0
	if b := m.shapeBands(); b[0] != 0.5 || m.shapeKick() != 0 {
		t.Fatalf("reactivity 0 should flatten to rest: band %.2f kick %.2f", b[0], m.shapeKick())
	}
	m.reactivity = 0.5
	if b := m.shapeBands(); b[0] < 0.899 || b[0] > 0.901 {
		t.Fatalf("reactivity 0.5 should be the unscaled signal, got %.2f", b[0])
	}
	m.reactivity = 1
	if b := m.shapeBands(); b[0] != 1 || m.shapeKick() < 0.79 {
		t.Fatalf("reactivity 1 should double the swing: band %.2f kick %.2f", b[0], m.shapeKick())
	}
}

func TestReactivityKeysStepPersistAndTitle(t *testing.T) {
	t.Setenv("AMTUI_CONFIG_DIR", t.TempDir())
	m := model{w: 120, h: 35, phase: phaseReady, st: demoState(), reactivity: 0.5, vizMode: vizTorus, vizLive: true, vizSource: "PIPEWIRE"}
	m = press(t, m, "]", "]", "]", "]", "]", "]") // clamps at 100%
	if m.reactivity != 1 || loadReactivity(nil) != 1 {
		t.Fatalf("] should step to 100%% and persist, got %.2f / %.2f", m.reactivity, loadReactivity(nil))
	}
	m = press(t, m, "[", "[", "[")
	if m.reactivity != 0.7 {
		t.Fatalf("[ should step down by 10%%, got %.2f", m.reactivity)
	}
	if got := m.visualizerTitle(); got != "VISUALIZER · LIVE · PIPEWIRE · REACT 70%" {
		t.Fatalf("title = %q", got)
	}
	m.vizMode = vizBars
	if got := m.visualizerTitle(); got != "VISUALIZER · LIVE · PIPEWIRE" {
		t.Fatalf("the bars are an EQ and should not show the slider, got %q", got)
	}
}
