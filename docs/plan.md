# drift — design

A terminal diff viewer for a worktree. Run `drift` anywhere inside a repository and see everything
that changed from the base branch, as one continuous review surface. The same binary serves people
and agents.

This document records the decisions that shape the code. Where it disagrees with a comment, the
comment is probably stale; fix it.

## 1. What the default does

- Compares `merge-base(base, HEAD)` against the working tree. Not "vs HEAD" (which would hide your
  commits) and not "vs base tip" (which would show changes that landed on the base branch after you
  branched).
- Includes staged, unstaged and untracked files. Untracked files are synthesized as all-added,
  because `git diff` cannot see them and in a fresh worktree they are often most of the work.
- Scope is the invocation directory. `git` commands run with the repository root as their working
  directory, so the pathspec is the `--show-prefix` value rather than `.`.
- Paths display relative to the invocation directory.

Base resolution order: `--base`, `$DRIFT_BASE`, `git config drift.base`, `refs/remotes/origin/HEAD`,
`origin/main`, `origin/master`, `main`, `master`. Worktrees are usually detached, which is why
`origin/HEAD` comes before the local branches.

## 2. Surfaces and routing

One row model, four frontends:

| stdin | stdout | behaviour |
|---|---|---|
| tty | tty | interactive view |
| pipe | tty | pager: parse the piped patch, render with colour |
| any | pipe | plain text, no ANSI, never interactive |
| any | any, `--json` / `--list` | machine output regardless of tty |

`--tui` opens `/dev/tty` for keys so a piped patch can still be explored. `--color always|never`
overrides the tty guess. `NO_COLOR` only downgrades colour; it never changes routing.

An empty stdin falls back to the repository. CI and cron hand you a non-tty stdin with nothing on
it, and silently showing nothing there would be a trap.

## 3. Pipeline

```
git ──► patch parser ──► ChangeSet ──► rows ──┬──► split / unified / result-only (ANSI)
                                             ├──► plain text
                                             ├──► JSON
                                             └──► TUI viewport
```

Rows are independent of terminal width. Wrapping and truncation happen at draw time. That is what
keeps the pager and the viewport from disagreeing, and it makes layout testable as plain strings.

## 4. Data model

```go
ChangeSet{ Base, MergeBase, Head, Scope string; Files []*File }
File{ OldPath, NewPath string; Status Status; OldMode, NewMode string
      Binary bool; Additions, Deletions int; Hunks []Hunk; Note string }
Hunk{ OldStart, OldCount, NewStart, NewCount int; Section string; Lines []Line }
Line{ Kind Kind; Text string; OldNo, NewNo int }
```

`Text` never carries the leading sign or a trailing newline. Tabs are expanded to four spaces in
the *layout* copy only, so the JSON and the plain output stay byte-faithful to the patch.

## 5. Parsing

drift parses patches; it does not implement git. Rename detection, binary detection, mode changes,
submodules and text conversion all come from the real thing.

Leniency is deliberate: a line the parser does not understand is ignored, never fatal. The parser
tests enumerate the shapes that actually break naive implementations:

- lines longer than `bufio.Scanner`'s 64 KB token limit (we use `bufio.Reader`)
- `\ No newline at end of file`
- C-quoted paths and paths containing spaces, which make `diff --git a/x b/x` ambiguous
- `rename from`/`rename to`, `copy from`/`copy to`
- `old mode`/`new mode`, `new file mode`, `deleted file mode`
- `Binary files ... differ` and `GIT binary patch` (whose base85 payload must not leak into the
  next file)
- `--- /dev/null`, `+++ /dev/null`
- CRLF content
- colour codes, because git colours its output when it thinks its pager is a terminal — which is
  exactly the case when drift is `core.pager`

Combined diffs (`diff --cc`) are not modelled and degrade to no rows rather than mis-rendering.

## 6. Highlighting

The obvious approach — tokenise the hunk — breaks any block comment or raw string that starts above
the hunk, and needs an incremental lexer Chroma does not have.

Instead: read the file from disk, tokenise it once, index the spans by line number. Additions and
context lines map through the new side, deletions through the old side (`git show <merge-base>:path`
when the file no longer exists). Correct multi-line state, no incremental lexing, one file read.

Highlighting is deferred: rows paint as plain text immediately, files on screen are tokenised off
the main goroutine, and the view redraws when a table lands. A nil table always means "render
plain", so the failure mode is dull, not broken.

## 7. Layout

- **Split** pairs each deletion run with the following addition run, one-to-one, padding the
  shorter side. Missing halves paint as a filler column.
- **Unified** shows the runs in patch order.
- **Result-only** drops deletions entirely: read what the file became.
- **Emphasis** runs a word-level LCS over paired lines and marks only the differing tokens. Lines
  that are too large or too dissimilar are left undivided rather than speckled.
- Split rows truncate; single-column rows wrap. Comparing two wrapped columns is worse than
  truncating them, and a diff is usually narrow code.

## 8. Colour

Structure uses the terminal's own 16 colours where possible; syntax uses truecolor only when
`COLORTERM` says so, with a 256-colour fallback. Two themes ship, because a background-tinted band
looks wrong on the opposite background. `auto` reads `COLORFGBG` and otherwise assumes dark.

Never assume a background colour for text that is not part of a band. That is the terminal
equivalent of adapting to the host theme.

## 9. Agent contract

```sh
drift --list              status, path, +n, -n per file
drift --file src/a.go     one file
drift --json              versioned structure
```

Guarantees: no colour unless asked, no pagination, no prompting, deterministic ordering, stable
schema (`version: 1`). `--context` passes through, so `--context 0` is the token-frugal mode.
`--exit-code` returns 1 when anything changed, 0 when nothing did; errors are 2.

This is deliberately the same shape as `git diff --stat` then `git diff -- path`, so an agent does
not have to learn anything.

## 10. Open questions

1. A file rail for changesets with dozens of files. `n`/`p` covers the common case, but a list is
   better when you cannot see the boundaries.
2. `--watch` currently re-walks the watched tree only at startup; directories created later are
   added when the event arrives, but a moved tree can outrun it.
3. Highlighting in pager mode. It needs hunk-scoped tokenising, because the working tree is not
   necessarily the revision that produced a piped patch, and colouring from the wrong file is
   worse than no colour.
4. Inline image diffs through the kitty/iTerm graphics protocol.
