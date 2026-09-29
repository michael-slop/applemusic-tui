package main

import (
	"math"
	"strings"
	"testing"
)

// orbPanelReference is orbPanel as it was before the sin/cos tables, kept to
// prove the tables change nothing.
func orbPanelReference(w, h int, spin, wobble float64, bands [32]float64) string {
	if w < 4 || h < 2 {
		return strings.Repeat(" ", max(0, w))
	}

	tilt := 0.85 + 0.55*math.Sin(wobble)
	sinTilt, cosTilt := math.Sincos(tilt)
	sinSpin, cosSpin := math.Sincos(spin)

	// Breathe with the overall loudness on top of the per-band corrugation.
	ring := orbRing * (1 + 0.30*bandsLevel(bands))

	tube := orbTubeRadii(orbPhiSamples, bands)

	// Scale so the widest the torus can get still fits, with a margin. Terminal
	// cells are about twice as tall as wide, hence the 2x on the x axis.
	extent := ring + orbTube*1.4
	scale := math.Min(0.45*float64(h), 0.225*float64(w)) * orbDepth / extent

	cells := make([]byte, w*h)
	shade := make([]int, w*h)
	invZ := make([]float64, w*h)
	for i := range cells {
		cells[i] = ' '
	}

	for ti := 0; ti < orbThetaSamples; ti++ {
		sinTheta, cosTheta := math.Sincos(2 * math.Pi * float64(ti) / orbThetaSamples)
		for pi := 0; pi < orbPhiSamples; pi++ {
			sinPhi, cosPhi := math.Sincos(2 * math.Pi * float64(pi) / orbPhiSamples)

			circleX := ring + tube[pi]*cosTheta
			circleY := tube[pi] * sinTheta

			x := circleX*(cosSpin*cosPhi+sinTilt*sinSpin*sinPhi) - circleY*cosTilt*sinSpin
			y := circleX*(sinSpin*cosPhi-sinTilt*cosSpin*sinPhi) + circleY*cosTilt*cosSpin
			z := orbDepth + cosTilt*circleX*sinPhi + circleY*sinTilt
			if z <= 0 {
				continue
			}
			ooz := 1 / z

			light := cosPhi*cosTheta*sinSpin - cosTilt*cosTheta*sinPhi - sinTilt*sinTheta +
				cosSpin*(cosTilt*sinTheta-cosTheta*sinTilt*sinPhi)
			if light <= 0 {
				continue // facing away from the light, leave it dark
			}

			col := w/2 + int(2*scale*ooz*x)
			row := h/2 - int(scale*ooz*y)
			if col < 0 || col >= w || row < 0 || row >= h {
				continue
			}
			idx := row*w + col
			if ooz <= invZ[idx] {
				continue
			}
			level := min(len(orbRamp)-1, int(light*8))
			invZ[idx] = ooz
			cells[idx] = orbRamp[level]
			shade[idx] = level
		}
	}

	return paintCells(cells, shade, w, h)
}

func TestOrbPanelTablesAreExact(t *testing.T) {
	for _, size := range [][2]int{{40, 20}, {76, 36}, {120, 40}} {
		for i := range 20 {
			var bands [32]float64
			for b := range bands {
				bands[b] = math.Mod(float64(i*7+b*3)/13, 1)
			}
			spin, wobble := float64(i)*0.37, float64(i)*0.11
			if got, want := orbPanel(size[0], size[1], spin, wobble, bands), orbPanelReference(size[0], size[1], spin, wobble, bands); got != want {
				t.Fatalf("%dx%d frame %d differs", size[0], size[1], i)
			}
		}
	}
	_ = strings.Repeat
}
