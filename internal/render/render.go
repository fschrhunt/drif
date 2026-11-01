package render

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/fschrhunt/drif/internal/expand"
	"github.com/fschrhunt/drif/internal/layout"
	"github.com/fschrhunt/drif/internal/patch"
)

// Renderer turns layout rows into styled segments.
type Renderer struct {
	Pal     *Palette
	Width   int
	Mode    layout.Mode
	Depth   Depth
	Prefix  string
	Numbers bool
	// Query, when set, is highlighted in every rendered line.
	Query string
}

// Segments renders one row. Unified and plain rows are returned at full length
// so the caller can wrap them; split rows are fitted to Width because their
// two columns have fixed positions.
func (r *Renderer) Segments(row layout.Row) []Seg {
	switch row.Kind {
	case layout.FileRow:
		return r.fileSegs(row.File)
	case layout.MetaRow:
		return r.metaSegs(row)
	case layout.HunkRow:
		return r.hunkSegs(row)
	}
	if r.Mode == layout.Split {
		return r.splitSegs(row)
	}
	return r.unifiedSegs(row)
}

// Line renders a row as a single physical line.
func (r *Renderer) Line(row layout.Row) string {
	return ANSI(Fit(r.Segments(row), r.Width, r.Fill(row)), r.Depth)
}

// Lines renders every row.
func (r *Renderer) Lines(rows []layout.Row) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = r.Line(row)
	}
	return out
}

// Fill is the style that extends a row to the full width.
func (r *Renderer) Fill(row layout.Row) Style {
	if row.Kind != layout.LineRow || r.Mode == layout.Split {
		return Style{}
	}
	return r.baseStyle(row.Sign)
}

// Height is how many visual lines a row occupies at the current width.
func (r *Renderer) Height(row layout.Row) int {
	if row.Kind == layout.FileRow {
		return 2 // the name and the rule under it
	}
	if r.Width <= 0 || r.Mode == layout.Split {
		return 1
	}
	var w int
	switch row.Kind {
	case layout.MetaRow, layout.HunkRow:
		w = runewidth.StringWidth(row.Text) + 2
	default:
		w = runewidth.StringWidth(row.Text) + row.Gutter + 2
		if !r.Numbers {
			w -= row.Gutter + 1
		}
	}
	if w <= r.Width {
		return 1
	}
	return (w + r.Width - 1) / r.Width
}

// fileSegs draws a file header. It is text on the terminal's own background:
// a header is a label, not a surface, and a band here would compete with the
// bands that mark changed lines.
func (r *Renderer) fileSegs(f *patch.File) []Seg {
	bg := Style{Bg: r.Pal.FileBg}
	left := []Seg{{Text: " ", Style: bg}}

	path := expand.Expand(strings.TrimPrefix(f.Path(), r.Prefix))
	dir, base := path, ""
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		dir, base = path[:i+1], path[i+1:]
	}
	if dir != "" {
		left = append(left, Seg{Text: dir, Style: Style{Fg: r.Pal.Dim, Bg: r.Pal.FileBg}})
	}
	left = append(left, Seg{Text: base, Style: Style{Fg: r.Pal.FileFg, Bg: r.Pal.FileBg, Bold: true}})
	if f.Status != patch.Modified {
		left = append(left, Seg{Text: "  " + string(f.Status.Letter()), Style: Style{Fg: r.Pal.Dim, Bg: r.Pal.FileBg}})
	}

	right := make([]Seg, 0, 4)
	if f.Additions > 0 {
		right = append(right, Seg{Text: "+" + strconv.Itoa(f.Additions), Style: Style{Fg: r.Pal.AddFg, Bg: r.Pal.FileBg}})
	}
	if f.Deletions > 0 {
		if len(right) > 0 {
			right = append(right, Seg{Text: " ", Style: bg})
		}
		right = append(right, Seg{Text: "-" + strconv.Itoa(f.Deletions), Style: Style{Fg: r.Pal.DelFg, Bg: r.Pal.FileBg}})
	}
	right = append(right, Seg{Text: " ", Style: bg})

	return r.joinBand(left, right, bg)
}

