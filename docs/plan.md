# drift — design

A terminal diff viewer for a worktree. Run `drift` anywhere inside a repository and see everything
that changed from the base branch, as one continuous review surface. The same binary serves people
and agents.

## What this is

One command, run inside a worktree, that shows you what you did there — rendered the way a good
review UI would render it, using nothing but the terminal you already have.

Six decisions follow from that sentence, and most of the code exists to keep them true:

1. **Nothing to set up.** The defaults are the product: no `init`, no profile, no daemon. A config
   file exists for the handful of choices worth remembering, and it is entirely optional — delete
   it and drift still knows what to do. A setting worth a flag someone types twice a year is worth
   inferring instead.
2. **Borrow the host, never impose on it.** Colours come from the terminal. Nobody is asked to pick
   a theme, because the terminal was already themed.
3. **One surface, not a file viewer.** The whole change set is a single scrollable document. That
   is what separates a review surface from a pager.
4. **Dead simple by default, capable when asked.** Bare `drift` means one thing. Everything else is
   a keystroke, not a menu.
5. **Two audiences, one model.** A person gets colour and layout; an agent gets the same rows as
   plain text or JSON, in the `--list` then `--file` shape it already knows. There is no agent mode.
6. **Render diffs; do not reimplement git.** Git decides what changed. drift owns how it looks.

Anti-goals: a git client, a merge tool, a plugin host, a scripting surface, a file browser. The
config is a flat file of choices; it does not run code.

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

Two palettes ship, one for dark backgrounds and one for light, and drift picks between them from
`COLORFGBG` when the terminal exports it, otherwise by asking the terminal for its background
(OSC 11), and otherwise by assuming dark. There is no theme option on purpose: the terminal is already themed, and a viewer that asks
the user to describe their own screen has failed at the one thing it can see for free.

Structure uses the terminal's own colours where possible; syntax uses truecolor only when
`COLORTERM` says so, with a 256-colour fallback. Never assume a background colour for text that is
not part of a band — that is the terminal equivalent of adapting to the host theme, and it is why
the output blends instead of fighting you.

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

## 10. Configuration

The file is TOML, at `$XDG_CONFIG_HOME/drift/config.toml`, and it only ever holds values, never
code. Precedence is defaults, then the file, then flags that were actually passed — `fs.Visit` is
what makes "actually passed" knowable, so a flag left at its default never clobbers a configured
value.

Unknown keys are errors. A typo that silently does nothing is worse than a startup failure, and the
same applies to unknown actions, unknown themes, unparseable colours and two actions fighting over
one key.

Key bindings are a map from action name to key list. Actions are the stable vocabulary; keys are
the unstable part. Replacing an action's list replaces all of its keys, which is what
`down = ["n"]` reads as. `keys.Map` keeps both directions so help can print bindings in the order
they were written.

The command line exists so the loop is edit, look, edit. `R` re-reads the config file and rebuilds
the view in place; `:theme` re-resolves one theme without touching the file. That is the whole
reason configuration is worth having in an interactive tool: the feedback is immediate.

## 11. Open questions

1. A file rail for changesets with dozens of files. `n`/`p` covers the common case, but a list is
   better when you cannot see the boundaries.
2. `--watch` currently re-walks the watched tree only at startup; directories created later are
   added when the event arrives, but a moved tree can outrun it.
3. Highlighting in pager mode. It needs hunk-scoped tokenising, because the working tree is not
   necessarily the revision that produced a piped patch, and colouring from the wrong file is
   worse than no colour.
4. Inline image diffs through the kitty/iTerm graphics protocol.
5. A project-local `.drift.toml`. Deliberately absent for now: a repository should not be able to
   change how reviewing it feels, and the global file covers the real cases.
6. Whether themes should be able to override the *structure* (gutters, separators), which is
   currently fixed so every theme keeps the same geometry.
