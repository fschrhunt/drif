// Package tui is drif's interactive review surface: a viewport over the same
// rows the pager prints, plus navigation, search, layout toggles and a command
// line for changing the running configuration.
package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fschrhunt/drif/internal/expand"
	"github.com/fschrhunt/drif/internal/git"
	"github.com/fschrhunt/drif/internal/highlight"
	"github.com/fschrhunt/drif/internal/keys"
	"github.com/fschrhunt/drif/internal/layout"
	"github.com/fschrhunt/drif/internal/patch"
	"github.com/fschrhunt/drif/internal/render"
)

// Refresh is a re-read of the configuration while the view is running, which
// is what makes editing a theme file and pressing R work.
type Refresh struct {
	Palette *render.Palette
	Bind    *keys.Map
	Mode    layout.Mode
	Auto    bool
	Numbers bool
}

// Options configures the initial view.
type Options struct {
	Mode    layout.Mode
	Auto    bool // choose split or unified from the terminal width
	Numbers bool
	Palette *render.Palette
	Bind    *keys.Map
	Repo    *git.Repo
	Cache   *highlight.Cache
	Watch   bool
	// Reload re-reads the working tree.
	Reload func() (*patch.ChangeSet, error)
	// Refresh re-reads the configuration.
	Refresh func() (*Refresh, error)
	// Theme resolves a theme name for :theme.
	Theme func(string) (*render.Palette, error)
}

// Model is the bubbletea state.
type Model struct {
	cs    *patch.ChangeSet
	cache *highlight.Cache
	r     *render.Renderer

	mode    layout.Mode
	numbers bool
	bind    *keys.Map

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

	cmdMode  bool
	cmdInput string
	message  string

	help    bool
	loading map[string]bool
	wrapped map[int][][]render.Seg

	auto     bool
	userMode bool
	repo     *git.Repo
	watchOn  bool
	watch    *watcher
	reload   func() (*patch.ChangeSet, error)
	refresh  func() (*Refresh, error)
	themefn  func(string) (*render.Palette, error)
}

type loadedMsg struct{ keys []string }

// New builds the model for a change set.
func New(cs *patch.ChangeSet, opts Options) *Model {
	prefix := ""
	if opts.Repo != nil {
		prefix = opts.Repo.Prefix
	}
	pal := opts.Palette
	if pal == nil {
		pal = render.DetectPalette()
	}
	bind := opts.Bind
	if bind == nil {
		bind, _ = keys.Build(nil)
	}
	return &Model{
		cs:      cs,
		cache:   opts.Cache,
		mode:    opts.Mode,
		numbers: opts.Numbers,
		bind:    bind,
		auto:    opts.Auto,
		repo:    opts.Repo,
		watchOn: opts.Watch,
		reload:  opts.Reload,
		refresh: opts.Refresh,
		themefn: opts.Theme,
		r: &render.Renderer{
			Pal:     pal,
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
			m.chooseAuto()
		}
		m.rebuild()
		return m, m.requestVisible()

	case loadedMsg:
		for _, k := range msg.keys {
			delete(m.loading, k)
		}
		m.wrapped = map[int][][]render.Seg{}
		return m, m.requestVisible()

	case reloadMsg:
		m.reloadTree()
		m.wrapped = map[int][][]render.Seg{}
		if w := m.watch; w != nil {
			return m, w.wait()
		}
		return m, m.requestVisible()

	case tea.KeyMsg:
		switch {
		case m.typing:
			return m, m.typingKey(msg)
		case m.cmdMode:
			return m, m.commandKey(msg)
		}
		return m, m.key(msg)
	}
	return m, nil
}

func (m *Model) chooseAuto() {
	if m.width >= 100 {
		m.mode = layout.Split
	} else {
		m.mode = layout.Unified
	}
}

func (m *Model) key(msg tea.KeyMsg) tea.Cmd {
	m.message = ""
	if msg.String() == "esc" {
		m.clearSearch()
		m.help = false
		return m.requestVisible()
	}

	action, ok := m.bind.Key[msg.String()]
	if !ok {
		return m.requestVisible()
	}
	body := m.bodyHeight()
	switch action {
	case keys.Quit:
		return tea.Quit
	case keys.Down:
		m.scroll(1)
	case keys.Up:
		m.scroll(-1)
	case keys.HalfDown:
		m.scroll(body / 2)
	case keys.HalfUp:
		m.scroll(-body / 2)
	case keys.PageDown:
		m.scroll(body)
	case keys.PageUp:
		m.scroll(-body)
	case keys.Top:
		m.top = 0
	case keys.Bottom:
		m.top = m.maxTop()
	case keys.NextFile:
		m.jumpRow(layout.FileRow, 1)
	case keys.PrevFile:
		m.jumpRow(layout.FileRow, -1)
	case keys.NextHunk:
		m.jumpRow(layout.HunkRow, 1)
	case keys.PrevHunk:
		m.jumpRow(layout.HunkRow, -1)
	case keys.CycleLayout:
		m.userMode = true
		m.setMode(m.mode.Next())
	case keys.ToggleNumbers:
		m.numbers = !m.numbers
		m.rebuild()
	case keys.Search:
		m.typing = true
		m.input = ""
	case keys.NextMatch:
		m.jumpMatch(1)
	case keys.PrevMatch:
		m.jumpMatch(-1)
	case keys.Help:
		m.help = !m.help
	case keys.Command:
		m.cmdMode = true
		m.cmdInput = ""
	case keys.Reload:
		return m.reloadConfig()
	}
	m.clampTop()
	return m.requestVisible()
}

