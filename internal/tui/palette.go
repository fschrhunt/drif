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
		{"Search the diff", bind(keys.Search), func(m *Model) tea.Cmd {
			m.typing = true
			m.input = ""
			return nil
		}},
		{"Next file", bind(keys.NextFile), func(m *Model) tea.Cmd { m.jumpRow(layout.FileRow, 1); return nil }},
		{"Previous file", bind(keys.PrevFile), func(m *Model) tea.Cmd { m.jumpRow(layout.FileRow, -1); return nil }},
		{"Next change", bind(keys.NextHunk), func(m *Model) tea.Cmd { m.jumpRow(layout.HunkRow, 1); return nil }},
		{"Previous change", bind(keys.PrevHunk), func(m *Model) tea.Cmd { m.jumpRow(layout.HunkRow, -1); return nil }},
		{"Go to the top", bind(keys.Top), func(m *Model) tea.Cmd { m.top = 0; m.clampTop(); return nil }},
		{"Go to the bottom", bind(keys.Bottom), func(m *Model) tea.Cmd { m.top = m.maxTop(); m.clampTop(); return nil }},
		{"Layout: side by side", "", setMode(layout.Split)},
		{"Layout: unified", "", setMode(layout.Unified)},
		{"Layout: result only", "", setMode(layout.New)},
		{"Theme: dark", "", setTheme("dark")},
		{"Theme: light", "", setTheme("light")},
		{"Theme: follow the terminal", "", setTheme("auto")},
		{"Toggle line numbers", bind(keys.ToggleNumbers), func(m *Model) tea.Cmd {
			m.numbers = !m.numbers
			m.rebuild()
			return nil
		}},
		{"Reload configuration", bind(keys.Reload), func(m *Model) tea.Cmd { return m.reloadConfig() }},
		{"Quit", bind(keys.Quit), func(m *Model) tea.Cmd { return tea.Quit }},
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

// paletteView draws the list over the whole screen. The only furniture is a
// prompt and rows: no border, no title bar, no footer.
func (m *Model) paletteView() string {
	items := m.filtered()
	if len(items) == 0 {
		return render.ANSI([]render.Seg{
			{Text: " ?", Style: render.Style{Fg: m.r.Pal.Dim}},
			{Text: m.palQuery},
			{Text: "▏", Style: render.Style{Fg: m.r.Pal.Dim}},
			{Text: "   no matching command", Style: render.Style{Fg: m.r.Pal.Dim}},
		}, m.r.Depth) + "\n"
	}
	if m.palIndex >= len(items) {
		m.palIndex = len(items) - 1
	}

	var b strings.Builder
	b.WriteString(render.ANSI([]render.Seg{
		{Text: " ?", Style: render.Style{Fg: m.r.Pal.Dim}},
		{Text: m.palQuery},
		{Text: "▏", Style: render.Style{Fg: m.r.Pal.Dim}},
	}, m.r.Depth))
	b.WriteByte('\n')

	for i, item := range items {
		if i >= m.height-1 {
			break
		}
		labelStyle := render.Style{}
		hintStyle := render.Style{Fg: m.r.Pal.Dim}
		if i == m.palIndex {
			labelStyle = render.Style{Reverse: true}
			hintStyle = render.Style{Reverse: true}
		}
		label := " " + item.label
		gap := m.r.Width - render.Width([]render.Seg{{Text: label}}) - len(item.hint) - 2
		if gap < 1 {
			gap = 1
		}
		row := []render.Seg{
			{Text: label, Style: labelStyle},
			{Text: strings.Repeat(" ", gap), Style: labelStyle},
			{Text: item.hint, Style: hintStyle},
			{Text: "  ", Style: hintStyle},
		}
		b.WriteString(render.ANSI(render.Fit(row, m.r.Width, labelStyle), m.r.Depth))
		b.WriteByte('\n')
	}
	return b.String()
}
