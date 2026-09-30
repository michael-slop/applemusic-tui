package fx

// Shared helpers for the ports of panefx's plasma/tunnel/starfield/spin3d
// ("pts"): a quantised colour table sampled from the theme ramp, a smoothing
// glide for audio reactions, and a clamped frame time.

import "math"

// ptsColourSteps is how many distinct colours the theme ramp is sampled into.
// panefx quantised colours because its renderer paid one draw call per colour
// per row; amtui's Render likewise emits one escape per colour run, so the
// same budget applies. Shared by plasma, tunnel, starfield and spin3d.
const ptsColourSteps = 16

// ptsLUT samples the palette ramp from lo to hi into a fixed table.
func ptsLUT(p Palette, lo, hi float64) (lut [ptsColourSteps]RGB) {
	for i := range lut {
		lut[i] = p.At(lo + (hi-lo)*float64(i)/float64(ptsColourSteps-1))
	}
	return lut
}

// ptsLUTAt picks the LUT entry for t in 0..1.
func ptsLUTAt(lut *[ptsColourSteps]RGB, t float64) RGB {
	i := int(t*float64(ptsColourSteps-1) + 0.5)
	return lut[min(max(i, 0), ptsColourSteps-1)]
}

// ptsGlide moves cur toward target with time constant tau seconds, so audio
// reactions ease in and out instead of flickering frame to frame.
func ptsGlide(cur, target, dt, tau float64) float64 {
	return cur + (target-cur)*(1-math.Exp(-dt/tau))
}

// ptsDT is the frame's elapsed time, clamped so a stalled frame (a resize, a
// suspended laptop) cannot fling a simulation forward.
func ptsDT(a Audio) float64 {
	if !(a.DT > 0) {
		return 0
	}
	return min(a.DT, 0.25)
}