func (m *Model) clearSearch() {
	if m.query == "" {
		return
	}
	m.query = ""
	m.r.Query = ""
	m.matches = nil
	m.wrapped = map[int][][]render.Seg{}
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
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.input += string(msg.Runes)
		}
	}
	return nil
}

func (m *Model) commandKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.cmdMode = false
		m.cmdInput = ""
	case "enter":
		line := m.cmdInput
		m.cmdMode = false
		m.cmdInput = ""
		return m.exec(line)
	case "backspace":
		if m.cmdInput != "" {
			r := []rune(m.cmdInput)
			m.cmdInput = string(r[:len(r)-1])
		}
	case "ctrl+c":
		return tea.Quit
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.cmdInput += string(msg.Runes)
		}
	}
	return nil
}

// exec runs one colon command. The set is deliberately small: everything that
// changes the view, and nothing that changes the repository.
func (m *Model) exec(line string) tea.Cmd {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "q", "quit":
		return tea.Quit

	case "help", "h":
		m.help = true

	case "theme":
		if len(fields) != 2 {
			m.message = "usage: :theme <name>"
			return nil
		}
		if m.themefn == nil {
			m.message = "themes are unavailable when reading a piped patch"
			return nil
		}
		pal, err := m.themefn(fields[1])
		if err != nil {
			m.message = err.Error()
			return nil
		}
		m.r.Pal = pal
		m.wrapped = map[int][][]render.Seg{}
		m.message = "theme " + fields[1]

	case "layout":
		if len(fields) != 2 {
			m.message = "usage: :layout split|unified|new"
			return nil
		}
		switch fields[1] {
		case "split":
			m.setMode(layout.Split)
		case "unified":
			m.setMode(layout.Unified)
		case "new":
			m.setMode(layout.New)
		default:
			m.message = "usage: :layout split|unified|new"
			return nil
		}
		m.userMode = true

	case "numbers":
		m.numbers = !m.numbers
		m.rebuild()

	case "reload", "e":
		return m.reloadConfig()

	default:
		m.message = "unknown command: " + fields[0]
	}
	return nil
}

// reloadConfig re-reads the config file and applies it without losing the
// place in the diff.
func (m *Model) reloadConfig() tea.Cmd {
	if m.refresh == nil {
		m.message = "no config file to reload"
		return nil
	}
	next, err := m.refresh()
	if err != nil {
		m.message = err.Error()
		return nil
	}
	idx := m.rowAt(m.top)
	m.r.Pal = next.Palette
	m.bind = next.Bind
	m.numbers = next.Numbers
	m.auto = next.Auto
	if next.Auto {
		m.userMode = false
		m.chooseAuto()
	} else {
		m.userMode = true
		m.mode = next.Mode
	}
	m.rebuild()
	if idx < len(m.starts) {
		m.top = m.starts[idx]
	}
	m.message = "config reloaded"
	return m.requestVisible()
}

