// Package render draws layout rows as terminal output. It owns the palette,
// the ANSI encoding and the width maths; the TUI and the pager both use it so
// they cannot drif apart.
package render

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fschrhunt/drif/internal/highlight"
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

// ClassCount is the number of highlight classes a palette assigns colours to.
const ClassCount = int(highlight.Error) + 1

// Palette is one background's colours. Structural colours (headers, gutters)
// come from the palette; syntax colours are only applied to tokenised text.
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

	Syntax [ClassCount]Color
}

// Dark suits terminals with a dark background: muted structure, syntax colour,
// and tinted bands on changed lines.
func Dark() *Palette {
	p := &Palette{
		Text:      Hex("#c9d1d9"),
		Dim:       Hex("#6e7681"),
		Accent:    Hex("#a371f7"),
		AddFg:     Hex("#7ee787"),
		AddBg:     Hex("#10241a"),
		AddEmphBg: Hex("#1f4d2f"),
		DelFg:     Hex("#ffa198"),
		DelBg:     Hex("#2a1417"),
		DelEmphBg: Hex("#5c1f26"),
		HunkFg:    Hex("#79c0ff"),
		HunkBg:    Hex("#0d1727"),
		FileFg:    Hex("#e6edf3"),
		FileBg:    Hex("#161b22"),
		Filler:    Hex("#0b0e13"),
	}
	p.Syntax[highlight.Keyword] = Hex("#ff7b72")
	p.Syntax[highlight.String] = Hex("#a5d6ff")
	p.Syntax[highlight.Comment] = Hex("#6e7681")
	p.Syntax[highlight.Number] = Hex("#79c0ff")
	p.Syntax[highlight.Function] = Hex("#d2a8ff")
	p.Syntax[highlight.Type] = Hex("#ffa657")
	p.Syntax[highlight.Constant] = Hex("#79c0ff")
	p.Syntax[highlight.Operator] = Hex("#c9d1d9")
	p.Syntax[highlight.Punctuation] = Hex("#8b949e")
	p.Syntax[highlight.Tag] = Hex("#7ee787")
	p.Syntax[highlight.Attribute] = Hex("#79c0ff")
	p.Syntax[highlight.Error] = Hex("#ff7b72")
	return p
}

// Light suits terminals with a light background.
func Light() *Palette {
	p := &Palette{
		Text:      Hex("#1f2328"),
		Dim:       Hex("#6e7781"),
		Accent:    Hex("#8250df"),
		AddFg:     Hex("#1a7f37"),
		AddBg:     Hex("#e6ffec"),
		AddEmphBg: Hex("#aceebb"),
		DelFg:     Hex("#cf222e"),
		DelBg:     Hex("#ffebe9"),
		DelEmphBg: Hex("#ffcecb"),
		HunkFg:    Hex("#0550ae"),
		HunkBg:    Hex("#ddf4ff"),
		FileFg:    Hex("#1f2328"),
		FileBg:    Hex("#f6f8fa"),
		Filler:    Hex("#f0f2f4"),
	}
	p.Syntax[highlight.Keyword] = Hex("#cf222e")
	p.Syntax[highlight.String] = Hex("#0a3069")
	p.Syntax[highlight.Comment] = Hex("#6e7781")
	p.Syntax[highlight.Number] = Hex("#0550ae")
	p.Syntax[highlight.Function] = Hex("#8250df")
	p.Syntax[highlight.Type] = Hex("#953800")
	p.Syntax[highlight.Constant] = Hex("#0550ae")
	p.Syntax[highlight.Operator] = Hex("#24292f")
	p.Syntax[highlight.Punctuation] = Hex("#6e7781")
	p.Syntax[highlight.Tag] = Hex("#116329")
	p.Syntax[highlight.Attribute] = Hex("#0550ae")
	p.Syntax[highlight.Error] = Hex("#cf222e")
	return p
}

// DetectPalette returns the palette for the terminal drif is drawing into.
// There is deliberately no way to choose: the terminal already knows, and a
// viewer that asks the user to describe their own screen has failed at the one
// thing it can see for free.
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
