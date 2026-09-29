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
