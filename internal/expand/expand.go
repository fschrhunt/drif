// Package expand turns source text into the form the renderers draw: tabs
// become spaces and every other control character becomes a visible picture.
//
// The second part is not cosmetic. drif draws file contents that come from a
// repository, and a file containing ESC can otherwise drive the terminal: move
// the cursor, change colours, or set the clipboard through OSC 52. Git does the
// same thing for its own output; drif has to do it for the text it adds.
package expand

import "strings"

// TabWidth is how many columns a tab occupies. Source that indents with tabs
// would otherwise jump to the terminal's tab stops after the gutter, which
// makes every indentation level look distorted.
const TabWidth = 4

// Expand widens tabs and replaces C0 controls and DEL with their Unicode
// control pictures (U+2400..U+241F and U+2421). The result never contains a
// byte a terminal would act on, and each replacement keeps a width of one, so
// byte offsets and columns stay predictable.
func Expand(s string) string {
	if !needs(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20:
			b.WriteRune(0x2400 + r)
		case r == 0x7f:
			b.WriteRune(0x2421)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Controls neutralises control characters but leaves tabs alone. Plain output
// is not laid out in columns, so a tab is harmless there and dropping it would
// change the bytes an agent reads.
func Controls(s string) string {
	bad := false
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 && c != '\t' || c == 0x7f {
			bad = true
			break
		}
	}
	if !bad {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0x7f:
			b.WriteRune(0x2421)
		case r < 0x20 && r != '\t':
			b.WriteRune(0x2400 + r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func needs(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}
