# Working on drift

## Commands

- `./x check` (the default) checks Go formatting, vet, build and race tests without rewriting
  sources.
- `./x --help` lists targets; `./x test` and `./x build` forward Go arguments.
- `go build -o drift ./cmd/drift` builds the binary (Go 1.26 or newer).
- `go test ./...` runs the offline suites. The git tests create throwaway repositories in
  `t.TempDir()`; nothing touches a real repository.
- `gofmt -l .` must print nothing.

## Code map

- `cmd/drift/main.go`: flags, exit codes, and the routing between the four output surfaces.
- `internal/git/`: repository discovery, base resolution, loading the change set, reading file
  content at a revision.
- `internal/patch/`: the unified-diff parser and the change model. Deliberately lenient; the
  parser tests enumerate every awkward shape git can emit.
- `internal/layout/`: rows, split pairing, word-level emphasis. Width-independent by design.
- `internal/highlight/`: Chroma lexing per file, trimmed to the lines a diff shows, cached.
- `internal/render/`: theme, ANSI encoding, width maths, plain text, list output, JSON.
- `internal/tui/`: the bubbletea model and the fsnotify watcher behind `--watch`.
- `test/`: no directory yet; parser fixtures live beside the parser they exercise.

## Conventions

Use the fewest moving parts. drift shells out to git and parses its output; do not grow a second
implementation of history, renames or whitespace rules.

Keep the four surfaces honest. Split view, pager, plain text and JSON must all read the same
`layout` rows, so a thing the JSON cannot express is a thing the TUI should not draw. When you
add output, add it to the row model rather than to one renderer.

Rows stay independent of terminal width. Wrapping and truncation happen at draw time; a row that
knows about columns breaks the pager/TUI agreement.

Comment modules and functions with their purpose and contract; avoid line-by-line comments.
Update the comments and docs a change touches.

Write one focused test per behaviour worth protecting, using `testing`. Tests must be hermetic and
offline: no network, no real repositories, no dependence on the developer's terminal. Add a
CHANGELOG entry for user-visible changes.

Preserve unrelated changes and never expose secrets.
