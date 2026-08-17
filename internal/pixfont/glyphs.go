package pixfont

// Glyphs are 5x7 bitmaps with a uniform one-pixel stroke and closed counters.
// '#' is on, anything else is off. Zero carries a diagonal so it never reads
// as an O.
//
// Keep every glyph exactly GlyphW x GlyphH; the tests enforce it.
var glyphs = map[rune][]string{
	'0': {
		".###.",
		"#...#",
		"#..##",
		"#.#.#",
		"##..#",
		"#...#",
		".###.",
	},
	'1': {
		"..#..",
		".##..",
		"..#..",
		"..#..",
		"..#..",
		"..#..",
		".###.",
	},
	'2': {
		".###.",
		"#...#",
		"....#",
		"..##.",
		".#...",
		"#....",
		"#####",
	},
	'3': {
		"####.",
		"....#",
		"....#",
		".###.",
		"....#",
		"....#",
		"####.",
	},
	'4': {
		"#...#",
		"#...#",
		"#...#",
		"#####",
		"....#",
		"....#",
		"....#",
	},
	'5': {
		"#####",
		"#....",
		"####.",
		"....#",
		"....#",
		"#...#",
		".###.",
	},
	'6': {
		".###.",
		"#....",
		"#....",
		"####.",
		"#...#",
		"#...#",
		".###.",
	},
	'7': {
		"#####",
		"....#",
		"...#.",
		"..#..",
		".#...",
		".#...",
		".#...",
	},
	'8': {
		".###.",
		"#...#",
		"#...#",
		".###.",
		"#...#",
		"#...#",
		".###.",
	},
	'9': {
		".###.",
		"#...#",
		"#...#",
		".####",
		"....#",
		"....#",
		".###.",
	},
	// Dots sit on rows 1-2 and 5-6 so the gap between them is two pixels.
	// Tucking them closer reads as a single smeared bar at this scale.
	':': {
		".....",
		"..#..",
		"..#..",
		".....",
		".....",
		"..#..",
		"..#..",
	},
	' ': {
		".....",
		".....",
		".....",
		".....",
		".....",
		".....",
		".....",
	},
}

// ghostSource is the character whose lit pixels stand in for the unlit
// segments behind a glyph. '8' lights every digit segment, which is exactly
// the retro-LCD effect: you see the whole cell faintly, with the live digit
// bright on top.
const ghostSource = '8'

// mask reports the on/off bitmap for r, and whether r is drawable at all.
func mask(r rune) ([]string, bool) {
	g, ok := glyphs[r]
	return g, ok
}

func on(g []string, x, y int) bool {
	if y < 0 || y >= GlyphH || x < 0 || x >= GlyphW {
		return false
	}
	return g[y][x] == '#'
}
