// Package tui is drif's interactive review surface: a viewport over the same
// rows the pager prints, plus navigation, search, layout toggles and a command
// line for changing the running configuration.
package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fschrhunt/drif/internal/git"
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
	cs *patch.ChangeSet
	r  *render.Renderer

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

	palette  bool
	palQuery string
	palIndex int
	wrapped  map[int][][]render.Seg

	auto     bool
	userMode bool
	repo     *git.Repo
	watchOn  bool
	watch    *watcher
	reload   func() (*patch.ChangeSet, error)
	refresh  func() (*Refresh, error)
	themefn  func(string) (*render.Palette, error)
}

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
			Prefix:  prefix,
			Numbers: opts.Numbers,
		},
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
		return m, nil

	case reloadMsg:
		m.reloadTree()
		m.wrapped = map[int][][]render.Seg{}
		if w := m.watch; w != nil {
			return m, w.wait()
		}
		return m, nil

	case tea.KeyMsg:
		switch {
		case m.palette:
			return m, m.paletteKey(msg)
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
		return nil
	}

	action, ok := m.bind.Key[msg.String()]
	if !ok {
		return nil
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
		m.scroll(pageStep(body))
	case keys.PageUp:
		m.scroll(-pageStep(body))
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
		m.openPalette()
	case keys.Command:
		m.cmdMode = true
		m.cmdInput = ""
	case keys.Reload:
		return m.reloadConfig()
	}
	m.clampTop()
	return nil
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
		m.openPalette()

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
	return nil
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

// pageStep is a page minus two lines, so the first lines of the next page are
// the last lines of this one. A pager that steps a whole page makes the reader
// find their place again every time.
func pageStep(body int) int {
	if body > 3 {
		return body - 2
	}
	return body
}

func (m *Model) bodyHeight() int {
	// The chrome line overlays the last row instead of shrinking the viewport,
	// so opening a search prompt never moves the diff.
	h := m.height - 1 // pinned file header
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) maxTop() int {
	t := m.total - m.bodyHeight()
	// The viewport may not start above the first content line, so a diff that
	// fits on screen has minTop == maxTop rather than a negative range.
	if lo := m.minTop(); t < lo {
		t = lo
	}
	return t
}

func (m *Model) clampTop() {
	if m.top < m.minTop() {
		m.top = m.minTop()
	}
	if m.top > m.maxTop() {
		m.top = m.maxTop()
	}
	// Jumps land wherever they like; move off a header if one ended up on top.
	m.settle()
}

// settle moves a top that landed on a file header onto the nearest content
// line. It is only for explicit jumps, never for scrolling, which uses step and
// therefore knows which way it is going.
func (m *Model) settle() {
	if len(m.rows) == 0 || m.top >= m.total {
		return
	}
	for m.rows[m.rowAt(m.top)].Kind == layout.FileRow {
		switch {
		case m.top+1 <= m.maxTop():
			m.top++
		case m.top-1 >= m.minTop():
			m.top--
		default:
			return
		}
	}
}

// minTop is the first line of content: leading file headers belong to the
// pinned header, not to the scrolling body.
func (m *Model) minTop() int {
	for i, row := range m.rows {
		if row.Kind != layout.FileRow {
			return m.starts[i]
		}
	}
	return 0
}

func (m *Model) scroll(n int) {
	if n == 0 {
		return
	}
	dir := 1
	if n < 0 {
		dir = -1
		n = -n
	}
	for i := 0; i < n; i++ {
		if !m.step(dir) {
			return
		}
	}
}

// step moves the viewport one line in dir, skipping file headers: the viewport
// may not start on one, because the pinned header already shows it. Direction
// matters. A step that always moved forward would make scrolling upwards onto a
// header cancel itself, which is how a view gets stuck.
func (m *Model) step(dir int) bool {
	next := m.top + dir
	for next >= 0 && next < m.total && m.rows[m.rowAt(next)].Kind == layout.FileRow {
		next += dir
	}
	if next < m.minTop() || next > m.maxTop() {
		return false
	}
	m.top = next
	return true
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
	start := cur + dir
	if kind == layout.FileRow && dir < 0 {
		// Walking back from a file's first hunk finds that file's own header
		// first. Step over it so "previous file" means the previous one.
		for start >= 0 && m.rows[start].Kind != layout.FileRow {
			start--
		}
		start--
	}
	for i := start; i >= 0 && i < len(m.rows); i += dir {
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

// View implements tea.Model.
//
// The file header is chrome, not content: it is pinned to the top line and
// changes when the view moves into another file, so scrolling never loses the
// answer to "which file am I in". The bottom line exists only while there is
// something to say — a search, a command, an error.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.palette {
		return m.paletteView()
	}
	body := m.bodyHeight()
	chrome := m.chromeVisible()
	visible := body
	if chrome {
		visible--
	}

	var b strings.Builder
	b.WriteString(m.headerLine())
	b.WriteByte('\n')
	for i := 0; i < visible; i++ {
		line := m.top + i
		if line < m.total {
			b.WriteString(m.visualLine(line))
		}
		b.WriteString("\x1b[0m")
		if i < visible-1 || chrome {
			b.WriteByte('\n')
		}
	}
	if chrome {
		b.WriteString(m.chromeLine())
	}
	return b.String()
}

// headerLine renders the pinned header for the file under the viewport.
func (m *Model) headerLine() string {
	if len(m.rows) == 0 {
		return ""
	}
	row := m.rows[m.rowAt(m.top)]
	if row.File == nil {
		return ""
	}
	return render.ANSI(m.r.Segments(layout.Row{Kind: layout.FileRow, File: row.File}), m.r.Depth)
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

// chromeVisible is true only while there is something transient to show.
func (m *Model) chromeVisible() bool {
	return m.typing || m.cmdMode || m.query != "" || m.message != ""
}

// chromeLine is the transient bottom line: a prompt, a search, or a message.
func (m *Model) chromeLine() string {
	pal := m.r.Pal
	segs := []render.Seg{{Text: " ", Style: render.Style{Fg: pal.Dim}}}

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
			Text:  fmt.Sprintf("/%s %d/%d", m.query, pos, len(m.matches)),
			Style: render.Style{Fg: pal.Accent},
		})
	case m.message != "":
		segs = append(segs, render.Seg{Text: m.message, Style: render.Style{Fg: pal.Accent}})
	}
	return render.ANSI(render.Fit(segs, m.r.Width, render.Style{}), m.r.Depth)
}
