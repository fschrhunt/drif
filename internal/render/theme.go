// Package render draws layout rows as terminal output. It owns the palette,
// the ANSI encoding and the width maths; the TUI and the pager both use it so
// they cannot drift apart.
package render

import (
	"os"
	"strconv"
	"strings"

	"github.com/muesli/termenv"
)

// Color is an optional RGB colour. An unset colour means "leave whatever the
// terminal is already using", which is how drif adapts to a user's theme.
type Color struct {
	R, G, B uint8
	Set     bool
}

// Hex parses "#rrggbb".
func Hex(s string) Color {
	if len(s) != 7 || s[0] != '#' {
		return Color{}
	}
	v := func(i int) uint8 {
		var n uint8
		for _, c := range s[i : i+2] {
			n <<= 4
			switch {
			case c >= '0' && c <= '9':
				n |= uint8(c - '0')
			case c >= 'a' && c <= 'f':
				n |= uint8(c-'a') + 10
			case c >= 'A' && c <= 'F':
				n |= uint8(c-'A') + 10
			}
		}
		return n
	}
	return Color{R: v(1), G: v(3), B: v(5), Set: true}
}

// Palette is one background's colours.
//
// Everything is monochrome except the two things a diff is actually about:
// additions and deletions. Colour that does not mean "added" or "removed" is
// noise while reading, so there is none.
type Palette struct {
	// Text and FileFg are usually unset, which leaves the terminal's own
	// foreground in place: the code is the user's type, not ours to colour.
	Text   Color
	Dim    Color
	Accent Color

	AddFg     Color
	AddBg     Color
	AddEmphBg Color
	DelFg     Color
	DelBg     Color
	DelEmphBg Color

	HunkFg Color
	FileFg Color
	// FileDim is the directory and status letter inside a file header: lighter
	// than the global dim, because it sits on the header's own surface.
	FileDim Color
	// FileBg is the dim surface behind a file header. It exists so a header
	// reads as a strip of chrome rather than a line of content.
	FileBg Color
}

// Dark suits terminals with a dark background. Dim is the same grey on both
// backgrounds, so it can be a literal; the change colours are mixed from the
// accent the way a diff band should be, not picked by eye.
func Dark() *Palette {
	return &Palette{
		Dim:       Hex("#636363"),
		AddFg:     Hex("#07c480"),
		AddBg:     Hex("#092f22"),
		AddEmphBg: Hex("#084d35"),
		DelFg:     Hex("#ff2e3f"),
		DelBg:     Hex("#3b1114"),
		DelEmphBg: Hex("#62171d"),
		HunkFg:    Hex("#636363"),
		FileFg:    Hex("#fafafa"),
		FileDim:   Hex("#9b9b9b"),
		FileBg:    mix(Hex("#0a0a0a"), Hex("#fafafa"), 0.12),
	}
}

// Light suits terminals with a light background.
func Light() *Palette {
	return &Palette{
		Dim:       Hex("#636363"),
		AddFg:     Hex("#0b8a46"),
		AddBg:     Hex("#e2f7ea"),
		AddEmphBg: Hex("#a2e6bb"),
		DelFg:     Hex("#cf222e"),
		DelBg:     Hex("#ffe6e8"),
		DelEmphBg: Hex("#f0abb0"),
		HunkFg:    Hex("#636363"),
		FileFg:    Hex("#0a0a0a"),
		FileDim:   Hex("#676767"),
		FileBg:    mix(Hex("#ffffff"), Hex("#0a0a0a"), 0.12),
	}
}

// DetectPalette returns the palette for the terminal drif is drawing into.
// There is deliberately no way to choose: the terminal already knows.
//
// COLORFGBG wins when it is set, because it is the terminal's own statement of
// its colours and it is cheap to read. Otherwise the background is asked of the
// terminal directly, and a terminal that will not answer is assumed dark, where
// a light palette would be unreadable.
func DetectPalette() *Palette {
	// A terminal has no alpha. The closest thing to a translucent layer is a
	// blend with the page, so ask the terminal for its actual background and
	// derive every surface from it. That is why the header band works on a
	// black terminal and on a warm-grey one.
	if bg, ok := terminalBackground(); ok {
		return paletteFor(bg)
	}
	if light, ok := colorFGBG(); ok {
		if light {
			return Light()
		}
		return Dark()
	}
	return Dark()
}

// terminalBackground asks the terminal for its background colour. Terminals
// that do not answer return NoColor, which is why every caller needs a fallback.
func terminalBackground() (Color, bool) {
	out := termenv.DefaultOutput()
	if out.TTY() == nil {
		return Color{}, false
	}
	c := out.BackgroundColor()
	switch c.(type) {
	case nil, termenv.NoColor:
		return Color{}, false
	}
	r, g, b := termenv.ConvertToRGB(c).RGB255()
	if r == 0 && g == 0 && b == 0 && c == termenv.ANSIColor(0) {
		// Either a genuinely black terminal or the library's default. Both are
		// safe to treat as black, which is the common case.
		return Color{R: 0, G: 0, B: 0, Set: true}, true
	}
	return Color{R: r, G: g, B: b, Set: true}, true
}

// paletteFor derives a palette from the page colour. Change bands are the page
// mixed toward the accent, the way a band should belong to the same family as
// everything else.
func paletteFor(bg Color) *Palette {
	dark := luminance(bg) < 0.5
	add := Hex("#07c480")
	del := Hex("#ff2e3f")
	mixer := Hex("#fafafa")
	if !dark {
		add = Hex("#0b8a46")
		del = Hex("#cf222e")
		mixer = Hex("#0a0a0a")
	}
	return &Palette{
		Dim:       Hex("#636363"),
		AddFg:     add,
		AddBg:     mix(bg, add, 0.20),
		AddEmphBg: mix(bg, add, 0.34),
		DelFg:     del,
		DelBg:     mix(bg, del, 0.20),
		DelEmphBg: mix(bg, del, 0.34),
		HunkFg:    Hex("#636363"),
		FileFg:    mixer,
		FileDim:   mix(bg, mixer, 0.62),
		FileBg:    mix(bg, mixer, 0.12),
	}
}

// mix blends b into a by t (0..1), in sRGB. Terminals cannot composite, so this
// is how a lower-opacity surface is expressed.
func mix(a, b Color, t float64) Color {
	l := func(x, y uint8) uint8 {
		return uint8(float64(x)*(1-t) + float64(y)*t + 0.5)
	}
	return Color{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), Set: true}
}

func luminance(c Color) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// colorFGBG decodes the de facto COLORFGBG convention, "foreground;background",
// where the background is one of the sixteen ANSI colours. 7 and 9-15 are light;
// 8, the bright black, is a dark grey and is treated as dark.
func colorFGBG() (light bool, ok bool) {
	v := os.Getenv("COLORFGBG")
	i := strings.LastIndexByte(v, ';')
	if i < 0 || i+1 >= len(v) {
		return false, false
	}
	n, err := strconv.Atoi(v[i+1:])
	if err != nil {
		return false, false
	}
	return n == 7 || n >= 9, true
}
