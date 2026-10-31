# drift

A terminal diff viewer. Run it inside a worktree and see everything that changed from the base
branch, as one continuous, syntax-highlighted review surface.

```
$ cd ~/room/code/worktrees/opencode-site
$ drift
```

Split view compares old and new side by side. Press `s` to fold it into one column, `s` again to
hide the deletions and read only the result.

```
 internal/render/render.go                                                          +7 -7
 @@ -14,16 +14,16 @@ type Renderer struct {
14   // Draw writes every row to the buffer.       │ 14   // Draw writes every row to the buffer.
15   func (r *Renderer) Draw(rows []string) string │ 15   func (r *Renderer) Draw(rows []string) string
17 -     for _, row := range rows {                │ 17 +     for i, row := range rows {
                                                   │ 18 +         if i > 0 {
18           b.WriteString(row)                    │ 21           b.WriteString(row)
 internal/render/render.go +7 -7                                   split · 42% · ? help
```

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/fschrhunt/drift/main/install.sh | sh
```

or, from a checkout:

```sh
go install ./cmd/drift
ln -sf drift "$(go env GOPATH)/bin/git-drift"
```

The `git-drift` link is what makes `git drift` work; git looks up `git-<name>` on `PATH`.

## Try it on awkward diffs

`scripts/demo.sh` builds a throwaway repository with the cases that break diff viewers: a rename, a
binary, a mode-only change, a symlink, a deleted file, CRLF, non-ASCII and space-containing paths, a
4000-line file and an untracked binary.

```sh
scripts/demo.sh
cd /tmp/drift-demo && drift
```

`drift config` in any repository prints the revision being compared, which is the quickest way to
check you are looking at what you think you are:

```
config  ~/.config/drift/config.toml
base    origin/main → 061321a
```

## Usage

```
drift                      review the working tree
drift --base origin/main   compare against something else
drift --file render.go     one file only
drift --layout split       start in a specific layout
drift --list               one line per changed file
drift --json               the change set as JSON
git diff | drift           render a patch
```

Bare `drift` compares the merge base of the base branch and `HEAD` against the working tree,
so it includes staged, unstaged and untracked files, and leaves out commits that landed on the
base branch after you branched. Everything under the current directory is in scope; run it from
`src/` and you only see `src/`.

The base is found from `--base`, `$DRIFT_BASE`, `git config drift.base`, `origin/HEAD`,
`origin/main`, `origin/master`, `main`, then `master`.

## Configuration

Nothing has to be configured. The file exists for the handful of things worth remembering.

```sh
drift init     # write a commented config
drift config   # show what is actually in effect
```

The file lives at `~/.config/drift/config.toml` (`$XDG_CONFIG_HOME/drift/config.toml`, or
`$DRIFT_CONFIG_DIR`; override the file itself with `--config`). Every key is optional — delete a
line and you get the built-in behaviour back.

```toml
theme = "auto"        # auto follows the terminal; or a theme name
layout = "auto"       # auto, split, unified, new
context = 3
numbers = true
untracked = true
watch = false
# base = "origin/main"

[keys]
quit = ["q", "ctrl+c"]
next_file = ["tab", "]"]

[colors]
add_bg = "#10241a"
```

Flags override the file, and only when you actually pass them.

### Per repository

One setting genuinely belongs to the project rather than to you: which branch it
branches from. Put a `.drift.toml` at the repository root and everyone who
clones it compares against the right place.

```toml
base = "develop"
```

`drift init --repo` writes one. That is the entire schema, on purpose: a
repository may say **what** to compare against, never **how** the diff is
painted. Themes, colours, keys, layout and context are rejected there with an
error, because a review tool whose presenter can be set by the thing being
reviewed is a review tool that can be made to lie.

Your own `base` still wins over the repository's, and `--base` wins over both.

## Themes

Two palettes ship and drift picks between them by asking your terminal for its background — there
is nothing to choose. When you want a different look, you layer on top of that.

Adjust a few colours in your config:

```toml
[colors]
add_bg = "#10241a"
del_bg = "#2a1417"

[colors.syntax]
keyword = "#ff9d6f"
```

Or write a theme file at `~/.config/drift/themes/<name>.toml` and set `theme = "<name>"`:

```toml
extends = "dark"       # dark, light, auto, or another theme

[colors]
add_fg = "#b8f397"

[colors.syntax]
string = "#a5d6ff"
```

`extends` inherits everything and overrides the rest, so a theme can be one line. `extends = "auto"`
follows the terminal, which is the default when you leave it out.

```sh
drift theme              # list installed themes
drift theme dark         # print the resolved colours as a theme file to copy
```

Inside the viewer, `:theme <name>` switches immediately and `R` re-reads the config from disk, so
you can keep the theme file open in another window while you tune it.

## Keys

Every action is rebindable. These are the defaults:

```
j k ↓ ↑         line                space b          page
g G             top / bottom        tab shift+tab    next / prev file
} {             next / prev hunk    s                split → unified → new
/               search              n N              next / prev match
t               line numbers        :                command
R               reload config       ?                help
q               quit
```

Rebind them in `[keys]`. Replacing an action's list replaces all of that action's keys, and a key
can only do one thing — a conflict is an error rather than a surprise.

```toml
[keys]
quit = ["x"]
next_file = ["J"]
down = ["e", "down"]
```

Keys are written the way your terminal reports them: `j`, `G`, `ctrl+d`, `tab`, `shift+tab`,
`pgdown`, `space`, `enter`, `esc`. Multi-key sequences like `gg` are not supported.

The command line takes a small set of verbs:

```
:theme <name>    :layout split|unified|new    :numbers    :reload    :q
```

## For agents

`drift --list` then `drift --file PATH` is the same shape as `git diff --stat` then
`git diff -- PATH`, so there is nothing new to learn:

```sh
$ drift --list
M	internal/render/render.go	+7	-7
A	notes.txt	+1	-0

$ drift --file notes.txt
```

`--json` emits the full structure with a versioned schema. When output is not a terminal nothing
is coloured, nothing paginates and nothing waits for input, so piping is always safe. `--context 0`
gives the most token-frugal patch, and `--exit-code` returns 1 when anything changed.

Config and themes never change the machine-readable output: `--json` returns the patch as git
produced it.

## How it works

drift renders patches; it does not implement git. It asks git for the diff, parses the unified
patch, and lays the hunks out as rows. Every surface — split view, pager, plain text, JSON — reads
those same rows, so what you see interactively is what an agent gets.

Syntax highlighting tokenises the whole file on disk and indexes the tokens by line, rather than
tokenising the hunk. A block comment that starts above the change still colours correctly, and
there is no incremental lexer to keep in sync.

## Development

```sh
./x            # format check, vet, build, race tests
./x test -run Parser ./internal/patch
```
