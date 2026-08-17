// Package assets embeds the art files and hands back parsed sprites. Art lives
// as plain text under sprites/ so it can be redrawn without touching Go code;
// embedding keeps the shipped binary self-contained.
package assets

import (
	"embed"
	"fmt"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/sprite"
)

//go:embed sprites/*.pix
var files embed.FS

// Sprite parses an embedded .pix file. name omits the directory and
// extension, e.g. "tomato".
func Sprite(name string) (*sprite.Sprite, error) {
	path := "sprites/" + name + ".pix"
	f, err := files.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return sprite.Parse(path, f)
}

// MustSprite is Sprite for art the binary cannot run without. A malformed
// embedded asset is a build-time mistake, so failing loudly at startup beats
// rendering garbage.
func MustSprite(name string) *sprite.Sprite {
	s, err := Sprite(name)
	if err != nil {
		panic(err)
	}
	return s
}
