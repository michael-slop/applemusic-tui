package fx

import (
	"math"
	"testing"
)

func builtStarfield(cols, rows int) *starfield {
	s := newStarfield().(*starfield)
	s.SetPalette(testPalette)
	s.Resize(cols, rows)
	return s
}

func TestStarfieldIsPopulatedFromTheFirstFrame(t *testing.T) {
	// Spawning every star at the far plane would open on an empty screen
	// followed by a wall of stars arriving together.
	s := builtStarfield(80, 40)
	s.Step(silence())
	first := ptsLit(s, 80, 40)
	if first <= 5 {
		t.Fatalf("the field is empty: %d lit", first)
	}
	for range 40 {
		s.Step(silence())
	}
	if later := ptsLit(s, 80, 40); float64(first) <= float64(later)*0.35 {
		t.Fatalf("first frame %d is far emptier than the steady state %d", first, later)
	}
}

func TestStarfieldCountIsStableAndInFrontOfTheViewer(t *testing.T) {
	// Every star must be respawned, not leaked or lost, and a star at z <= 0
	// would project mirrored behind the camera.
	s := builtStarfield(80, 40)
	want := s.targetCount()
	for i := range 500 {
		s.Step(loud(i))
		for _, st := range s.stars {
			if st.z <= 0 {
				t.Fatalf("star behind the viewer: %v", st.z)
			}
		}
	}
	if len(s.stars) != want {
		t.Fatalf("%d stars, want %d", len(s.stars), want)
	}
}

func TestStarfieldDensityIsAreaRelative(t *testing.T) {
	small, big := builtStarfield(40, 20), builtStarfield(160, 80)
	if big.targetCount() <= small.targetCount()*3 {
		t.Fatalf("%d vs %d", big.targetCount(), small.targetCount())
	}
}

func TestStarfieldResizeRebuildsTheGrid(t *testing.T) {
	s := builtStarfield(80, 40)
	s.Step(silence())
	s.Resize(30, 15)
	s.Step(silence())
	if len(s.grid) != 30*15 {
		t.Fatalf("grid %d, want %d", len(s.grid), 30*15)
	}
}

func TestStarfieldIsPacedByTimeNotFrameCount(t *testing.T) {
	// 1 s of travel must move a star the same distance at either rate.
	fast, slow := builtStarfield(80, 40), builtStarfield(80, 40)
	z0 := fast.stars[0].z
	a := silence()
	a.DT = 0.02
	for range 5 {
		fast.Step(a)
	}
	a.DT = 0.1
	slow.Step(a)
	if d := (z0 - fast.stars[0].z) - (z0 - slow.stars[0].z); math.Abs(d) > 1e-9 {
		t.Fatalf("paced by frames: diff %v", d)
	}
}

func TestStarfieldKickWarps(t *testing.T) {
	calm, hit := builtStarfield(80, 40), builtStarfield(80, 40)
	z0 := calm.stars[3].z
	a := silence()
	a.Playing = true
	calm.Step(a)
	a.Kick = 1
	hit.Step(a)
	if z0-hit.stars[3].z <= z0-calm.stars[3].z {
		t.Fatal("a kick should speed the warp")
	}
}
