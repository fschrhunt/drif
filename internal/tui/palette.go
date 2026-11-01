package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/fschrhunt/drif/internal/keys"
	"github.com/fschrhunt/drif/internal/layout"
	"github.com/fschrhunt/drif/internal/render"
)

// paletteMode is what the panel is for. Both modes share one control: that is
// the point, there is no second place in the interface to type.
type paletteMode int

const (
	paletteCommands paletteMode = iota
	paletteSearch
)

// paletteItem is one row of the panel.
type paletteItem struct {
	glyph string
	label string
	hint  string
	run   func(*Model) tea.Cmd
}

// openPalette shows the panel, empty, in the given mode.
func (m *Model) openPalette(mode paletteMode) {
	m.palette = true
	m.palMode = mode
	m.palQuery = ""
	m.palCursor = 0
	m.palIndex = 0
}

// paletteItems is rebuilt on every open so it reflects the live keymap.
func (m *Model) paletteItems() []paletteItem {
	bind := func(action string) string {
		parts := make([]string, 0, 2)
		for _, k := range m.bind.Action[action] {
			parts = append(parts, keys.Display(k))
		}
		return strings.Join(parts, " ")
	}
	setTheme := func(name string) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			if m.themefn == nil {
				m.message = "themes are unavailable when reading a piped patch"
				return nil
			}
			pal, err := m.themefn(name)
			if err != nil {
				m.message = err.Error()
				return nil
			}
			m.r.Pal = pal
			m.wrapped = map[int][][]render.Seg{}
			return nil
		}
	}
	setMode := func(mode layout.Mode) func(*Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			m.userMode = true
			m.setMode(mode)
			return nil
		}
	}

	return []paletteItem{
		{"⌕", "Search the diff", bind(keys.Search), func(m *Model) tea.Cmd {
			m.openPalette(paletteSearch)
			return nil
		}},
		{"→", "Next file", bind(keys.NextFile), func(m *Model) tea.Cmd { m.jumpRow(layout.FileRow, 1); return nil }},
		{"←", "Previous file", bind(keys.PrevFile), func(m *Model) tea.Cmd { m.jumpRow(layout.FileRow, -1); return nil }},
		{"↕", "Next change", bind(keys.NextHunk), func(m *Model) tea.Cmd { m.jumpRow(layout.HunkRow, 1); return nil }},
		{"↕", "Previous change", bind(keys.PrevHunk), func(m *Model) tea.Cmd { m.jumpRow(layout.HunkRow, -1); return nil }},
		{"↑", "Go to the top", bind(keys.Top), func(m *Model) tea.Cmd { m.top = 0; m.clampTop(); return nil }},
		{"↓", "Go to the bottom", bind(keys.Bottom), func(m *Model) tea.Cmd { m.top = m.maxTop(); m.clampTop(); return nil }},
		{"◫", "Layout: side by side", "", setMode(layout.Split)},
		{"▭", "Layout: unified", "", setMode(layout.Unified)},
		{"▯", "Layout: result only", "", setMode(layout.New)},
		{"◐", "Theme: dark", "", setTheme("dark")},
		{"○", "Theme: light", "", setTheme("light")},
		{"◑", "Theme: follow the terminal", "", setTheme("auto")},
		{"#", "Toggle line numbers", bind(keys.ToggleNumbers), func(m *Model) tea.Cmd {
			m.numbers = !m.numbers
			m.rebuild()
			return nil
		}},
		{"↻", "Reload configuration", bind(keys.Reload), func(m *Model) tea.Cmd { return m.reloadConfig() }},
		{"✕", "Quit", bind(keys.Quit), func(m *Model) tea.Cmd { return tea.Quit }},
	}
}

// rows is what the panel shows right now: filtered commands, or diff lines that
// match the query.
func (m *Model) paletteRows() []paletteItem {
	if m.palMode == paletteSearch {
		return m.searchRows(m.palQuery)
	}
	all := m.paletteItems()
	if m.palQuery == "" {
		return all
	}
	q := strings.ToLower(m.palQuery)
	out := make([]paletteItem, 0, len(all))
	for _, item := range all {
		if strings.Contains(strings.ToLower(item.label), q) {
			out = append(out, item)
		}
	}
	return out
}

