// Package layout turns a parsed change set into the rows every renderer
// draws. Rows are independent of terminal width: wrapping and truncation
// happen at draw time, which keeps the pager, the TUI and the plain output
// from drifing apart.
package layout

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fschrhunt/drif/internal/patch"
)

// Mode is how the change is presented.
type Mode uint8

const (
	// Unified interleaves deletions and additions in one column.
	Unified Mode = iota
	// Split compares the old file on the left with the new one on the right.
	Split
	// New shows only the resulting file, with deletions left out.
	New
)

func (m Mode) String() string {
	switch m {
	case Split:
		return "split"
	case New:
		return "new"
	}
	return "unified"
}

// ModeNames lists the modes in the order the toggle key cycles them.
var ModeNames = []Mode{Split, Unified, New}

// Next returns the mode reached by pressing the toggle key.
func (m Mode) Next() Mode {
	for i, v := range ModeNames {
		if v == m {
			return ModeNames[(i+1)%len(ModeNames)]
		}
	}
	return Unified
}

// Kind is what a row represents.
type Kind uint8

const (
	FileRow Kind = iota
	MetaRow
	HunkRow
	LineRow
)

// Span is a byte range inside a row or cell text.
type Span struct {
	Start, End int
}

// Cell is one side of a split row: an old or new file line.
type Cell struct {
	No   int
	Text string
	Kind patch.LineKind
	Emph []Span
}

// Row is one logical line of the review surface.
type Row struct {
	Kind  Kind
	File  *patch.File
	Hunk  *patch.Hunk
	Text  string
	Sign  byte
	No    int
	Emph  []Span
	Left  *Cell
	Right *Cell
	// Gutter is how many digits this file's line numbers need.
	Gutter int
}

// Build lays out an entire change set.
func Build(cs *patch.ChangeSet, mode Mode) []Row {
	rows := make([]Row, 0, 1024)
	for _, f := range cs.Files {
		rows = append(rows, fileRows(f, mode)...)
	}
	return rows
}

func fileRows(f *patch.File, mode Mode) []Row {
	gutter := gutterWidth(f)
	rows := []Row{{Kind: FileRow, File: f, Gutter: gutter}}

	for _, meta := range MetaLines(f) {
		rows = append(rows, Row{Kind: MetaRow, File: f, Text: meta, Gutter: gutter})
	}
	if f.Binary || len(f.Hunks) == 0 {
		return rows
	}

	for i := range f.Hunks {
		h := &f.Hunks[i]
		rows = append(rows, Row{Kind: HunkRow, File: f, Hunk: h, Text: HunkHeader(h), Gutter: gutter})

		ops := walk(h)
		if mode == Split {
			rows = append(rows, splitRows(f, ops, gutter)...)
			continue
		}
		for _, o := range ops {
			if mode == New && o.kind == patch.Del {
				continue
			}
			sign := byte(' ')
			switch o.kind {
			case patch.Add:
				sign = '+'
			case patch.Del:
				sign = '-'
			}
			no := o.line.NewNo
			if o.kind == patch.Del {
				no = o.line.OldNo
			}
			rows = append(rows, Row{
				Kind: LineRow, File: f, Hunk: h, Text: Expand(o.line.Text),
				Sign: sign, No: no, Emph: o.emph, Gutter: gutter,
			})
		}
	}
	return rows
}

// op is a line with the emphasis computed against its counterpart.
type op struct {
	kind patch.LineKind
	line patch.Line
	emph []Span
}

// walk groups a hunk into context lines and deletion/addition runs, pairing
// each run one-to-one so intra-line emphasis can be computed once.
func walk(h *patch.Hunk) []op {
	ops := make([]op, 0, len(h.Lines))
	lines := h.Lines
	i := 0
	for i < len(lines) {
		switch lines[i].Kind {
		case patch.NoNewline:
			i++
		case patch.Context:
			ops = append(ops, op{kind: patch.Context, line: lines[i]})
			i++
		default:
			start := i
			for i < len(lines) && lines[i].Kind == patch.Del {
				i++
			}
			delEnd := i
			for i < len(lines) && lines[i].Kind == patch.Add {
				i++
			}
			addEnd := i
			if start == i {
				i++ // nothing we model; never loop forever
				continue
			}
			dels := lines[start:delEnd]
			adds := lines[delEnd:addEnd]

			delEmph := make([][]Span, len(dels))
			addEmph := make([][]Span, len(adds))
			for k := 0; k < len(dels) && k < len(adds); k++ {
				delEmph[k], addEmph[k] = inline(Expand(dels[k].Text), Expand(adds[k].Text))
			}
			for k := range dels {
				ops = append(ops, op{kind: patch.Del, line: dels[k], emph: delEmph[k]})
			}
			for k := range adds {
				ops = append(ops, op{kind: patch.Add, line: adds[k], emph: addEmph[k]})
			}
		}
	}
	return ops
}

