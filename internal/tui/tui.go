// Package tui is drift's interactive review surface: a viewport over the same
// rows the pager prints, plus navigation, search and layout toggles.
package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fschrhunt/drift/internal/git"
	"github.com/fschrhunt/drift/internal/highlight"
	"github.com/fschrhunt/drift/internal/layout"
	"github.com/fschrhunt/drift/internal/patch"
	"github.com/fschrhunt/drift/internal/render"
)

// Options configures the initial view.
type Options struct {
	Mode    layout.Mode
	Auto    bool // choose split or unified from the terminal width
	Numbers bool
	Theme   string
	Repo    *git.Repo
	Cache   *highlight.Cache
	Watch   bool
	Reload  func() (*patch.ChangeSet, error)
}

// Model is the bubbletea state.
type Model struct {
	cs    *patch.ChangeSet
	cache *highlight.Cache
	r     *render.Renderer

	mode    layout.Mode
	numbers bool

	width, height int

	rows   []layout.Row
	starts []int // first visual line of each row
	total  int
	top    int

	query   string
	typing  bool
	input   string
	matches []int
	matchAt int

	help    bool
	loading map[string]bool
	wrapped map[int][][]render.Seg

	auto     bool
	userMode bool
	repo     *git.Repo
	watchOn  bool
	watch    *watcher
	reload   func() (*patch.ChangeSet, error)
}

type loadedMsg struct{}

// New builds the model for a change set.
func New(cs *patch.ChangeSet, opts Options) *Model {
	prefix := ""
	if opts.Repo != nil {
		prefix = opts.Repo.Prefix
	}
	return &Model{
		cs:      cs,
		cache:   opts.Cache,
		mode:    opts.Mode,
		numbers: opts.Numbers,
		auto:    opts.Auto,
		repo:    opts.Repo,
		watchOn: opts.Watch,
		reload:  opts.Reload,
		r: &render.Renderer{
			Pal:     render.ThemeByName(opts.Theme),
			Mode:    opts.Mode,
			Depth:   render.DetectDepth(),
			Cache:   opts.Cache,
			Prefix:  prefix,
			Numbers: opts.Numbers,
		},
		loading: map[string]bool{},
		wrapped: map[int][][]render.Seg{},
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	if !m.watchOn || m.repo == nil || m.reload == nil {
		return nil
	}
	w, err := newWatcher(m.repo.Root)
	if err != nil {
		return nil
	}
	m.watch = w
	return w.wait()
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.auto && !m.userMode {
			if m.width >= 100 {
				m.mode = layout.Split
			} else {
				m.mode = layout.Unified
			}
		}
		m.rebuild()
		return m, m.requestVisible()

	case reloadMsg:
		if m.reload != nil {
			idx := m.rowAt(m.top)
			if cs, err := m.reload(); err == nil {
				m.cs = cs
				m.rebuild()
				if idx < len(m.starts) {
					m.top = m.starts[idx]
				}
			}
		}
		m.wrapped = map[int][][]render.Seg{}
		if w := m.watch; w != nil {
			return m, w.wait()
		}
		return m, m.requestVisible()

	case loadedMsg:
		m.wrapped = map[int][][]render.Seg{}
		return m, m.requestVisible()

	case tea.KeyMsg:
		if m.typing {
			return m, m.typingKey(msg)
		}
		return m, m.key(msg)
	}
	return m, nil
}

func (m *Model) key(msg tea.KeyMsg) tea.Cmd {
	body := m.bodyHeight()
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "j", "down":
		m.scroll(1)
	case "k", "up":
		m.scroll(-1)
	case "ctrl+d":
		m.scroll(body / 2)
	case "ctrl+u":
		m.scroll(-body / 2)
	case " ", "f", "pgdown":
		m.scroll(body)
	case "b", "pgup":
		m.scroll(-body)
	case "g", "home":
		m.top = 0
	case "G", "end":
		m.top = m.maxTop()
	case "n":
		if m.query != "" {
			m.jumpMatch(1)
		} else {
			m.jumpRow(layout.FileRow, 1)
		}
	case "N":
		m.jumpMatch(-1)
	case "p":
		m.jumpRow(layout.FileRow, -1)
	case "}", "]":
		m.jumpRow(layout.HunkRow, 1)
	case "{", "[":
		m.jumpRow(layout.HunkRow, -1)
	case "s":
		m.userMode = true
		m.setMode(m.mode.Next())
	case "t":
		m.numbers = !m.numbers
		m.rebuild()
	case "/":
		m.typing = true
		m.input = ""
	case "?":
		m.help = !m.help
	case "esc":
		if m.query != "" {
			m.query = ""
			m.r.Query = ""
			m.matches = nil
			m.wrapped = map[int][][]render.Seg{}
		}
		m.help = false
	}
	m.clampTop()
	return m.requestVisible()
}

