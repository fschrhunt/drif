// Command drif shows what changed in a worktree as a terminal review surface.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	tea "github.com/charmbracelet/bubbletea"
	xterm "github.com/charmbracelet/x/term"
	isatty "github.com/mattn/go-isatty"

	"github.com/fschrhunt/drif/internal/config"
	"github.com/fschrhunt/drif/internal/git"
	"github.com/fschrhunt/drif/internal/keys"
	"github.com/fschrhunt/drif/internal/layout"
	"github.com/fschrhunt/drif/internal/patch"
	"github.com/fschrhunt/drif/internal/paths"
	"github.com/fschrhunt/drif/internal/render"
	"github.com/fschrhunt/drif/internal/theme"
	"github.com/fschrhunt/drif/internal/tui"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	// Subcommands come before the flag set so `drif config` is a command, not
	// a stray argument.
	if len(args) > 0 {
		switch args[0] {
		case "config", "theme", "init":
			return command(args[0], args[1:], stdout, stderr)
		}
	}

	fs := flag.NewFlagSet("drif", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "config file (default: $XDG_CONFIG_HOME/drif/config.toml)")
	base := fs.String("base", "", "revision to compare against")
	context := fs.Int("context", 3, "lines of context in each hunk")
	noUntracked := fs.Bool("no-untracked", false, "ignore files git does not track")
	list := fs.Bool("list", false, "list changed files instead of showing the diff")
	file := fs.String("file", "", "show only this file")
	asJSON := fs.Bool("json", false, "emit the change set as JSON")
	plain := fs.Bool("plain", false, "never start the interactive view")
	forceTUI := fs.Bool("tui", false, "start the interactive view even when a patch is piped in")
	themeName := fs.String("theme", "", "theme name, or a file in the themes directory")
	layoutName := fs.String("layout", "", "initial layout: auto, split, unified or new")
	colorMode := fs.String("color", "auto", "colour: auto, always or never")
	noNumbers := fs.Bool("no-numbers", false, "hide line numbers")
	exitCode := fs.Bool("exit-code", false, "exit 1 when there are changes")
	showVersion := fs.Bool("version", false, "print the version")
	fs.Usage = func() { usage(stderr) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "drif "+version)
		return 0
	}

	// Flags win over every file, but only when they were actually given; a
	// flag left at its default must not overwrite a configured value.
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// The repository may be absent: a patch can arrive on stdin outside one.
	repo, _ := git.Discover()

	// resolve is the single precedence path — user config, then the
	// repository's base, then flags — used at startup and again on reload.
	resolve := func() (*config.Config, error) {
		c, _, err := config.Load(*configPath)
		if err != nil {
			return nil, err
		}
		if repo != nil {
			rc, _, err := config.LoadRepo(repo.Root)
			if err != nil {
				return nil, err
			}
			c.MergeRepo(rc)
		}
		if set["base"] {
			c.Base = *base
		}
		if set["context"] {
			c.Context = *context
		}
		if set["no-untracked"] {
			c.Untracked = !*noUntracked
		}
		if set["theme"] {
			c.Theme = *themeName
		}
		if set["layout"] {
			c.Layout = *layoutName
		}
		if set["no-numbers"] {
			c.Numbers = !*noNumbers
		}
		return c, c.Validate()
	}

	active, err := resolve()
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}

	stdinTTY := isatty.IsTerminal(stdin.Fd()) || isatty.IsCygwinTerminal(stdin.Fd())
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	cs, err := load(repo, active.Base, active.Context, active.Untracked, stdin, stdinTTY)
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	if *file != "" {
		cs.Files = pick(cs.Files, *file)
	}

	switch {
	case *asJSON:
		out, err := render.JSON(cs)
		if err != nil {
			fmt.Fprintf(stderr, "drif: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, string(out))

	case *list:
		for _, line := range render.ListLines(cs, cs.Scope) {
			fmt.Fprintln(stdout, line)
		}

	default:
		if len(cs.Files) == 0 {
			fmt.Fprintf(stdout, "no changes vs %s\n", orDefault(cs.Base, "the base revision"))
			return 0
		}
		interactive := stdoutTTY && !*plain && (stdinTTY || *forceTUI)
		if interactive {
			if code := runTUI(cs, active, resolve, repo, stdinTTY, *file, stderr); code != 0 {
				return code
			}
		} else {
			if code := writeStatic(cs, active, *colorMode, stdoutTTY, stdout, stderr); code != 0 {
				return code
			}
		}
	}

	if *exitCode && len(cs.Files) > 0 {
		return 1
	}
	return 0
}

// load builds the change set either from a piped patch or from the repository
// containing the working directory. An empty stdin falls back to the
// repository, because a non-tty stdin is also what CI and cron give you.
func load(repo *git.Repo, base string, context int, untracked bool, stdin *os.File, stdinTTY bool) (*patch.ChangeSet, error) {
	if !stdinTTY {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(data)) > 0 {
			files, err := patch.Parse(bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			return &patch.ChangeSet{Files: files}, nil
		}
	}
	if repo == nil {
		r, err := git.Discover()
		if err != nil {
			return nil, err
		}
		repo = r
	}
	cs, err := repo.Load(git.Options{Base: base, Context: context, Untracked: untracked})
	if err != nil {
		return nil, err
	}
	return cs, nil
}

// runTUI starts the interactive view. The config is re-read on demand so a
// theme can be edited in one window and seen in another.
func runTUI(cs *patch.ChangeSet, cfg *config.Config, resolve func() (*config.Config, error), repo *git.Repo, stdinTTY bool, file string, stderr io.Writer) int {
	active := cfg
	var activeMu sync.RWMutex

	keymap, err := active.KeyMap()
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	palette, err := theme.Resolve(active.Theme, active.Colors)
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}

	refresh := func() (*tui.Refresh, error) {
		next, err := resolve()
		if err != nil {
			return nil, err
		}
		pal, err := theme.Resolve(next.Theme, next.Colors)
		if err != nil {
			return nil, err
		}
		km, err := next.KeyMap()
		if err != nil {
			return nil, err
		}
		activeMu.Lock()
		active = next
		activeMu.Unlock()
		mode, auto := layoutOf(next.Layout)
		return &tui.Refresh{Palette: pal, Bind: km, Mode: mode, Auto: auto, Numbers: next.Numbers}, nil
	}

	themeFor := func(name string) (*render.Palette, error) {
		activeMu.RLock()
		colors := active.Colors
		activeMu.RUnlock()
		return theme.Resolve(name, colors)
	}

	mode, auto := layoutOf(active.Layout)
	if cs.MergeBase == "" {
		repo = nil // A piped patch is a snapshot, even inside a repository.
	}

	opts := tui.Options{
		Mode:    mode,
		Auto:    auto,
		Numbers: active.Numbers,
		Palette: palette,
		Bind:    keymap,
		Repo:    repo,
		Reload: func() (*patch.ChangeSet, error) {
			activeMu.RLock()
			options := git.Options{Base: active.Base, Context: active.Context, Untracked: active.Untracked}
			activeMu.RUnlock()
			next, err := repo.Load(options)
			if err != nil {
				return nil, err
			}
			if file != "" {
				next.Files = pick(next.Files, file)
			}
			return next, nil
		},
		Refresh: refresh,
		Theme:   themeFor,
	}
	// Alternate scroll mode: in the alternate screen the terminal turns wheel
	// events into arrow keys, which drif already scrolls with. That gives a
	// wheel without capturing the mouse, so drag-to-select keeps working.
	fmt.Fprint(os.Stdout, "\x1b[?1007h")
	defer fmt.Fprint(os.Stdout, "\x1b[?1007l")

	model := tui.New(cs, opts)
	defer model.Close()
	if _, err := tea.NewProgram(model, programOptions(stdinTTY)...).Run(); err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	return 0
}

