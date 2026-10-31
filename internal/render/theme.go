// Package render draws layout rows as terminal output. It owns the palette,
// the ANSI encoding and the width maths; the TUI and the pager both use it so
// they cannot drift apart.
package render

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	HunkBg Color
	FileFg Color
	FileBg Color
	Filler Color
}

// Dark suits terminals with a dark background.
func Dark() *Palette {
	return &Palette{
		Text:      Hex("#d4d4d4"),
		Dim:       Hex("#6b6b6b"),
		Accent:    Hex("#e8e8e8"),
		AddFg:     Hex("#4ec97a"),
		AddBg:     Hex("#101a14"),
		AddEmphBg: Hex("#1d3b2a"),
		DelFg:     Hex("#e5534b"),
		DelBg:     Hex("#1e1113"),
		DelEmphBg: Hex("#4a1e22"),
		HunkFg:    Hex("#8a8a8a"),
		HunkBg:    Hex("#121212"),
		FileFg:    Hex("#e6e6e6"),
		FileBg:    Hex("#191919"),
		Filler:    Hex("#0d0d0d"),
	}
}

// Light suits terminals with a light background.
func Light() *Palette {
	return &Palette{
		Text:      Hex("#1f1f1f"),
		Dim:       Hex("#6e6e6e"),
		Accent:    Hex("#111111"),
		AddFg:     Hex("#1a7f37"),
		AddBg:     Hex("#eefbf1"),
		AddEmphBg: Hex("#c9f0d6"),
		DelFg:     Hex("#cf222e"),
		DelBg:     Hex("#fdeff0"),
		DelEmphBg: Hex("#ffd5d8"),
		HunkFg:    Hex("#6e6e6e"),
		HunkBg:    Hex("#f0f0f0"),
		FileFg:    Hex("#111111"),
		FileBg:    Hex("#f6f6f6"),
		Filler:    Hex("#f0f0f0"),
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
	if light, ok := colorFGBG(); ok {
		if light {
			return Light()
		}
		return Dark()
	}
	if lipgloss.HasDarkBackground() {
		return Dark()
	}
	return Light()
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
