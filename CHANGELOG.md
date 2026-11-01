# drif releases

## Unreleased

- The interactive worktree view updates automatically; Git reloads run in the background.
  The `--watch` flag and `watch` setting are no longer needed.

## v0.1.1 · 2026-11-01

- Monochrome: colour now means added or removed and nothing else. Only changed lines paint a
  background; headers, hunk separators and metadata are text on the terminal's own background.
  Syntax highlighting is gone with its lexer cache.
- The file header is pinned to the top on a dim surface blended from the terminal's own
  background, and follows the file being read.
- Hunk boundaries read `⋯ 9 lines` or `⋯ func main()`, not `@@ -1,8 +1,9 @@`.
- Split view joins the panes at the seam, and the empty half of an unpaired change stays blank.
- Long lines wrap in both layouts; there is no horizontal scrolling and no toggle.
- `/` and `?` open a floating command palette with a real caret. Search lives there now, listing
  matching lines with `file:line`; the bottom search bar is gone.
- Page scrolling overlaps by two lines, the prompt overlays the bottom row instead of moving the
  diff, and the wheel scrolls through alternate scroll mode without capturing the mouse.
- Frames are presented atomically, so scrolling does not tear on a slow link.

## v0.1.0 · 2026-10-31

- First version. Run `drif` in a worktree to review everything changed since the merge base with
  the base branch, including untracked files.
- Three layouts: side-by-side comparison, unified, and result-only. One key cycles them.
- Syntax highlighting tokenises the file rather than the hunk, so multi-line constructs stay
  correct, and fills in after the first paint instead of blocking it.
- Word-level emphasis inside changed line pairs, so a one-character edit reads as a one-character
  edit.
- `drif --list`, `drif --file PATH` and `drif --json` give scripts and agents a cheap index and
  a stable structure.
- Terminal control characters in file content are drawn as visible pictures instead of being sent
  to the terminal, in the interactive view, the pager and the plain output. `--json` is unchanged.
- Pager mode for `git diff | drif` and `git config core.pager drif`; previously coloured patches
  are stripped before parsing.
- `--watch` reloads the review surface as files change.
- Configuration at `~/.config/drif/config.toml`: layout, context, numbers, base, watch and colour
  overrides. `drif init` writes a commented starter file and `drif config` prints what is
  actually in effect.
- Themes: `~/.config/drif/themes/<name>.toml`, layered over a built-in with `extends`, so a theme
  can be a single colour. `drif theme` lists them and `drif theme NAME` prints one as a file to
  copy.
- Every key is rebindable in `[keys]`, with conflicts reported instead of silently resolved.
- Repositories with no commits yet are compared against the empty tree, so a brand new project
  shows its first files instead of asking for a base branch.
- Plain `diff -u` output (not just git's) can be piped in.
- Deleted files no longer report a new path, and the plain output now carries the rename, mode and
  binary notes the interactive view already showed.
- `.drif.toml` at the repository root, declaring only `base`. A repository may say what to compare
  against, not how the diff is painted; `drif init --repo` writes one.
- A command line inside the viewer: `:theme`, `:layout`, `:numbers`, `:reload`, `:q`. `R` re-reads
  the config from disk, so a theme can be edited and seen without restarting.