// syncWriter brackets each write in DECSET 2026, the terminal's synchronized
// output mode. Bubbletea writes a whole frame in one Write, so the terminal
// either shows the previous frame or the next one, never half of each. That is
// the difference between smooth scrolling and lines that visibly slide into
// place on a high-latency link. Terminals without support ignore the sequence.
// syncFile keeps the file descriptor: bubbletea only treats its output as a
// terminal when it can call Fd on it.
type syncFile struct{ *os.File }

func (s syncFile) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if _, err := io.WriteString(s.File, "\x1b[?2026h"); err != nil {
		return 0, err
	}
	n, err := s.File.Write(p)
	if _, werr := io.WriteString(s.File, "\x1b[?2026l"); err == nil {
		err = werr
	}
	return n, err
}

func programOptions(stdinTTY bool) []tea.ProgramOption {
	opts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithOutput(syncFile{os.Stdout})}
	if !stdinTTY {
		// A patch arrived on stdin, so read keys from the terminal directly.
		if f, err := os.Open("/dev/tty"); err == nil {
			opts = append(opts, tea.WithInput(f))
		}
	}
	return opts
}

func layoutOf(name string) (layout.Mode, bool) {
	switch name {
	case "split":
		return layout.Split, false
	case "unified":
		return layout.Unified, false
	case "new":
		return layout.New, false
	}
	return layout.Split, true
}

