# Working on drif

## Commands

- `./x check` (the default) checks Go formatting, vet, build and race tests without rewriting
  sources.
- `./x --help` lists targets; `./x test` and `./x build` forward Go arguments.
- `go build -o drif ./cmd/drif` builds the binary (Go 1.26 or newer).
- `go test ./...` runs the offline suites. The git tests create throwaway repositories in
  `t.TempDir()`; nothing touches a real repository. `internal/git/real_test.go` runs the real git
  binary and parses its real output — keep it that way, because the bugs live in that seam.
- `gofmt -l .` must print nothing.

## Code map

- `cmd/drif/main.go`: flags, exit codes, and the routing between the four output surfaces.
- `internal/git/`: repository discovery, base resolution, loading the change set, reading file
  content at a revision.
- `internal/paths/`: where the config directory, config file and themes live.
- `internal/config/`: the TOML file, its defaults, and validation.
- `internal/keys/`: the action vocabulary and the keymap, in both directions.
- `internal/theme/`: colour sets, `extends` layering, and palette resolution.
- `internal/patch/`: the unified-diff parser and the change model. Deliberately lenient; the
  parser tests enumerate every awkward shape git can emit.
- `internal/layout/`: rows, split pairing, word-level emphasis. Width-independent by design.
- `internal/highlight/`: Chroma lexing per file, trimmed to the lines a diff shows, cached.
- `internal/render/`: palette detection, ANSI encoding, width maths, plain text, list output, JSON.
- `internal/tui/`: the bubbletea model and the fsnotify watcher behind `--watch`.
- `scripts/demo.sh`: builds a repository of awkward changes (rename, binary, mode-only, symlink,
  CRLF, non-ASCII paths, unborn HEAD) for looking at by hand. The same cases are pinned in
  `internal/git/real_test.go`.
- `test/`: no directory yet; parser fixtures live beside the parser they exercise.

## Conventions

Use the fewest moving parts. drif shells out to git and parses its output; do not grow a second
implementation of history, renames or whitespace rules.

Repository-level config (`.drif.toml`) may only declare what to compare against. Before adding a
setting there, ask whether it changes what is compared or how it looks; the second kind belongs in
the user config, because a repository must not be able to change how it is reviewed.

Adding an action means three edits that must stay in step: the constant and `All` in
`internal/keys`, a case in the TUI's key switch, and a row in `helpRows`. A binding that does
nothing is the easiest bug to ship here.

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