// reloadTree re-reads the working tree, keeping the reader roughly in place.
func (m *Model) reloadTree() {
	if m.reload == nil {
		return
	}
	idx := m.rowAt(m.top)
	cs, err := m.reload()
	if err != nil {
		m.message = err.Error()
		return
	}
	m.cs = cs
	m.rebuild()
	if m.cache != nil {
		m.cache.Reset(cs.MergeBase)
	}
	m.loading = map[string]bool{}
	if idx < len(m.starts) {
		m.top = m.starts[idx]
	}
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
			key := highlight.Key(row.File, old)
			if seen[key] || m.loading[key] || m.cache.Ready(row.File, old) {
				return
			}
			seen[key] = true
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
	keys := make([]string, 0, len(need))
	for _, n := range need {
		key := highlight.Key(n.f, n.old)
		m.loading[key] = true
		keys = append(keys, key)
	}
	return func() tea.Msg {
		for _, n := range need {
			m.cache.Table(n.f, n.old)
		}
		return loadedMsg{keys: keys}
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

	switch {
	case m.cmdMode:
		segs = append(segs,
			render.Seg{Text: ":", Style: render.Style{Fg: pal.Accent, Bold: true}},
			render.Seg{Text: m.cmdInput, Style: render.Style{Fg: pal.Text}},
			render.Seg{Text: "▏", Style: render.Style{Fg: pal.Accent}},
		)
	case m.typing:
		segs = append(segs,
			render.Seg{Text: "/", Style: render.Style{Fg: pal.Text}},
			render.Seg{Text: m.input, Style: render.Style{Fg: pal.Text}},
			render.Seg{Text: "▏", Style: render.Style{Fg: pal.Accent}},
		)
	case m.query != "":
		pos := 0
		if len(m.matches) > 0 {
			pos = m.matchAt + 1
		}
		segs = append(segs, render.Seg{
			Text:  fmt.Sprintf("/%s %d/%d  ", m.query, pos, len(m.matches)),
			Style: render.Style{Fg: pal.Accent},
		})
	case m.message != "":
		segs = append(segs, render.Seg{Text: m.message, Style: render.Style{Fg: pal.Accent}})
	default:
		row := layout.Row{}
		if len(m.rows) > 0 {
			row = m.rows[m.rowAt(m.top)]
		}
		if row.File != nil {
			path := expand.Expand(strings.TrimPrefix(row.File.Path(), m.r.Prefix))
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

	var right []render.Seg
	// A message or an open prompt gets the whole line: nothing is more useless
	// than a diagnostic clipped in half by a scroll percentage.
	if m.message == "" && !m.cmdMode && !m.typing {
		pct := 0
		if m.maxTop() > 0 {
			pct = m.top * 100 / m.maxTop()
		}
		right = []render.Seg{
			{Text: m.mode.String(), Style: dim},
			{Text: " · ", Style: dim},
			{Text: fmt.Sprintf("%d%%", pct), Style: dim},
			{Text: " · ? help ", Style: dim},
		}
	}
	avail := m.r.Width - render.Width(right)
	segs = clipSegs(segs, avail)
	return render.ANSI(append(segs, right...), m.r.Depth)
}

// clipSegs truncates to w columns and marks the cut, so a long path or error
// reads as truncated instead of as a broken tail.
func clipSegs(segs []render.Seg, w int) []render.Seg {
	if w <= 0 {
		return nil
	}
	if render.Width(segs) <= w {
		return segs
	}
	out := render.Fit(segs[:len(segs):len(segs)], w-1, render.Style{})
	out = append(out, render.Seg{Text: "…", Style: lastStyle(segs)})
	return out
}

func lastStyle(segs []render.Seg) render.Style {
	if len(segs) == 0 {
		return render.Style{}
	}
	return segs[len(segs)-1].Style
}

// helpRows pairs the actions shown in help with a description. The keys come
// from the live keymap, so rebinding changes the help too.
var helpRows = []struct {
	actions []string
	what    string
}{
	{[]string{keys.Down, keys.Up}, "line"},
	{[]string{keys.HalfDown, keys.HalfUp}, "half page"},
	{[]string{keys.PageDown, keys.PageUp}, "page"},
	{[]string{keys.Top, keys.Bottom}, "top / bottom"},
	{[]string{keys.NextFile, keys.PrevFile}, "next / previous file"},
	{[]string{keys.NextHunk, keys.PrevHunk}, "next / previous hunk"},
	{[]string{keys.Search}, "search"},
	{[]string{keys.NextMatch, keys.PrevMatch}, "next / previous match"},
	{[]string{keys.CycleLayout}, "split → unified → new"},
	{[]string{keys.ToggleNumbers}, "line numbers"},
	{[]string{keys.Command}, "command"},
	{[]string{keys.Reload}, "reload config"},
	{[]string{keys.Help}, "help"},
	{[]string{keys.Quit}, "quit"},
}

func (m *Model) helpView() string {
	pal := m.r.Pal
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(render.ANSI([]render.Seg{{Text: "drif", Style: render.Style{Fg: pal.Accent, Bold: true}}}, m.r.Depth))
	b.WriteString("\n\n")
	for _, row := range helpRows {
		label := m.bindings(row.actions...)
		gap := 28 - len(label)
		if gap < 1 {
			gap = 1
		}
		b.WriteString("  ")
		b.WriteString(render.ANSI([]render.Seg{{Text: label, Style: render.Style{Fg: pal.Text}}}, m.r.Depth))
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(render.ANSI([]render.Seg{{Text: row.what, Style: render.Style{Fg: pal.Dim}}}, m.r.Depth))
		b.WriteString("\n")
	}
	b.WriteString("\n  ")
	b.WriteString(render.ANSI([]render.Seg{{Text: ":theme <name>  :layout split|unified|new  :numbers  :reload  :q", Style: render.Style{Fg: pal.Dim}}}, m.r.Depth))
	b.WriteString("\n\n  press ? to close\n")
	return b.String()
}

// bindings lists the keys currently bound to the given actions, in keymap
// order, so help reflects a rebinding immediately.
func (m *Model) bindings(actions ...string) string {
	var out []string
	for _, action := range actions {
		for _, key := range m.bind.Action[action] {
			out = append(out, keys.Display(key))
		}
	}
	return strings.Join(out, " ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
