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

## Usage

```
drift                      review the working tree
drift --base origin/main   compare against something else
drift --file render.go     one file only
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

Nothing needs configuring. If the base is wrong, `drift --base <ref>` tells it once.

## Keys

```
j k ↓ ↑     line          space b     page           g G     top / bottom
n p         next / prev file            } {          next / prev hunk
s           split → unified → new
/           search        n N         next / prev match
t           line numbers  ?           help           q       quit
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
