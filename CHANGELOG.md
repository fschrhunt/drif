# drift releases

## Unreleased

- First version. Run `drift` in a worktree to review everything changed since the merge base with
  the base branch, including untracked files.
- Three layouts: side-by-side comparison, unified, and result-only. One key cycles them.
- Syntax highlighting tokenises the file rather than the hunk, so multi-line constructs stay
  correct, and fills in after the first paint instead of blocking it.
- Word-level emphasis inside changed line pairs, so a one-character edit reads as a one-character
  edit.
- `drift --list`, `drift --file PATH` and `drift --json` give scripts and agents a cheap index and
  a stable structure.
- Pager mode for `git diff | drift` and `git config core.pager drift`; previously coloured patches
  are stripped before parsing.
- `--watch` reloads the review surface as files change.