func writeStatic(cs *patch.ChangeSet, cfg *config.Config, colorMode string, stdoutTTY bool, stdout, stderr io.Writer) int {
	useColor := false
	switch colorMode {
	case "always":
		useColor = true
	case "never":
		useColor = false
	default:
		useColor = stdoutTTY
	}
	if !useColor {
		for _, line := range render.PlainLines(cs, cs.Scope) {
			fmt.Fprintln(stdout, line)
		}
		return 0
	}
	palette, err := theme.Resolve(cfg.Theme, cfg.Colors)
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	r := &render.Renderer{
		Pal:     palette,
		Width:   termWidth(os.Stdout),
		Mode:    layout.Unified,
		Depth:   render.DetectDepth(),
		Prefix:  cs.Scope,
		Numbers: cfg.Numbers,
	}
	for _, line := range r.Lines(layout.Build(cs, layout.Unified)) {
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// command runs the subcommands that configure drif itself.
func command(name string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("drif "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "config file")
	repoFile := fs.Bool("repo", false, "write .drif.toml in the repository instead")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	switch name {
	case "init":
		return initConfig(*configPath, *repoFile, stdout, stderr)
	case "config":
		return showConfig(*configPath, stdout, stderr)
	case "theme":
		return themeCommand(fs.Args(), stdout, stderr)
	}
	return 2
}

func initConfig(configPath string, repoFile bool, stdout, stderr io.Writer) int {
	if repoFile {
		repo, err := git.Discover()
		if err != nil {
			fmt.Fprintf(stderr, "drif: %v\n", err)
			return 2
		}
		path := filepath.Join(repo.Root, config.RepoFileName)
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(stderr, "drif: %s already exists; edit it instead\n", path)
			return 1
		}
		if err := os.WriteFile(path, []byte(config.RepoTemplate()), 0o644); err != nil {
			fmt.Fprintf(stderr, "drif: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "wrote %s\n", path)
		return 0
	}

	path, err := paths.ConfigFile(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(stderr, "drif: %s already exists; edit it instead\n", path)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	if err := os.WriteFile(path, []byte(config.Template()), 0o644); err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	themes, err := paths.ThemesDir()
	if err == nil {
		_ = os.MkdirAll(themes, 0o755)
	}
	fmt.Fprintf(stdout, "wrote %s\nthemes go in %s\n", path, themes)
	return 0
}

func showConfig(configPath string, stdout, stderr io.Writer) int {
	cfg, path, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	palette, err := theme.Resolve(cfg.Theme, cfg.Colors)
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	_, statErr := os.Stat(path)
	source := path
	if statErr != nil {
		source += " (missing, using defaults)"
	}
	fmt.Fprintf(stdout, "config  %s\n", source)
	// Show the resolution, not just the setting: "which revision am I looking
	// at" is the question drif exists to answer.
	if repo, err := git.Discover(); err == nil {
		if base, err := repo.ResolveBase(cfg.Base); err == nil {
			line := "base    " + base
			if mb, err := repo.MergeBase(base); err == nil {
				if len(mb) > 7 {
					mb = mb[:7]
				}
				line += " → " + mb
			}
			fmt.Fprintln(stdout, line)
		}
	}
	fmt.Fprintf(stdout, "theme   %s → %s background\n", cfg.Theme, backgroundName(palette))
	fmt.Fprintf(stdout, "layout  %s\n", cfg.Layout)
	fmt.Fprintf(stdout, "context %d\n", cfg.Context)
	fmt.Fprintf(stdout, "numbers %v\n", cfg.Numbers)
	fmt.Fprintf(stdout, "untracked %v\n", cfg.Untracked)
	if cfg.Base != "" {
		fmt.Fprintf(stdout, "base    %s\n", cfg.Base)
	}
	if repo, err := git.Discover(); err == nil {
		rc, repoPath, err := config.LoadRepo(repo.Root)
		if err != nil {
			fmt.Fprintf(stderr, "drif: %v\n", err)
			return 2
		}
		switch {
		case repoPath == "":
			fmt.Fprintln(stdout, "repo    none")
		case rc.Base != "":
			fmt.Fprintf(stdout, "repo    %s (base = %s)\n", repoPath, rc.Base)
		default:
			fmt.Fprintf(stdout, "repo    %s\n", repoPath)
		}
	}

	names, _ := theme.List()
	fmt.Fprintf(stdout, "themes  %s\n", strings.Join(names, ", "))

	km, err := cfg.KeyMap()
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, "\nkeys")
	for _, action := range keys.All {
		if len(km.Action[action]) == 0 {
			continue
		}
		shown := make([]string, 0, len(km.Action[action]))
		for _, k := range km.Action[action] {
			shown = append(shown, keys.Display(k))
		}
		fmt.Fprintf(stdout, "  %-14s %s\n", action, strings.Join(shown, " "))
	}
	return 0
}

func backgroundName(p *render.Palette) string {
	if p == nil {
		return "unknown"
	}
	// The light palette's text colour is dark; that is the cheapest reliable
	// way to describe which one is active.
	if p.Text.R < 0x80 && p.Text.G < 0x80 && p.Text.B < 0x80 {
		return "light"
	}
	return "dark"
}

func themeCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		names, err := theme.List()
		if err != nil {
			fmt.Fprintf(stderr, "drif: %v\n", err)
			return 2
		}
		for _, n := range names {
			fmt.Fprintln(stdout, n)
		}
		return 0
	}
	name := args[0]
	palette, err := theme.Resolve(name, theme.ColorSet{})
	if err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "# drif theme: %s\n", name)
	fmt.Fprintf(stdout, "# Copy this to %s/themes/%s.toml and edit.\n\n", configDir(), name)
	enc := toml.NewEncoder(stdout)
	if err := enc.Encode(struct {
		Colors theme.ColorSet `toml:"colors"`
	}{theme.ToColorSet(palette)}); err != nil {
		fmt.Fprintf(stderr, "drif: %v\n", err)
		return 2
	}
	return 0
}