func (m *Model) typingKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.typing = false
		m.input = ""
	case "enter":
		m.typing = false
		m.query = m.input
		m.r.Query = m.query
		m.matches = m.findMatches()
		m.matchAt = 0
		m.wrapped = map[int][][]render.Seg{}
		if len(m.matches) > 0 {
			m.jumpMatch(0)
		}
	case "backspace":
		if m.input != "" {
			r := []rune(m.input)
			m.input = string(r[:len(r)-1])
		}
	case "ctrl+c":
		return tea.Quit
	default:
		if msg.Type == tea.KeyRunes {
			m.input += string(msg.Runes)
		}
	}
	return nil
}

func (m *Model) setMode(mode layout.Mode) {
	idx := m.rowAt(m.top)
	m.mode = mode
	m.rebuild()
	if len(m.starts) > 0 {
		if idx >= len(m.starts) {
			idx = len(m.starts) - 1
		}
		m.top = m.starts[idx]
	}
}

func (m *Model) rebuild() {
	if m.width <= 0 {
		return
	}
	m.r.Width = m.width
	m.r.Mode = m.mode
	m.r.Numbers = m.numbers
	m.rows = layout.Build(m.cs, m.mode)
	m.starts = make([]int, len(m.rows))
	n := 0
	for i, row := range m.rows {
		m.starts[i] = n
		n += m.r.Height(row)
	}
	m.total = n
	m.wrapped = map[int][][]render.Seg{}
	m.clampTop()
}

func (m *Model) bodyHeight() int {
	h := m.height - 1
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) maxTop() int {
	t := m.total - m.bodyHeight()
	if t < 0 {
		return 0
	}
	return t
}

