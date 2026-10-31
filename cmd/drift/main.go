// Command drift shows what changed in a worktree as a terminal review surface.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	xterm "github.com/charmbracelet/x/term"
	isatty "github.com/mattn/go-isatty"

	"github.com/fschrhunt/drift/internal/git"
	"github.com/fschrhunt/drift/internal/highlight"
	"github.com/fschrhunt/drift/internal/layout"
	"github.com/fschrhunt/drift/internal/patch"
	"github.com/fschrhunt/drift/internal/render"
	"github.com/fschrhunt/drift/internal/tui"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	fs.SetOutput(stderr)

	base := fs.String("base", "", "revision to compare against")
	context := fs.Int("context", 3, "lines of context in each hunk")
	noUntracked := fs.Bool("no-untracked", false, "ignore files git does not track")
	list := fs.Bool("list", false, "list changed files instead of showing the diff")
	file := fs.String("file", "", "show only this file")
	asJSON := fs.Bool("json", false, "emit the change set as JSON")
	plain := fs.Bool("plain", false, "never start the interactive view")
	forceTUI := fs.Bool("tui", false, "start the interactive view even when a patch is piped in")
	color := fs.String("color", "auto", "colour: auto, always or never")
	noNumbers := fs.Bool("no-numbers", false, "hide line numbers")
	mode := fs.String("mode", "", "initial layout: split, unified or new")
	exitCode := fs.Bool("exit-code", false, "exit 1 when there are changes")
	watch := fs.Bool("watch", false, "reload when files change")
	showVersion := fs.Bool("version", false, "print the version")
	fs.Usage = func() { usage(stderr) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "drift "+version)
		return 0
	}

	stdinTTY := isatty.IsTerminal(stdin.Fd()) || isatty.IsCygwinTerminal(stdin.Fd())
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	cs, repo, err := load(*base, *context, !*noUntracked, stdin, stdinTTY)
	if err != nil {
		fmt.Fprintf(stderr, "drift: %v\n", err)
		return 2
	}
	if *file != "" {
		cs.Files = pick(cs.Files, *file)
	}

	switch {
	case *asJSON:
		out, err := render.JSON(cs)
		if err != nil {
			fmt.Fprintf(stderr, "drift: %v\n", err)
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
			var cache *highlight.Cache
			if repo != nil {
				cache = highlight.New(repo, cs.MergeBase)
			}
			m := layout.Unified
			auto := *mode == ""
			switch *mode {
			case "split":
				m = layout.Split
			case "new":
				m = layout.New
			case "unified":
				m = layout.Unified
			case "":
				m = layout.Split // narrowed automatically on small terminals
			default:
				fmt.Fprintf(stderr, "drift: unknown mode %q\n", *mode)
				return 2
			}
			var reload func() (*patch.ChangeSet, error)
			if repo != nil {
				reload = func() (*patch.ChangeSet, error) {
					next, err := repo.Load(git.Options{Base: *base, Context: *context, Untracked: !*noUntracked})
					if err != nil {
						return nil, err
					}
					if *file != "" {
						next.Files = pick(next.Files, *file)
					}
					return next, nil
				}
			}
			opts := tui.Options{
				Mode: m, Auto: auto, Numbers: !*noNumbers,
				Repo: repo, Cache: cache, Watch: *watch, Reload: reload,
			}
			if err := runTUI(cs, opts, stdinTTY); err != nil {
				fmt.Fprintf(stderr, "drift: %v\n", err)
				return 2
			}
		} else {
			writeStatic(cs, !*noNumbers, *color, stdoutTTY, stdout)
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
func load(base string, context int, untracked bool, stdin *os.File, stdinTTY bool) (*patch.ChangeSet, *git.Repo, error) {
	if !stdinTTY {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, nil, err
		}
		if len(bytes.TrimSpace(data)) > 0 {
			files, err := patch.Parse(bytes.NewReader(data))
			if err != nil {
				return nil, nil, err
			}
			return &patch.ChangeSet{Files: files}, nil, nil
		}
	}
	repo, err := git.Discover()
	if err != nil {
		return nil, nil, err
	}
	cs, err := repo.Load(git.Options{Base: base, Context: context, Untracked: untracked})
	if err != nil {
		return nil, nil, err
	}
	return cs, repo, nil
}

func runTUI(cs *patch.ChangeSet, opts tui.Options, stdinTTY bool) error {
	model := tui.New(cs, opts)
	progOpts := []tea.ProgramOption{tea.WithAltScreen()}
	if !stdinTTY {
		// A patch arrived on stdin, so read keys from the terminal directly.
		if f, err := os.Open("/dev/tty"); err == nil {
			defer f.Close()
			progOpts = append(progOpts, tea.WithInput(f))
		}
	}
	_, err := tea.NewProgram(model, progOpts...).Run()
	return err
}

func writeStatic(cs *patch.ChangeSet, numbers bool, colorMode string, stdoutTTY bool, stdout io.Writer) {
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
		return
	}
	rows := layout.Build(cs, layout.Unified)
	r := &render.Renderer{
		Pal:     render.DetectPalette(),
		Width:   termWidth(os.Stdout),
		Mode:    layout.Unified,
		Depth:   render.DetectDepth(),
		Prefix:  cs.Scope,
		Numbers: numbers,
	}
	for _, line := range r.Lines(rows) {
		fmt.Fprintln(stdout, line)
	}
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
	fmt.Fprint(w, `drift — see what changed in a worktree

usage:
  drift [flags]            interactive review of the working tree
  git diff | drift         render a patch
  drift --list             one line per changed file
  drift --json             machine-readable change set

flags:
  --base REF          revision to compare against (default: origin/HEAD, main, master)
  --context N         lines of context per hunk (default 3)
  --file PATH         show only this file
  --mode MODE         initial layout: split, unified, new
  --list              list changed files
  --json              emit JSON
  --plain             never start the interactive view
  --color WHEN        auto, always or never
  --no-numbers        hide line numbers
  --no-untracked      ignore files git does not track
  --watch             reload when files change
  --exit-code         exit 1 when there are changes
  --version           print the version

keys (interactive):
  j k                 scroll          space b        page
  n p                 next/prev file  } {            next/prev hunk
  s                   split → unified → new
  /                   search          t              line numbers
  ?                   help            q              quit
`)
}
