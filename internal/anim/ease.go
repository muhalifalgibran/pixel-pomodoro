package anim

import "math"

// Clamp01 constrains t to [0,1].
func Clamp01(t float64) float64 {
	switch {
	case t < 0:
		return 0
	case t > 1:
		return 1
	}
	return t
}

// EaseOutCubic decelerates toward the target. Used for the digit roll, where
// the motion should land rather than arrive at full speed.
func EaseOutCubic(t float64) float64 {
	t = Clamp01(t)
	u := 1 - t
	return 1 - u*u*u
}

// EaseOutQuad is a gentler deceleration, for the palette cross-fade.
func EaseOutQuad(t float64) float64 {
	t = Clamp01(t)
	return 1 - (1-t)*(1-t)
}

// Pulse is a sine wave in [0,1] with the given period in seconds. It drives
// the idle breathing and the paused heartbeat.
func Pulse(elapsed, period float64) float64 {
	if period <= 0 {
		return 0
	}
	return 0.5 + 0.5*math.Sin(2*math.Pi*elapsed/period)
}