// searchRows finds lines whose text contains the query. File headers match on
// their path, so a filename jump works from the same field.
func (m *Model) searchRows(q string) []paletteItem {
	if q == "" {
		return nil
	}
	needle := strings.ToLower(q)
	out := make([]paletteItem, 0, 64)
	for i, row := range m.rows {
		if len(out) >= 200 {
			break
		}
		text := row.Text
		if row.Left != nil {
			text = row.Left.Text
			if row.Right != nil && row.Right.Text != "" {
				text = row.Right.Text
			}
		}
		if row.Kind == layout.FileRow && row.File != nil {
			text = row.File.Path()
		}
		if !strings.Contains(strings.ToLower(text), needle) {
			continue
		}
		no := row.No
		if no == 0 && row.Right != nil {
			no = row.Right.No
		}
		if no == 0 && row.Left != nil {
			no = row.Left.No
		}
		hint := ""
		if row.File != nil {
			base := filepath.Base(row.File.Path())
			if no > 0 {
				hint = fmt.Sprintf("%s:%d", base, no)
			} else {
				hint = base
			}
		}
		row := i
		out = append(out, paletteItem{
			glyph: "·",
			label: strings.TrimSpace(text),
			hint:  hint,
			run: func(m *Model) tea.Cmd {
				m.query = q
				m.r.Query = q
				m.matches = m.findMatches()
				m.matchAt = 0
				for i, idx := range m.matches {
					if idx == row {
						m.matchAt = i
						break
					}
				}
				m.wrapped = map[int][][]render.Seg{}
				m.jumpToRow(row)
				return nil
			},
		})
	}
	return out
}

func (m *Model) paletteKey(msg tea.KeyMsg) tea.Cmd {
	items := m.paletteRows()
	switch msg.String() {
	case "esc":
		m.palette = false
	case "up", "ctrl+p":
		if len(items) > 0 {
			m.palIndex = (m.palIndex - 1 + len(items)) % len(items)
		}
	case "down", "ctrl+n":
		if len(items) > 0 {
			m.palIndex = (m.palIndex + 1) % len(items)
		}
	case "enter":
		if len(items) == 0 {
			return nil
		}
		if m.palIndex >= len(items) {
			m.palIndex = len(items) - 1
		}
		item := items[m.palIndex]
		m.palette = false
		if item.run == nil {
			return nil
		}
		return item.run(m)

	case "left":
		if m.palCursor > 0 {
			m.palCursor--
		}
	case "right":
		if m.palCursor < len([]rune(m.palQuery)) {
			m.palCursor++
		}
	case "home", "ctrl+a":
		m.palCursor = 0
	case "end", "ctrl+e":
		m.palCursor = len([]rune(m.palQuery))
	case "backspace":
		m.deleteAt(-1)
	case "delete":
		m.deleteAt(1)
	case "ctrl+c":
		return tea.Quit
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.insertAt(string(msg.Runes))
		}
	}
	return nil
}

// insertAt puts text at the caret and leaves the caret after it.
func (m *Model) insertAt(text string) {
	r := []rune(m.palQuery)
	if m.palCursor > len(r) {
		m.palCursor = len(r)
	}
	r = append(r[:m.palCursor], append([]rune(text), r[m.palCursor:]...)...)
	m.palQuery = string(r)
	m.palCursor += len([]rune(text))
	m.palIndex = 0
}

// deleteAt removes the rune before the caret (-1) or under it (+1).
func (m *Model) deleteAt(dir int) {
	r := []rune(m.palQuery)
	at := m.palCursor + dir - 1
	if dir < 0 {
		at = m.palCursor - 1
	}
	if at < 0 || at >= len(r) {
		return
	}
	m.palQuery = string(append(r[:at], r[at+1:]...))
	if dir < 0 && m.palCursor > 0 {
		m.palCursor--
	}
	m.palIndex = 0
}

// jumpToRow puts a layout row at the top of the viewport.
func (m *Model) jumpToRow(row int) {
	if row < 0 || row >= len(m.starts) {
		return
	}
	m.top = m.starts[row]
	m.clampTop()
}

// paletteOverlay draws the panel on top of the diff instead of replacing it.
// The rows it covers are rebuilt around the box, so the review surface stays
// visible on either side.
func (m *Model) paletteOverlay(base string) string {
	top, left, boxW, box := m.paletteBox()
	lines := strings.Split(base, "\n")
	for i, row := range box {
		idx := top + i
		if idx < 0 || idx >= len(lines) {
			continue
		}
		lines[idx] = ansi.Truncate(lines[idx], left, "") +
			row +
			ansi.TruncateLeft(lines[idx], left+boxW, "")
	}
	return strings.Join(lines, "\n")
}

