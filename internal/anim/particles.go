// Package anim holds the motion primitives the HUD animates with: a
// fixed-capacity particle pool and a few easing helpers.
package anim

import (
	"math"
	"math/rand"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
)

// Particle is one pixel with velocity and a lifetime, in pixel units per
// second.
type Particle struct {
	X, Y   float64
	VX, VY float64

	Age, Life float64

	Color canvas.RGBA
	// Fade dims the particle toward black as it ages.
	Fade bool
}

// Alive reports whether the particle should still be drawn.
func (p Particle) Alive() bool { return p.Age < p.Life }

// System is a fixed-capacity particle pool. Capacity is fixed so a long focus
// session cannot grow the heap one wisp at a time; emitting into a full pool
// is a no-op rather than an allocation.
type System struct {
	pool []Particle
	rng  *rand.Rand

	// Gravity is added to every particle's VY each second. Negative values
	// make particles rise, which is how the steam wisps work.
	Gravity float64
	// Drag scales velocity each second, in [0,1]. Zero means no drag.
	Drag float64
}

// NewSystem builds a pool. seed is explicit so animations are reproducible in
// tests.
func NewSystem(capacity int, seed int64) *System {
	return &System{
		pool: make([]Particle, 0, capacity),
		rng:  rand.New(rand.NewSource(seed)),
	}
}

// Rand exposes the system's generator so callers scatter particles with the
// same reproducible stream.
func (s *System) Rand() *rand.Rand { return s.rng }

// Count is the number of live particles.
func (s *System) Count() int { return len(s.pool) }

// Cap is the pool capacity.
func (s *System) Cap() int { return cap(s.pool) }

// Emit adds a particle, reporting false when the pool is full.
func (s *System) Emit(p Particle) bool {
	if len(s.pool) >= cap(s.pool) {
		return false
	}
	s.pool = append(s.pool, p)
	return true
}

// Clear removes every particle.
func (s *System) Clear() { s.pool = s.pool[:0] }

// Update advances the simulation by dt seconds and reaps dead particles.
func (s *System) Update(dt float64) {
	if dt <= 0 {
		return
	}
	drag := 1.0
	if s.Drag > 0 {
		drag = math.Max(0, 1-s.Drag*dt)
	}

	out := s.pool[:0]
	for _, p := range s.pool {
		p.Age += dt
		if !p.Alive() {
			continue
		}
		p.VY += s.Gravity * dt
		p.VX *= drag
		p.VY *= drag
		p.X += p.VX * dt
		p.Y += p.VY * dt
		out = append(out, p)
	}
	s.pool = out
}

// Draw plots every live particle. Out-of-bounds particles are clipped by the
// canvas rather than filtered here, so a wisp can drift off the panel and the
// simulation stays simple.
func (s *System) Draw(c *canvas.Canvas) {
	for _, p := range s.pool {
		col := p.Color
		if p.Fade && p.Life > 0 {
			remaining := 1 - p.Age/p.Life
			col = canvas.Scale(col, remaining)
			col.A = true
		}
		c.Set(int(math.Round(p.X)), int(math.Round(p.Y)), col)
	}
}
