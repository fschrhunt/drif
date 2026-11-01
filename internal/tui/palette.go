package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fschrhunt/drif/internal/keys"
	"github.com/fschrhunt/drif/internal/layout"
	"github.com/fschrhunt/drif/internal/render"
)

// paletteItem is one thing the command palette can do. There is no separate
// help screen: this list is the only place drif explains itself, which is why
// nothing else on screen carries key hints.
type paletteItem struct {
	glyph string
	label string
	hint  string
	run   func(*Model) tea.Cmd
}

// openPalette shows the list, unfiltered, with the selection on the first row.
func (m *Model) openPalette() {
	m.palette = true
	m.palQuery = ""
	m.palIndex = 0
}

// paletteItems is rebuilt on every open so it reflects the live keymap and the
// current configuration.
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
			m.typing = true
			m.input = ""
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

// filtered is the list narrowed by the query, in its original order.
func (m *Model) filtered() []paletteItem {
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

func (m *Model) paletteKey(msg tea.KeyMsg) tea.Cmd {
	items := m.filtered()
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
	case "backspace":
		if m.palQuery != "" {
			r := []rune(m.palQuery)
			m.palQuery = string(r[:len(r)-1])
			m.palIndex = 0
		}
	case "ctrl+c":
		return tea.Quit
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.palQuery += string(msg.Runes)
			m.palIndex = 0
		}
	}
	return nil
}

// paletteView draws the palette as a floating panel over a blank page: search
// field, a section label, rows with their bindings, and a footer with the three
// keys that navigate it. Nothing here is printed anywhere else.
func (m *Model) paletteView() string {
	items := m.filtered()
	if len(items) == 0 {
		items = []paletteItem{{"·", "No matching command", "", nil}}
	}
	if m.palIndex >= len(items) {
		m.palIndex = len(items) - 1
	}

	width := m.width
	boxW := width - 8
	if boxW > 78 {
		boxW = 78
	}
	if boxW < 40 {
		boxW = width - 2
	}
	if boxW < 16 {
		boxW = width
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
	top := (m.height - boxH) / 4
	if top < 0 {
		top = 0
	}
	left := (width - boxW) / 2
	if left < 0 {
		left = 0
	}

	lines := make([]string, 0, m.height)
	blank := strings.Repeat(" ", width)
	for i := 0; i < top && len(lines) < m.height; i++ {
		lines = append(lines, blank)
	}

	lines = append(lines,
		m.panelEdge(left, width, m.borderSeg("╭", inner, "╮")),
		m.panelLine(left, width, m.searchSeg(inner)),
		m.panelLine(left, width, m.panelPad(nil, inner)),
		m.panelLine(left, width, m.sectionSeg(inner)),
	)
	for i, item := range shown {
		lines = append(lines, m.panelLine(left, width, m.itemSeg(item, i+start == m.palIndex, inner)))
	}
	lines = append(lines,
		m.panelLine(left, width, m.panelPad(nil, inner)),
		m.panelLine(left, width, m.footerSeg(inner)),
		m.panelEdge(left, width, m.borderSeg("╰", inner, "╯")),
	)
	for len(lines) < m.height {
		lines = append(lines, blank)
	}
	return strings.Join(lines[:m.height], "\n")
}

// panelEdge draws a top or bottom border row, which brings its own corners.
func (m *Model) panelEdge(left, width int, content []render.Seg) string {
	right := width - left - render.Width(content)
	if right < 0 {
		right = 0
	}
	segs := []render.Seg{{Text: strings.Repeat(" ", left)}}
	segs = append(segs, content...)
	segs = append(segs, render.Seg{Text: strings.Repeat(" ", right)})
	return render.ANSI(segs, m.r.Depth)
}

// panelLine frames one row of the panel: border, content, border, centred.
func (m *Model) panelLine(left, width int, content []render.Seg) string {
	edge := render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}
	pad := strings.Repeat(" ", left)
	right := strings.Repeat(" ", width-left-render.Width(content)-2)
	if len(right) < 0 {
		right = ""
	}
	segs := []render.Seg{
		{Text: pad},
		{Text: "│", Style: edge},
	}
	segs = append(segs, content...)
	segs = append(segs, render.Seg{Text: "│", Style: edge}, render.Seg{Text: right})
	return render.ANSI(segs, m.r.Depth)
}

func (m *Model) borderSeg(left string, n int, right string) []render.Seg {
	if n < 0 {
		n = 0
	}
	edge := render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}
	return []render.Seg{{Text: left + strings.Repeat("─", n) + right, Style: edge}}
}

func (m *Model) panelPad(segs []render.Seg, w int) []render.Seg {
	return render.Fit(segs, w, render.Style{Bg: m.r.Pal.Panel})
}

func (m *Model) searchSeg(inner int) []render.Seg {
	dim := render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}
	text := render.Style{Bg: m.r.Pal.Panel}
	segs := []render.Seg{
		{Text: " ⌕ ", Style: dim},
	}
	if m.palQuery == "" {
		segs = append(segs,
			render.Seg{Text: "Search commands", Style: dim},
			render.Seg{Text: "▏", Style: text},
		)
	} else {
		segs = append(segs,
			render.Seg{Text: m.palQuery, Style: text},
			render.Seg{Text: "▏", Style: text},
		)
	}
	return m.panelPad(segs, inner)
}

func (m *Model) sectionSeg(inner int) []render.Seg {
	segs := []render.Seg{{Text: "  Commands", Style: render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel}}}
	return m.panelPad(segs, inner)
}

func (m *Model) footerSeg(inner int) []render.Seg {
	segs := []render.Seg{{
		Text:  "  ↑↓ navigate   ↵ select   esc close",
		Style: render.Style{Fg: m.r.Pal.Dim, Bg: m.r.Pal.Panel},
	}}
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
