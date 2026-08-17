package theme

import (
	"testing"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

func TestForCoversEveryPhase(t *testing.T) {
	tests := []struct {
		phase timer.Phase
		want  string
	}{
		{timer.Focus, "ember"},
		{timer.ShortBreak, "mint"},
		{timer.LongBreak, "indigo"},
	}
	for _, tt := range tests {
		if got := For(tt.phase); got.Name != tt.want {
			t.Errorf("For(%v).Name = %q, want %q", tt.phase, got.Name, tt.want)
		}
	}
}

func TestByName(t *testing.T) {
	for _, name := range []string{"ember", "mint", "indigo"} {
		if _, ok := ByName(name); !ok {
			t.Errorf("ByName(%q) reported the palette as unknown", name)
		}
	}
	if _, ok := ByName("chartreuse"); ok {
		t.Error("ByName accepted a palette that does not exist")
	}
}

// The config file's theme keys and the phase palettes must be the same set, or
// a valid config value could name a palette nothing can select.
func TestEveryPhasePaletteIsAddressableByName(t *testing.T) {
	for _, p := range []timer.Phase{timer.Focus, timer.ShortBreak, timer.LongBreak} {
		name := For(p).Name
		if _, ok := ByName(name); !ok {
			t.Errorf("palette %q is used for %v but ByName cannot find it", name, p)
		}
	}
}

func TestLerpEndpoints(t *testing.T) {
	if got := Lerp(Ember, Mint, 0); got.Panel != Ember.Panel {
		t.Error("Lerp at t=0 did not return the first palette")
	}
	if got := Lerp(Ember, Mint, 1); got.Panel != Mint.Panel {
		t.Error("Lerp at t=1 did not return the second palette")
	}
}

func TestLerpBlendsEveryChannel(t *testing.T) {
	mid := Lerp(Ember, Mint, 0.5)

	if mid.Panel == Ember.Panel || mid.Panel == Mint.Panel {
		t.Error("Panel did not blend")
	}
	if mid.Accent == Ember.Accent || mid.Accent == Mint.Accent {
		t.Error("Accent did not blend")
	}
	if mid.Clock.FaceTop == Ember.Clock.FaceTop || mid.Clock.FaceTop == Mint.Clock.FaceTop {
		t.Error("clock face did not blend")
	}
	want := (Ember.SpriteTintStrength + Mint.SpriteTintStrength) / 2
	if mid.SpriteTintStrength != want {
		t.Errorf("SpriteTintStrength = %v, want %v", mid.SpriteTintStrength, want)
	}
}

func TestPalettesAreOpaque(t *testing.T) {
	// A transparent palette color would punch a hole in the HUD.
	for _, p := range []Palette{Ember, Mint, Indigo} {
		checks := []struct {
			field string
			ok    bool
		}{
			{"Panel", p.Panel.A},
			{"Frame", p.Frame.A},
			{"FrameDim", p.FrameDim.A},
			{"Accent", p.Accent.A},
			{"AccentDim", p.AccentDim.A},
			{"Text", p.Text.A},
			{"TextDim", p.TextDim.A},
			{"Clock.FaceTop", p.Clock.FaceTop.A},
			{"Clock.FaceBottom", p.Clock.FaceBottom.A},
			{"Clock.Outline", p.Clock.Outline.A},
			{"Clock.Bevel", p.Clock.Bevel.A},
			{"Clock.Ghost", p.Clock.Ghost.A},
		}
		for _, c := range checks {
			if !c.ok {
				t.Errorf("palette %q has a transparent %s", p.Name, c.field)
			}
		}
	}
}
