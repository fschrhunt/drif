package render

import (
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Depth is how much colour the terminal can show.
type Depth uint8

const (
	// NoColor emits plain text.
	NoColor Depth = iota
	ANSI256
	TrueColor
)

// DetectDepth reads the environment once. NO_COLOR always wins.
func DetectDepth() Depth {
	if os.Getenv("NO_COLOR") != "" {
		return NoColor
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit") {
		return TrueColor
	}
	return ANSI256
}

// Style is a terminal attribute set. Zero value renders nothing.
type Style struct {
	Fg, Bg    Color
	Bold      bool
	Dim       bool
	Italic    bool
	Underline bool
	Reverse   bool
}

// sgr encodes the style as an SGR sequence, or "" when nothing is set.
func (s Style) sgr(d Depth) string {
	if d == NoColor {
		return ""
	}
	var b strings.Builder
	add := func(v string) {
		if b.Len() > 0 {
			b.WriteByte(';')
		}
		b.WriteString(v)
	}
	if s.Bold {
		add("1")
	}
	if s.Dim {
		add("2")
	}
	if s.Italic {
		add("3")
	}
	if s.Underline {
		add("4")
	}
	if s.Reverse {
		add("7")
	}
	if s.Fg.Set {
		add(s.Fg.sgr(d, false))
	}
	if s.Bg.Set {
		add(s.Bg.sgr(d, true))
	}
	if b.Len() == 0 {
		return ""
	}
	return "\x1b[" + b.String() + "m"
}

func (c Color) sgr(d Depth, bg bool) string {
	lead := "38"
	if bg {
		lead = "48"
	}
	if d == TrueColor {
		return lead + ";2;" + strconv.Itoa(int(c.R)) + ";" + strconv.Itoa(int(c.G)) + ";" + strconv.Itoa(int(c.B))
	}
	return lead + ";5;" + strconv.Itoa(rgbTo256(c.R, c.G, c.B))
}

// rgbTo256 picks the nearest xterm-256 entry, comparing the colour cube with
// the greyscale ramp so neutral greys do not turn blue.
func rgbTo256(r, g, b uint8) int {
	ri, gi, bi := int(r), int(g), int(b)
	cube := func(v int) int {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return (v - 35) / 40
	}
	cr, cg, cb := cube(ri), cube(gi), cube(bi)
	cubeR := []int{0, 95, 135, 175, 215, 255}
	dc := sq(ri-cubeR[cr]) + sq(gi-cubeR[cg]) + sq(bi-cubeR[cb])

	grayAvg := (ri + gi + bi) / 3
	gi2 := (grayAvg - 8) / 10
	if gi2 < 0 {
		gi2 = 0
	}
	if gi2 > 23 {
		gi2 = 23
	}
	gv := 8 + gi2*10
	dg := sq(ri-gv) + sq(gi-gv) + sq(bi-gv)

	if dg < dc {
		return 232 + gi2
	}
	return 16 + 36*cr + 6*cg + cb
}

func sq(v int) int { return v * v }

// Seg is a run of text with one style.
type Seg struct {
	Text  string
	Style Style
}

// Width is the display width of a segment list.
func Width(segs []Seg) int {
	n := 0
	for _, s := range segs {
		n += runewidth.StringWidth(s.Text)
	}
	return n
}

// ANSI renders segments, resetting at the end of every line so a background
// band cannot bleed past the row.
func ANSI(segs []Seg, d Depth) string {
	if d == NoColor {
		var b strings.Builder
		for _, s := range segs {
			b.WriteString(s.Text)
		}
		return b.String()
	}
	var b strings.Builder
	for _, s := range segs {
		if s.Text == "" {
			continue
		}
		if code := s.Style.sgr(d); code != "" {
			b.WriteString(code)
			b.WriteString(s.Text)
			b.WriteString("\x1b[0m")
			continue
		}
		b.WriteString(s.Text)
	}
	return b.String()
}

// Fit truncates a segment list to w columns and pads it with fill, which is
// what makes changed-line bands reach the right edge of the screen.
func Fit(segs []Seg, w int, fill Style) []Seg {
	if w <= 0 {
		return segs
	}
	out := make([]Seg, 0, len(segs)+1)
	used := 0
	for _, s := range segs {
		if used >= w {
			break
		}
		text, w2 := clip(s.Text, w-used)
		if text != "" {
			out = append(out, Seg{Text: text, Style: s.Style})
			used += w2
		}
		if used >= w {
			break
		}
	}
	if used < w {
		out = append(out, Seg{Text: strings.Repeat(" ", w-used), Style: fill})
	}
	return out
}

// Wrap splits segments into visual lines of at most w columns.
func Wrap(segs []Seg, w int) [][]Seg {
	if w <= 0 {
		return [][]Seg{segs}
	}
	var lines [][]Seg
	var cur []Seg
	used := 0
	push := func() {
		lines = append(lines, cur)
		cur = nil
		used = 0
	}
	for _, s := range segs {
		text := s.Text
		for text != "" {
			if used == w {
				push()
			}
			chunk, w2 := clip(text, w-used)
			if chunk == "" {
				push()
				continue
			}
			if n := len(cur); n > 0 && cur[n-1].Style == s.Style {
				cur[n-1].Text += chunk
			} else {
				cur = append(cur, Seg{Text: chunk, Style: s.Style})
			}
			used += w2
			text = text[len(chunk):]
		}
	}
	if len(cur) > 0 || len(lines) == 0 {
		lines = append(lines, cur)
	}
	return lines
}

// clip returns the longest prefix of s that fits in w columns and its width.
func clip(s string, w int) (string, int) {
	if w <= 0 {
		return "", 0
	}
	used := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			continue
		}
		if used+rw > w {
			return s[:i], used
		}
		used += rw
	}
	return s, used
}