// Rule is the single dim line that ends a file header.
func (r *Renderer) Rule() string {
	if r.Width <= 0 {
		return ""
	}
	segs := []Seg{{Text: strings.Repeat("─", r.Width), Style: Style{Fg: r.Pal.Dim, Bg: r.Pal.FileBg}}}
	return ANSI(segs, r.Depth)
}

// joinBand places left and right text on one full-width band, filling the gap
// with the band's own background.
func (r *Renderer) joinBand(left, right []Seg, fill Style) []Seg {
	w := r.Width
	if w <= 0 {
		return append(left, right...)
	}
	lw, rw := Width(left), Width(right)
	gap := w - lw - rw
	if gap < 1 {
		left = Fit(left, max(w-rw-1, 0), fill)
		lw = Width(left)
		gap = w - lw - rw
	}
	if gap > 0 {
		left = append(left, Seg{Text: strings.Repeat(" ", gap), Style: fill})
	}
	return append(left, right...)
}

func (r *Renderer) metaSegs(row layout.Row) []Seg {
	segs := []Seg{
		{Text: "  ", Style: Style{}},
		{Text: row.Text, Style: Style{Fg: r.Pal.Dim}},
	}
	return Fit(segs, r.Width, Style{})
}

func (r *Renderer) hunkSegs(row layout.Row) []Seg {
	segs := []Seg{
		{Text: " ", Style: Style{}},
		{Text: row.Text, Style: Style{Fg: r.Pal.HunkFg}},
	}
	return Fit(segs, r.Width, Style{})
}

func (r *Renderer) unifiedSegs(row layout.Row) []Seg {
	base := r.baseStyle(row.Sign)
	segs := make([]Seg, 0, 8)

	if r.Numbers {
		segs = append(segs, Seg{
			Text:  padNum(row.No, row.Gutter),
			Style: Style{Fg: r.Pal.Dim, Bg: base.Bg},
		})
	}
	segs = append(segs, Seg{Text: string(row.Sign), Style: r.signStyle(row.Sign)})
	segs = append(segs, Seg{Text: " ", Style: base})

	segs = append(segs, r.compose(row.Text, row.Emph, base, r.emphStyle(row.Sign))...)

	// Deliberately unpadded: the caller decides whether to wrap or to fit, so
	// the TUI and the pager can share one segment stream.
	return segs
}

func (r *Renderer) splitSegs(row layout.Row) []Seg {
	total := r.Width
	if total <= 0 {
		total = 160
	}
	const sepW = 2
	avail := total - sepW
	if avail < 20 {
		// Too narrow to compare: fall back to a single column so the row is
		// still readable at any terminal size.
		return r.unifiedFromSplit(row)
	}
	lw := avail / 2
	rw := avail - lw

	segs := r.cellSegs(row.File, row.Left, true, lw, row.Gutter)
	// A drawn divider competes with the gutter for a job the gutter already
	// does. Two blank columns of the terminal's own background separate the
	// halves and nothing else.
	segs = append(segs, Seg{Text: "  ", Style: Style{}})
	segs = append(segs, r.cellSegs(row.File, row.Right, false, rw, row.Gutter)...)
	return segs
}

func (r *Renderer) unifiedFromSplit(row layout.Row) []Seg {
	if row.Right != nil {
		return r.unifiedSegs(layout.Row{
			Kind: layout.LineRow, File: row.File, Text: row.Right.Text,
			Sign: '+', No: row.Right.No, Emph: row.Right.Emph, Gutter: row.Gutter,
		})
	}
	return r.unifiedSegs(layout.Row{
		Kind: layout.LineRow, File: row.File, Text: row.Left.Text,
		Sign: '-', No: row.Left.No, Emph: row.Left.Emph, Gutter: row.Gutter,
	})
}

