package anim

import (
	"testing"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
)

var white = canvas.Opaque(0xff, 0xff, 0xff)

func TestEmitRespectsCapacity(t *testing.T) {
	s := NewSystem(2, 1)

	if !s.Emit(Particle{Life: 1}) || !s.Emit(Particle{Life: 1}) {
		t.Fatal("Emit rejected a particle while the pool had room")
	}
	if s.Emit(Particle{Life: 1}) {
		t.Error("Emit accepted a particle past capacity")
	}
	if s.Count() != 2 {
		t.Errorf("Count() = %d, want 2", s.Count())
	}
}

// The pool must not grow: a long session emits continuously, and a system that
// reallocates would leak one wisp at a time.
func TestUpdateReusesTheBackingArray(t *testing.T) {
	s := NewSystem(8, 1)
	for i := 0; i < 8; i++ {
		s.Emit(Particle{Life: 0.5, Color: white})
	}
	before := cap(s.pool)

	for i := 0; i < 100; i++ {
		s.Update(0.05)
		s.Emit(Particle{Life: 0.5, Color: white})
	}

	if got := cap(s.pool); got != before {
		t.Errorf("pool capacity grew from %d to %d", before, got)
	}
}

func TestUpdateReapsExpiredParticles(t *testing.T) {
	s := NewSystem(4, 1)
	s.Emit(Particle{Life: 0.1, Color: white})
	s.Emit(Particle{Life: 10, Color: white})

	s.Update(0.5)

	if s.Count() != 1 {
		t.Errorf("Count() = %d after expiry, want 1", s.Count())
	}
}

func TestUpdateIntegratesVelocityAndGravity(t *testing.T) {
	s := NewSystem(1, 1)
	s.Gravity = -10 // rising, like steam
	s.Emit(Particle{X: 5, Y: 10, VX: 2, VY: 0, Life: 10, Color: white})

	s.Update(1)

	p := s.pool[0]
	if p.X != 7 {
		t.Errorf("X = %v, want 7", p.X)
	}
	if p.VY != -10 {
		t.Errorf("VY = %v, want -10 after one second of gravity", p.VY)
	}
	if p.Y != 0 {
		t.Errorf("Y = %v, want 0", p.Y)
	}
}

func TestUpdateIgnoresNonPositiveDelta(t *testing.T) {
	s := NewSystem(1, 1)
	s.Emit(Particle{X: 1, VX: 100, Life: 10, Color: white})

	s.Update(0)
	s.Update(-1)

	if s.pool[0].X != 1 {
		t.Errorf("X = %v, want the particle to be unmoved", s.pool[0].X)
	}
}

func TestDrawPlotsParticlesAndClipsOffCanvas(t *testing.T) {
	c := canvas.New(4, 4)
	s := NewSystem(2, 1)
	s.Emit(Particle{X: 1, Y: 2, Life: 10, Color: white})
	s.Emit(Particle{X: 99, Y: 99, Life: 10, Color: white}) // drifted away

	s.Draw(c)

	if !c.At(1, 2).A {
		t.Error("particle was not plotted")
	}
	// Reaching here without a panic covers the clipped particle.
}

func TestDrawFadesWithAge(t *testing.T) {
	c := canvas.New(2, 2)
	s := NewSystem(1, 1)
	s.Emit(Particle{X: 0, Y: 0, Life: 1, Age: 0.5, Color: white, Fade: true})

	s.Draw(c)

	got := c.At(0, 0)
	if !got.A {
		t.Fatal("faded particle vanished entirely")
	}
	if got.R >= white.R {
		t.Errorf("R = %d, want it dimmed below %d", got.R, white.R)
	}
}

func TestClearEmptiesThePool(t *testing.T) {
	s := NewSystem(4, 1)
	s.Emit(Particle{Life: 10})
	s.Clear()

	if s.Count() != 0 {
		t.Errorf("Count() = %d after Clear(), want 0", s.Count())
	}
}

func TestEasing(t *testing.T) {
	for _, fn := range []struct {
		name string
		f    func(float64) float64
	}{
		{"EaseOutCubic", EaseOutCubic},
		{"EaseOutQuad", EaseOutQuad},
	} {
		if got := fn.f(0); got != 0 {
			t.Errorf("%s(0) = %v, want 0", fn.name, got)
		}
		if got := fn.f(1); got != 1 {
			t.Errorf("%s(1) = %v, want 1", fn.name, got)
		}
		// Clamped outside [0,1].
		if got := fn.f(-1); got != 0 {
			t.Errorf("%s(-1) = %v, want 0", fn.name, got)
		}
		if got := fn.f(2); got != 1 {
			t.Errorf("%s(2) = %v, want 1", fn.name, got)
		}
		// Decelerating: past the halfway point in time, past halfway in value.
		if got := fn.f(0.5); got <= 0.5 {
			t.Errorf("%s(0.5) = %v, want > 0.5 for an ease-out", fn.name, got)
		}
	}
}

func TestPulseStaysInRange(t *testing.T) {
	for i := 0; i <= 40; i++ {
		v := Pulse(float64(i)*0.1, 2)
		if v < 0 || v > 1 {
			t.Fatalf("Pulse() = %v, outside [0,1]", v)
		}
	}
	if got := Pulse(1, 0); got != 0 {
		t.Errorf("Pulse with a zero period = %v, want 0", got)
	}
}