func (m *Model) clampTop() {
	if m.top > m.maxTop() {
		m.top = m.maxTop()
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m *Model) scroll(n int) {
	m.top += n
	m.clampTop()
}

// rowAt finds the logical row containing a visual line.
func (m *Model) rowAt(line int) int {
	if len(m.starts) == 0 {
		return 0
	}
	i := sort.Search(len(m.starts), func(i int) bool { return m.starts[i] > line }) - 1
	if i < 0 {
		return 0
	}
	return i
}

func (m *Model) jumpRow(kind layout.Kind, dir int) {
	cur := m.rowAt(m.top)
	for i := cur + dir; i >= 0 && i < len(m.rows); i += dir {
		if m.rows[i].Kind == kind {
			m.top = m.starts[i]
			m.clampTop()
			return
		}
	}
}

func (m *Model) findMatches() []int {
	if m.query == "" {
		return nil
	}
	q := strings.ToLower(m.query)
	var out []int
	for i, row := range m.rows {
		text := row.Text
		if row.Left != nil {
			text += " " + row.Left.Text
		}
		if row.Right != nil {
			text += " " + row.Right.Text
		}
		if row.File != nil && row.Kind == layout.FileRow {
			text = row.File.Path()
		}
		if strings.Contains(strings.ToLower(text), q) {
			out = append(out, i)
		}
	}
	return out
}

func (m *Model) jumpMatch(dir int) {
	if len(m.matches) == 0 {
		return
	}
	if dir != 0 {
		m.matchAt = (m.matchAt + dir + len(m.matches)) % len(m.matches)
	}
	m.top = m.starts[m.matches[m.matchAt]]
	m.clampTop()
}

// requestVisible starts highlighting the files currently on screen. It is a
// no-op when everything visible is already cached, which keeps the loop from
// spinning.
func (m *Model) requestVisible() tea.Cmd {
	if m.cache == nil || m.total == 0 {
		return nil
	}
	type req struct {
		f   *patch.File
		old bool
	}
	var need []req
	seen := map[string]bool{}
	end := m.top + m.bodyHeight()
	for line := m.top; line < end && line < m.total; line++ {
		row := m.rows[m.rowAt(line)]
		if row.File == nil || row.Kind != layout.LineRow {
			continue
		}
		add := func(old bool) {
			if seen[highlight.Key(row.File, old)] || m.cache.Ready(row.File, old) {
				return
			}
			seen[highlight.Key(row.File, old)] = true
			need = append(need, req{row.File, old})
		}
		if m.mode == layout.Split {
			if row.Left != nil {
				add(true)
			}
			if row.Right != nil {
				add(false)
			}
			continue
		}
		add(row.Sign == '-')
	}
	if len(need) == 0 {
		return nil
	}
	for _, n := range need {
		m.loading[highlight.Key(n.f, n.old)] = true
	}
	return func() tea.Msg {
		for _, n := range need {
			m.cache.Table(n.f, n.old)
		}
		return loadedMsg{}
	}
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.help {
		return m.helpView()
	}
	body := m.bodyHeight()
	var b strings.Builder
	for i := 0; i < body; i++ {
		line := m.top + i
		if line < m.total {
			b.WriteString(m.visualLine(line))
		}
		b.WriteString("\x1b[0m")
		b.WriteByte('\n')
	}
	b.WriteString(m.statusBar())
	return b.String()
}

func (m *Model) visualLine(n int) string {
	idx := m.rowAt(n)
	sub := n - m.starts[idx]
	wrapped, ok := m.wrapped[idx]
	if !ok {
		segs := m.r.Segments(m.rows[idx])
		wrapped = render.Wrap(segs, m.r.Width)
		if len(m.wrapped) > 256 {
			m.wrapped = map[int][][]render.Seg{}
		}
		m.wrapped[idx] = wrapped
	}
	if sub < 0 || sub >= len(wrapped) {
		return ""
	}
	return render.ANSI(render.Fit(wrapped[sub], m.r.Width, m.r.Fill(m.rows[idx])), m.r.Depth)
}

func (m *Model) statusBar() string {
	pal := m.r.Pal
	dim := render.Style{Fg: pal.Dim}
	segs := []render.Seg{{Text: " ", Style: dim}}

	if m.typing {
		segs = append(segs,
			render.Seg{Text: "/", Style: render.Style{Fg: pal.Text}},
			render.Seg{Text: m.input, Style: render.Style{Fg: pal.Text}},
			render.Seg{Text: "▏", Style: render.Style{Fg: pal.Accent}},
		)
	} else if m.query != "" {
		pos := 0
		if len(m.matches) > 0 {
			pos = m.matchAt + 1
		}
		segs = append(segs, render.Seg{
			Text:  fmt.Sprintf("/%s %d/%d  ", m.query, pos, len(m.matches)),
			Style: render.Style{Fg: pal.Accent},
		})
	} else {
		row := layout.Row{}
		if len(m.rows) > 0 {
			row = m.rows[m.rowAt(m.top)]
		}
		if row.File != nil {
			path := strings.TrimPrefix(row.File.Path(), m.r.Prefix)
			segs = append(segs, render.Seg{Text: path, Style: render.Style{Fg: pal.Text}})
			if row.File.Additions > 0 {
				segs = append(segs, render.Seg{Text: " +" + itoa(row.File.Additions), Style: render.Style{Fg: pal.AddFg}})
			}
			if row.File.Deletions > 0 {
				segs = append(segs, render.Seg{Text: " -" + itoa(row.File.Deletions), Style: render.Style{Fg: pal.DelFg}})
			}
			segs = append(segs, render.Seg{Text: "  ", Style: dim})
		}
	}

	pct := 0
	if m.maxTop() > 0 {
		pct = m.top * 100 / m.maxTop()
	}
	right := []render.Seg{
		{Text: m.mode.String(), Style: dim},
		{Text: " · ", Style: dim},
		{Text: fmt.Sprintf("%d%%", pct), Style: dim},
		{Text: " · ? help ", Style: dim},
	}
	rightW := render.Width(right)
	left := render.Fit(segs, m.r.Width-rightW, render.Style{})
	return render.ANSI(append(left, right...), m.r.Depth)
}

func (m *Model) helpView() string {
	pal := m.r.Pal
	lines := [][2]string{
		{"j k ↓ ↑", "line"},
		{"ctrl-d ctrl-u", "half page"},
		{"space b", "page"},
		{"g G", "top / bottom"},
		{"n p", "next / previous file"},
		{"} {", "next / previous hunk"},
		{"/", "search"},
		{"n N", "next / previous match (while searching)"},
		{"s", "split → unified → new"},
		{"t", "line numbers"},
		{"esc", "clear search"},
		{"q", "quit"},
	}
	var b strings.Builder
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s\n\n", render.ANSI([]render.Seg{{Text: "drift", Style: render.Style{Fg: pal.Accent, Bold: true}}}, m.r.Depth))
	for _, l := range lines {
		key := render.ANSI([]render.Seg{{Text: l[0], Style: render.Style{Fg: pal.Text, Bold: true}}}, m.r.Depth)
		gap := 18 - render.Width([]render.Seg{{Text: l[0]}})
		if gap < 1 {
			gap = 1
		}
		b.WriteString("  " + key + strings.Repeat(" ", gap) + render.ANSI([]render.Seg{{Text: l[1], Style: render.Style{Fg: pal.Dim}}}, m.r.Depth) + "\n")
	}
	b.WriteString("\n  press ? to close\n")
	return b.String()
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