func splitRows(f *patch.File, ops []op, gutter int) []Row {
	rows := make([]Row, 0, len(ops))
	i := 0
	for i < len(ops) {
		if ops[i].kind == patch.Context {
			o := ops[i]
			text := Expand(o.line.Text)
			rows = append(rows, Row{
				Kind: LineRow, File: f, Gutter: gutter,
				Left:  &Cell{No: o.line.OldNo, Text: text, Kind: patch.Context},
				Right: &Cell{No: o.line.NewNo, Text: text, Kind: patch.Context},
			})
			i++
			continue
		}
		// Collect the deletion run and the addition run that follows it.
		var dels, adds []op
		for i < len(ops) && ops[i].kind == patch.Del {
			dels = append(dels, ops[i])
			i++
		}
		for i < len(ops) && ops[i].kind == patch.Add {
			adds = append(adds, ops[i])
			i++
		}
		n := len(dels)
		if len(adds) > n {
			n = len(adds)
		}
		for k := 0; k < n; k++ {
			r := Row{Kind: LineRow, File: f, Gutter: gutter}
			if k < len(dels) {
				d := dels[k]
				r.Left = &Cell{No: d.line.OldNo, Text: Expand(d.line.Text), Kind: patch.Del, Emph: d.emph}
			}
			if k < len(adds) {
				a := adds[k]
				r.Right = &Cell{No: a.line.NewNo, Text: Expand(a.line.Text), Kind: patch.Add, Emph: a.emph}
			}
			rows = append(rows, r)
		}
	}
	return rows
}

func gutterWidth(f *patch.File) int {
	max := 1
	for _, h := range f.Hunks {
		if v := h.OldStart + h.OldCount; v > max {
			max = v
		}
		if v := h.NewStart + h.NewCount; v > max {
			max = v
		}
	}
	return len(strconv.Itoa(max))
}

// HunkHeader is the "@@ -a,b +c,d @@ section" line for a hunk.
func HunkHeader(h *patch.Hunk) string {
	s := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	if h.Section != "" {
		s += " " + h.Section
	}
	return s
}

// MetaLines describes what changed about a file beyond its text: a rename, a
// mode change, a binary body. Both the styled renderers and the plain output
// use it, so an agent sees the same facts a person does.
func MetaLines(f *patch.File) []string {
	var out []string
	switch f.Status {
	case patch.Renamed:
		out = append(out, "renamed from "+f.OldPath)
	case patch.Copied:
		out = append(out, "copied from "+f.OldPath)
	}
	if f.OldMode != "" && f.NewMode != "" && f.OldMode != f.NewMode {
		out = append(out, fmt.Sprintf("mode %s → %s", f.OldMode, f.NewMode))
	}
	if f.Binary {
		out = append(out, "binary file")
	}
	if f.Note != "" {
		out = append(out, f.Note)
	}
	return out
}

// TabWidth is how many columns a tab occupies. Source that indents with tabs
// would otherwise jump to the terminal's tab stops after the gutter, which
// makes every indentation level look distorted.
const TabWidth = 4

// Expand replaces tabs with spaces. Line content and syntax spans are both
// expanded, so byte offsets stay consistent between them.
func Expand(s string) string {
	if !strings.ContainsRune(s, '\t') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	for _, c := range s {
		if c == '\t' {
			b.WriteString("    ")
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// inline marks the byte ranges of a and b that differ, at word granularity.
// It gives up on lines that are too large or too dissimilar for the result to
// be useful.
func inline(a, b string) ([]Span, []Span) {
	if a == "" || b == "" || a == b {
		return nil, nil
	}
	at, bt := tokenize(a), tokenize(b)
	if len(at)*len(bt) > 250_000 {
		return nil, nil
	}
	ma, mb := commonTokens(at, bt)

	matched := 0
	for _, t := range at {
		if ma[t.start] {
			matched += t.end - t.start
		}
	}
	// Similarity is measured over the union so a heavily rewritten line is
	// shown as one undivided change rather than a confetti of fragments.
	if 2*matched < (len(a)+len(b))/2 {
		return nil, nil
	}
	return spans(at, ma), spans(bt, mb)
}

type token struct {
	start, end int
	text       string
}

func tokenize(s string) []token {
	var out []token
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		start := i
		switch {
		case unicode.IsSpace(r):
			for i < len(s) {
				r2, sz := utf8.DecodeRuneInString(s[i:])
				if !unicode.IsSpace(r2) {
					break
				}
				i += sz
			}
		case isWord(r):
			for i < len(s) {
				r2, sz := utf8.DecodeRuneInString(s[i:])
				if !isWord(r2) {
					break
				}
				i += sz
			}
		default:
			i += size
		}
		out = append(out, token{start: start, end: i, text: s[start:i]})
	}
	return out
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// commonTokens marks the tokens that belong to a longest common subsequence,
// keyed by their byte start.
func commonTokens(a, b []token) (map[int]bool, map[int]bool) {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i].text == b[j].text {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	ma, mb := map[int]bool{}, map[int]bool{}
	for i, j := 0, 0; i < n && j < m; {
		if a[i].text == b[j].text {
			ma[a[i].start] = true
			mb[b[j].start] = true
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			i++
		} else {
			j++
		}
	}
	return ma, mb
}

// spans converts unmatched tokens into merged byte ranges.
func spans(tokens []token, marked map[int]bool) []Span {
	var out []Span
	for _, t := range tokens {
		if marked[t.start] {
			continue
		}
		if n := len(out); n > 0 && out[n-1].End == t.start {
			out[n-1].End = t.end
			continue
		}
		out = append(out, Span{Start: t.start, End: t.end})
	}
	return out
}
