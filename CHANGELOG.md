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
- Configuration at `~/.config/drift/config.toml`: layout, context, numbers, base, watch and colour
  overrides. `drift init` writes a commented starter file and `drift config` prints what is
  actually in effect.
- Themes: `~/.config/drift/themes/<name>.toml`, layered over a built-in with `extends`, so a theme
  can be a single colour. `drift theme` lists them and `drift theme NAME` prints one as a file to
  copy.
- Every key is rebindable in `[keys]`, with conflicts reported instead of silently resolved.
- `.drift.toml` at the repository root, declaring only `base`. A repository may say what to compare
  against, not how the diff is painted; `drift init --repo` writes one.
- A command line inside the viewer: `:theme`, `:layout`, `:numbers`, `:reload`, `:q`. `R` re-reads
  the config from disk, so a theme can be edited and seen without restarting.
