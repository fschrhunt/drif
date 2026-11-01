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
	if r.Mode == layout.Split && row.Kind == layout.LineRow {
		return r.SplitLines(row)[0]
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
	if r.Width <= 0 {
		return 1
	}
	if row.Kind == layout.LineRow && r.Mode == layout.Split {
		if lw, _, ok := r.splitWidths(); ok {
			tw := lw - r.cellLead(row.Gutter)
			h := 1
			if row.Left != nil {
				h = max(h, wrapCount(row.Left.Text, tw))
			}
			if row.Right != nil {
				h = max(h, wrapCount(row.Right.Text, tw))
			}
			return h
		}
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

// wrapCount is how many lines text needs at width w.
func wrapCount(text string, w int) int {
	if w <= 0 {
		return 1
	}
	width := runewidth.StringWidth(text)
	if width == 0 {
		return 1
	}
	return (width + w - 1) / w
}

// splitWidths is the width of each pane, or false when the terminal is too
// narrow to compare.
func (r *Renderer) splitWidths() (lw, rw int, ok bool) {
	if r.Width < 20 {
		return 0, 0, false
	}
	lw = r.Width / 2
	return lw, r.Width - lw, true
}

// cellLead is the columns before a cell's text: number, sign and a space.
func (r *Renderer) cellLead(gutter int) int {
	if !r.Numbers {
		return 2
	}
	return gutter + 3
}

// fileSegs draws a file header. It is text on the terminal's own background:
// a header is a label, not a surface, and a band here would compete with the
// bands that mark changed lines.
func (r *Renderer) fileSegs(f *patch.File) []Seg {
	bg := Style{Bg: r.Pal.FileBg}
	left := []Seg{{Text: " ", Style: bg}}

	path := expand.Expand(strings.TrimPrefix(f.Path(), r.Prefix))
	dir, base := "", path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		dir, base = path[:i+1], path[i+1:]
	}
	if dir != "" {
		left = append(left, Seg{Text: dir, Style: Style{Fg: r.Pal.FileDim, Bg: r.Pal.FileBg}})
	}
	left = append(left, Seg{Text: base, Style: Style{Fg: r.Pal.FileFg, Bg: r.Pal.FileBg, Bold: true}})
	if f.Status != patch.Modified {
		left = append(left, Seg{Text: "  " + string(f.Status.Letter()), Style: Style{Fg: r.Pal.FileDim, Bg: r.Pal.FileBg}})
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

// SplitLines returns every visual line of a row, wrapped to the renderer width.
// Both panes wrap independently and are then paired line for line, so a long
// line grows its row instead of being cut off at the seam.
func (r *Renderer) SplitLines(row layout.Row) [][]Seg {
	if row.Kind == layout.LineRow && r.Mode == layout.Split {
		lw, rw, ok := r.splitWidths()
		if !ok {
			return Wrap(r.unifiedFromSplit(row), r.Width)
		}
		left := r.cellLines(row.File, row.Left, true, lw, row.Gutter)
		right := r.cellLines(row.File, row.Right, false, rw, row.Gutter)
		n := len(left)
		if len(right) > n {
			n = len(right)
		}
		out := make([][]Seg, n)
		for i := 0; i < n; i++ {
			var segs []Seg
			if i < len(left) {
				segs = append(segs, left[i]...)
			} else {
				segs = append(segs, blankSegs(lw)...)
			}
			if i < len(right) {
				segs = append(segs, right[i]...)
			} else {
				segs = append(segs, blankSegs(rw)...)
			}
			out[i] = segs
		}
		return out
	}
	return Wrap(r.Segments(row), r.Width)
}

// cellLines wraps one half of a split row into display lines, each exactly w
// columns wide. The line number and sign appear once, on the first line.
func (r *Renderer) cellLines(f *patch.File, c *layout.Cell, old bool, w, gutter int) [][]Seg {
	if c == nil {
		return [][]Seg{blankSegs(w)}
	}
	base := r.baseStyle(signOf(c.Kind))
	lead := r.cellLead(gutter)
	tw := w - lead
	if tw < 1 {
		tw = 1
	}
	text := r.compose(c.Text, c.Emph, base, r.emphStyle(signOf(c.Kind)))
	chunks := Wrap(text, tw)

	out := make([][]Seg, 0, len(chunks))
	for i, chunk := range chunks {
		var segs []Seg
		if i == 0 {
			if r.Numbers {
				segs = append(segs, Seg{Text: padNum(c.No, gutter), Style: Style{Fg: r.Pal.Dim, Bg: base.Bg}})
			}
			sign := signOf(c.Kind)
			segs = append(segs, Seg{Text: string(sign), Style: r.signStyle(sign)})
			segs = append(segs, Seg{Text: " ", Style: base})
		} else {
			segs = append(segs, Seg{Text: strings.Repeat(" ", lead), Style: base})
		}
		segs = append(segs, chunk...)
		out = append(out, Fit(segs, w, base))
	}
	return out
}

func blankSegs(w int) []Seg {
	return []Seg{{Text: strings.Repeat(" ", w)}}
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
		// The empty half of an unpaired change stays blank: a gap is not a
		// change, and painting it made it look like one.
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
