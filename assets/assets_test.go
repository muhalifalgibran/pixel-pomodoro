package assets

import "testing"

// The .pix files are meant to be edited freely. This test is the safety net:
// a typo in the art fails here rather than at runtime.
func TestEmbeddedSpritesParse(t *testing.T) {
	for _, name := range []string{"tomato"} {
		t.Run(name, func(t *testing.T) {
			s, err := Sprite(name)
			if err != nil {
				t.Fatalf("Sprite(%q) error = %v", name, err)
			}
			if s.Canvas.W == 0 || s.Canvas.H == 0 {
				t.Errorf("Sprite(%q) is empty", name)
			}
		})
	}
}

func TestSpriteMissingFile(t *testing.T) {
	if _, err := Sprite("nope"); err == nil {
		t.Error("Sprite() on a missing asset returned no error")
	}
}