// paletteBox lays the panel out and returns where it sits and its lines.
func (m *Model) paletteBox() (top, left, boxW int, lines []string) {
	items := m.paletteRows()
	if len(items) == 0 && m.palQuery != "" {
		label := "No matching command"
		if m.palMode == paletteSearch {
			label = "No matching line"
		}
		items = []paletteItem{{glyph: "·", label: label}}
	}
	if m.palIndex >= len(items) {
		m.palIndex = len(items) - 1
	}

	boxW = m.width - 8
	if boxW > 78 {
		boxW = 78
	}
	if boxW < 40 {
		boxW = m.width - 2
	}
	if boxW < 16 {
		boxW = m.width
	}
	inner := boxW - 2

	maxItems := m.height - 8
	if maxItems < 1 {
		maxItems = 1
	}
	start := 0
	if m.palIndex >= maxItems {
		start = m.palIndex - maxItems + 1
	}
	shown := items[start:]
	if len(shown) > maxItems {
		shown = shown[:maxItems]
	}

	boxH := 7 + len(shown)
	top = (m.height - boxH) / 4
	if top < 0 {
		top = 0
	}
	left = (m.width - boxW) / 2
	if left < 0 {
		left = 0
	}

	lines = append(lines,
		m.boxEdge("╭", inner, "╮"),
		m.boxRow(m.searchSeg(inner), inner),
		m.boxRow(nil, inner),
		m.boxRow(m.sectionSeg(inner), inner),
	)
	for i, item := range shown {
		lines = append(lines, m.boxRow(m.itemSeg(item, i+start == m.palIndex, inner), inner))
	}
	lines = append(lines,
		m.boxRow(nil, inner),
		m.boxRow(m.footerSeg(inner), inner),
		m.boxEdge("╰", inner, "╯"),
	)
	return top, left, boxW, lines
}

// boxRow is a bordered row of the panel, exactly inner+2 columns wide.
func (m *Model) boxRow(content []render.Seg, inner int) string {
	edge := render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}
	content = render.Fit(content, inner, render.Style{Bg: m.r.Pal.Panel})
	segs := []render.Seg{{Text: "│", Style: edge}}
	segs = append(segs, content...)
	segs = append(segs, render.Seg{Text: "│", Style: edge})
	return render.ANSI(segs, m.r.Depth)
}

func (m *Model) boxEdge(left string, n int, right string) string {
	if n < 0 {
		n = 0
	}
	edge := render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}
	return render.ANSI([]render.Seg{{Text: left + strings.Repeat("─", n) + right, Style: edge}}, m.r.Depth)
}

func (m *Model) panelPad(segs []render.Seg, w int) []render.Seg {
	return render.Fit(segs, w, render.Style{Bg: m.r.Pal.Panel})
}

func (m *Model) searchSeg(inner int) []render.Seg {
	dim := render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}
	text := render.Style{Bg: m.r.Pal.Panel}
	caret := render.Style{Fg: m.r.Pal.FileFg, Bg: m.r.Pal.Panel}

	r := []rune(m.palQuery)
	at := m.palCursor
	if at > len(r) {
		at = len(r)
	}
	segs := []render.Seg{{Text: " ⌕ ", Style: dim}}
	if m.palQuery == "" {
		segs = append(segs,
			render.Seg{Text: "Search", Style: dim},
			render.Seg{Text: "▏", Style: caret},
		)
	} else {
		segs = append(segs,
			render.Seg{Text: string(r[:at]), Style: text},
			render.Seg{Text: "▏", Style: caret},
			render.Seg{Text: string(r[at:]), Style: text},
		)
	}
	return m.panelPad(segs, inner)
}

func (m *Model) sectionSeg(inner int) []render.Seg {
	label := "  Commands"
	if m.palMode == paletteSearch {
		label = "  Lines"
	}
	segs := []render.Seg{{Text: label, Style: render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}}}
	return m.panelPad(segs, inner)
}

func (m *Model) footerSeg(inner int) []render.Seg {
	hint := "  ↑↓ navigate   ↵ select   esc close"
	if m.palMode == paletteSearch {
		hint = "  ↑↓ navigate   ↵ jump   esc close"
	}
	segs := []render.Seg{{Text: hint, Style: render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}}}
	return m.panelPad(segs, inner)
}

func (m *Model) itemSeg(item paletteItem, selected bool, inner int) []render.Seg {
	bg := m.r.Pal.Panel
	if selected {
		bg = m.r.Pal.PanelSel
	}
	base := render.Style{Bg: bg}
	label := render.Style{Bg: bg}
	hint := render.Style{Fg: m.r.Pal.Dim, Bg: bg}

	left := " " + item.glyph + "  "
	// Cut the label, never the binding: a command you can't read is worse than
	// a line of diff you can't read.
	labelMax := inner - render.Width([]render.Seg{{Text: left}}) - render.Width([]render.Seg{{Text: item.hint}}) - 3
	if labelMax < 4 {
		labelMax = 4
	}
	item.label = ansi.Truncate(item.label, labelMax, "…")
	gap := inner - render.Width([]render.Seg{{Text: left}}) - render.Width([]render.Seg{{Text: item.label}}) - render.Width([]render.Seg{{Text: item.hint}}) - 2
	if gap < 1 {
		gap = 1
	}
	segs := []render.Seg{
		{Text: left, Style: base},
		{Text: item.label, Style: label},
		{Text: strings.Repeat(" ", gap), Style: base},
		{Text: item.hint, Style: hint},
		{Text: " ", Style: base},
	}
	return render.Fit(segs, inner, base)
}