func (r *Renderer) cellSegs(f *patch.File, c *layout.Cell, old bool, w, gutter int) []Seg {
	if c == nil {
		return []Seg{{Text: strings.Repeat(" ", w), Style: Style{}}}
	}
	base := r.baseStyle(signOf(c.Kind))
	segs := make([]Seg, 0, 8)
	gw := 0
	if r.Numbers {
		segs = append(segs, Seg{
			Text:  padNum(c.No, gutter),
			Style: Style{Fg: r.Pal.Dim, Bg: base.Bg},
		})
		gw = gutter + 1
	}
	segs = append(segs, Seg{Text: string(signOf(c.Kind)), Style: r.signStyle(signOf(c.Kind))})
	segs = append(segs, Seg{Text: " ", Style: base})
	gw += 2

	segs = append(segs, r.compose(c.Text, c.Emph, base, r.emphStyle(signOf(c.Kind)))...)
	return Fit(segs, w, base)
}

// emphStyle is the background used for the words that actually changed inside
// a changed line.
func (r *Renderer) emphStyle(sign byte) Color {
	if sign == '-' {
		return r.Pal.DelEmphBg
	}
	return r.Pal.AddEmphBg
}

// compose applies intra-line emphasis and the search highlight to a line that
// is otherwise a single colour.
func (r *Renderer) compose(text string, emph []layout.Span, base Style, emphBg Color) []Seg {
	if text == "" {
		return nil
	}
	cuts := make([]int, 0, 2+2*len(emph))
	cuts = append(cuts, 0, len(text))
	for _, s := range emph {
		if s.Start >= 0 && s.End <= len(text) && s.Start < s.End {
			cuts = append(cuts, s.Start, s.End)
		}
	}
	matches := matchRanges(text, r.Query)
	for _, m := range matches {
		cuts = append(cuts, m.Start, m.End)
	}
	sort.Ints(cuts)

	out := make([]Seg, 0, len(cuts))
	ei, mi := 0, 0
	for i := 0; i+1 < len(cuts); i++ {
		a, b := cuts[i], cuts[i+1]
		if a == b {
			continue
		}
		style := base
		for ei < len(emph) && emph[ei].End <= a {
			ei++
		}
		if ei < len(emph) && emph[ei].Start <= a {
			style.Bg = emphBg
			style.Bold = true
		}
		for mi < len(matches) && matches[mi].End <= a {
			mi++
		}
		if mi < len(matches) && matches[mi].Start <= a {
			style.Reverse = true
		}
		if n := len(out); n > 0 && out[n-1].Style == style {
			out[n-1].Text += text[a:b]
			continue
		}
		out = append(out, Seg{Text: text[a:b], Style: style})
	}
	return out
}

// matchRanges finds case-insensitive occurrences. Lowercasing is done byte by
// byte so byte offsets stay valid for slicing.
func matchRanges(text, query string) []layout.Span {
	if query == "" || text == "" {
		return nil
	}
	lower := lowerASCII(text)
	q := lowerASCII(query)
	var out []layout.Span
	for i := 0; ; {
		j := strings.Index(lower[i:], q)
		if j < 0 {
			return out
		}
		start := i + j
		out = append(out, layout.Span{Start: start, End: start + len(q)})
		i = start + len(q)
		if i >= len(lower) {
			return out
		}
	}
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func (r *Renderer) baseStyle(sign byte) Style {
	switch sign {
	case '+':
		return Style{Bg: r.Pal.AddBg}
	case '-':
		return Style{Bg: r.Pal.DelBg}
	}
	return Style{}
}

func (r *Renderer) signStyle(sign byte) Style {
	switch sign {
	case '+':
		return Style{Fg: r.Pal.AddFg, Bg: r.Pal.AddBg, Bold: true}
	case '-':
		return Style{Fg: r.Pal.DelFg, Bg: r.Pal.DelBg, Bold: true}
	}
	return Style{}
}

func signOf(kind patch.LineKind) byte {
	switch kind {
	case patch.Add:
		return '+'
	case patch.Del:
		return '-'
	}
	return ' '
}

// padNum right-aligns a line number and appends one space.
func padNum(n, digits int) string {
	s := strconv.Itoa(n)
	if len(s) < digits {
		s = strings.Repeat(" ", digits-len(s)) + s
	}
	return s + " "
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