func configDir() string {
	dir, err := paths.Dir()
	if err != nil {
		return "~/.config/drif"
	}
	return dir
}

func termWidth(f *os.File) int {
	if w, _, err := xterm.GetSize(f.Fd()); err == nil && w > 0 {
		return w
	}
	return 0
}

// pick keeps the files matching a name given on the command line, accepting
// either the path relative to the repository or its last components.
func pick(files []*patch.File, name string) []*patch.File {
	name = strings.TrimPrefix(name, "./")
	var out []*patch.File
	for _, f := range files {
		for _, p := range []string{f.Path(), f.OldPath} {
			if p == "" {
				continue
			}
			if p == name || strings.HasSuffix(p, "/"+name) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func usage(w io.Writer) {
	fmt.Fprint(w, `drif — see what changed in a worktree

usage:
  drif [flags]            interactive review of the working tree
  git diff | drif         render a patch
  drif --list             one line per changed file
  drif --json             machine-readable change set
  drif config             show the effective configuration
  drif theme [name]       list themes, or print one as a theme file
  drif init [--repo]      write a starter config (--repo: .drif.toml)

flags:
  --config PATH       configuration file
  --base REF          revision to compare against (default: origin/HEAD, main, master)
  --context N         lines of context per hunk (default 3)
  --file PATH         show only this file
  --layout MODE       initial layout: auto, split, unified, new
  --theme NAME        theme name, or a file in the themes directory
  --list              list changed files
  --json              emit JSON
  --plain             never start the interactive view
  --color WHEN        auto, always or never
  --no-numbers        hide line numbers
  --no-untracked      ignore files git does not track
  --exit-code         exit 1 when there are changes
  --version           print the version

keys (interactive, rebindable in config):
  j k                 scroll          space b        page
  tab shift+tab       next / prev file
  } {                 next / prev hunk
  s                   split → unified → new
  /                   search          n N            next / prev match
  :                   command         R              reload config
  ?                   help            q              quit
`)
}
