package fx

import (
	"math"
	"testing"
)

func builtTunnel(cols, rows int) *tunnel {
	t := newTunnel().(*tunnel)
	t.SetPalette(testPalette)
	t.Resize(cols, rows)
	return t
}

func TestTunnelCentreIsNotADivisionByZero(t *testing.T) {
	// radius 0 gives infinite depth; the sample must decline rather than
	// produce a NaN that indexes the ramp.
	tn := builtTunnel(80, 40)
	if _, _, ok := tn.sample(40, 20); ok {
		t.Fatal("the centre cell should have no sample")
	}
}

func TestTunnelBrightnessStaysInRange(t *testing.T) {
	tn := builtTunnel(80, 40)
	for i := range 30 {
		tn.Step(loud(i))
		for r := range tn.rows {
			for c := range tn.cols {
				if b, d, ok := tn.sample(c, r); ok {
					if b < 0 || b > 1 {
						t.Fatalf("brightness %v", b)
					}
					if math.IsInf(d, 0) || math.IsNaN(d) || d <= 0 {
						t.Fatalf("depth %v", d)
					}
				}
			}
		}
	}
}

func TestTunnelFarCellsAreDimmerThanNearOnes(t *testing.T) {
	// The whole depth cue: near the centre is FAR (small radius, large depth)
	// and must be dimmer than the edge.
	tn := builtTunnel(80, 40)
	nearEdge, farCentre := 0.0, 1.0
	for r := range tn.rows {
		for c := range tn.cols {
			if b, d, ok := tn.sample(c, r); ok {
				if d < 1 {
					nearEdge = max(nearEdge, b)
				}
				if d > 8 {
					farCentre = min(farCentre, b)
				}
			}
		}
	}
	if nearEdge <= farCentre {
		t.Fatalf("%v !> %v", nearEdge, farCentre)
	}
}

func TestTunnelPhasesWrapOnATexturePeriod(t *testing.T) {
	tn := builtTunnel(80, 40)
	for i := range 5000 {
		tn.Step(loud(i))
		if tn.travel < 0 || tn.travel >= 2 || tn.spinPh < 0 || tn.spinPh >= 2 {
			t.Fatalf("phase escaped: %v %v", tn.travel, tn.spinPh)
		}
	}
}

func TestTunnelIsPacedByTimeNotFrameCount(t *testing.T) {
	fast, slow := builtTunnel(20, 10), builtTunnel(20, 10)
	a := silence()
	a.DT = 0.02
	for range 50 {
		fast.Step(a)
	}
	a.DT = 0.1
	for range 10 {
		slow.Step(a)
	}
	if math.Abs(fast.travel-slow.travel) > 1e-9 || math.Abs(fast.travel-0.4) > 1e-9 {
		t.Fatalf("travel %v / %v, want 0.4 after 1 s", fast.travel, slow.travel)
	}
}

func TestTunnelKickSurgesForward(t *testing.T) {
	calm, hit := builtTunnel(40, 20), builtTunnel(40, 20)
	a := silence()
	a.Playing = true
	calm.Step(a)
	a.Kick = 1
	for range 3 {
		hit.Step(a)
	}
	a.Kick = 0
	for range 2 {
		calm.Step(a)
	}
	if hit.travel <= calm.travel {
		t.Fatalf("a kick should push the tunnel forward: %v !> %v", hit.travel, calm.travel)
	}
}
