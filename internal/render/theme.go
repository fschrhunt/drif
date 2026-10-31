// Package render draws layout rows as terminal output. It owns the palette,
// the ANSI encoding and the width maths; the TUI and the pager both use it so
// they cannot drift apart.
package render

import "github.com/fschrhunt/drift/internal/highlight"

// Color is an optional RGB colour. An unset colour means "leave whatever the
// terminal is already using", which is how drift adapts to a user's theme.
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

// Palette is one theme's colours. Structural colours (headers, gutters) come
// from the theme; syntax colours are only applied to tokenised text.
type Palette struct {
	Name string

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

// Dark is the default theme: muted structure, syntax colour, and tinted bands
// on changed lines.
func Dark() *Palette {
	p := &Palette{
		Name:      "dark",
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

// Light is for terminals with a light background.
func Light() *Palette {
	p := &Palette{
		Name:      "light",
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

// ThemeByName returns the named palette. "auto" picks light only when the
// terminal advertises a light background through COLORFGBG.
func ThemeByName(name string) *Palette {
	switch name {
	case "light":
		return Light()
	case "dark":
		return Dark()
	}
	if colorFGBGSaysLight() {
		return Light()
	}
	return Dark()
}
