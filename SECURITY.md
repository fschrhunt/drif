# Security

Report vulnerabilities through [GitHub private vulnerability reporting](https://github.com/fschrhunt/drif/security/advisories/new)
for `fschrhunt/drif`.

## What drif does with a repository

drif runs `git` as you and draws the patch it returns. Reviewing a repository
does not run that repository's code: `git diff` executes no hooks, drif passes
`--no-ext-diff` and `--no-textconv`, and nothing in a checkout can configure a
program for git to run. A `.drif.toml` is declarative and may only name a base
revision; themes and keybindings are read from your own config directory, never
from a repository.

The exception worth stating plainly: a repository you review can still contain
commands and scripts, and reading them changes nothing. Treat anything you later
execute as you would any untrusted code.

## Terminal output

A diff is arbitrary text, and a terminal acts on control characters. A file
containing `ESC` could otherwise move the cursor, change colours, or set the
clipboard through OSC 52 while you are reading a review.

drif replaces C0 controls and DEL with their Unicode control pictures
(`␛`, `␇`, `␡`) before drawing, in the interactive view, the pager and the
plain output; tabs are widened to spaces in laid-out views. A file's own
escapes survive parsing and are made visible rather than deleted, so what you
see is what the file contains. `--json` is unaffected and returns the patch as
git produced it.

This is a display guarantee, not a sandbox. Piping drif's output somewhere that
reinterprets it is outside its control.

## Configuration

Config and theme files are TOML values: no scripts, no includes, no code.

`drif --watch` uses the platform's file notification API and reads only inside
the repository. drif makes no network requests; resolving the base and the merge
base reads local references and never fetches.
