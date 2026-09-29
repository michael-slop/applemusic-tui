package main

import "math"

// Motion follows the music. Every animation advances by elapsed time (the fx
// effects through Audio.DT, the torus and sphere through their spin), so one
// number scales how fast all of them move: nearly still when the song is off
// or silent, full speed when it is loud. The frames are still drawn; the
// difference between music on and music off is visible at a glance.
//
// The reactivity slider decides how much of that the music controls: at 0 a
// playing song runs everything at the panefx/original speed; from 50% up the
// speed follows the loudness entirely. Paused is always nearly still.

const (
	motionIdle  = 0.04 // speed when paused or silent: barely moving
	motionQuiet = 0.15 // mean band level at or below which a song reads as silent
	motionLoud  = 0.60 // mean band level at or above which it runs at full speed
	motionEase  = 0.5  // seconds to ease between speeds (no snapping on pause)
)

// advanceMotion updates the speed for one 30 fps frame.
func (m *model) advanceMotion() {
	target := motionIdle
	if m.st.Playing {
		energy := min(max((bandsMean(m.vizBands)-motionQuiet)/(motionLoud-motionQuiet), 0), 1)
		follow := min(1, 2*m.reactivity) // how much of the speed the music owns
		target = motionIdle + (1-motionIdle)*(1-follow+follow*energy)
	}
	if !m.motionReady {
		m.motion, m.motionReady = target, true
		return
	}
	m.motion += (target - m.motion) * (1 - math.Exp(-1.0/30/motionEase))
}

// motionSpeed is the current speed multiplier, 0.04..1.
func (m model) motionSpeed() float64 {
	if !m.motionReady {
		return 1
	}
	return m.motion
}
