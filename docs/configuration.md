# Configuration

drif works with no configuration at all. This page is the reference for when you want to change
something.

## Where it lives

```
~/.config/drif/config.toml
```

`$XDG_CONFIG_HOME/drif/config.toml` when that is set, and `$DRIFT_CONFIG_DIR` overrides the
directory. `--config PATH` overrides the file, and `$DRIF_CONFIG` names a file directly.

Precedence is defaults, then the repository's `.drif.toml`, then your config, then flags that were
actually passed. A flag left at its default never overwrites a value you configured.

```sh
drif init      # write a commented starter file
drif config    # print the effective values, the resolved base, themes and keymap
```

Every key is optional: delete a line and the built-in behaviour comes back.

## Options

| key | default | meaning |
| --- | --- | --- |
| `theme` | `"auto"` | `auto`, `dark`, `light`, or the name of a file in `themes/` |
| `layout` | `"auto"` | `auto`, `split`, `unified`, `new` — `auto` picks split on a wide terminal |
| `context` | `3` | lines of context around each change |
| `numbers` | `true` | show line numbers |
| `untracked` | `true` | include files git does not track |
| `watch` | `false` | reload as files change |
| `base` | unset | revision to compare against |

```toml
theme = "auto"
layout = "auto"
context = 3
numbers = true
untracked = true
watch = false
# base = "origin/main"
```

Unknown keys are an error, because a typo that silently does nothing is worse than a startup
failure.

## Per repository

A repository may declare one thing: what it compares against. Put `.drif.toml` at the repository
root, and `drif init --repo` writes one.

```toml
base = "develop"
```

That is the whole schema. Themes, colours, keys, layout and context are rejected in that file with
an explanation: a repository may say **what** to compare against, never **how** the diff is
painted. A review tool whose presenter can be set by the thing being reviewed can be made to lie.

Your own `base` wins over the repository's, and `--base` wins over both.

## Themes

Two palettes ship and drif picks between them by asking the terminal for its background. To change
the look, layer on top of it.

```toml
[colors]
add_bg = "#10241a"
del_bg = "#2a1417"
add_fg = "#07c480"
del_fg = "#ff2e3f"
```

For a whole look, write `~/.config/drif/themes/<name>.toml` and set `theme = "<name>"`:

```toml
extends = "dark"     # dark, light, auto, or another theme

[colors]
add_fg = "#b8f397"
```

`extends` inherits everything else, so a theme can be one line. Leaving it out follows the
terminal, the same as `extends = "auto"`.

Available colour names:

```
text dim accent file_fg file_dim file_bg
add_fg add_bg add_emphasis
del_fg del_bg del_emphasis
hunk_fg
```

```sh
drif theme              # list installed themes
drif theme dark         # print the resolved colours as a theme file to copy
```

Inside the viewer, `:theme <name>` switches immediately and `R` re-reads the config from disk, so
you can keep the theme file open in another window while you tune it.

## Key bindings

Every action is rebindable in `[keys]`. The names on the left are the stable vocabulary; the keys on
the right are whatever you like.

```toml
[keys]
quit = ["x"]
next_file = ["J"]
down = ["e", "down"]
```

Replacing an action's list replaces all of that action's keys. A key can only do one thing: binding
the same key to two actions is an error, not a silent override.

Keys are written the way your terminal reports them: `j`, `G`, `ctrl+d`, `tab`, `shift+tab`,
`pgdown`, `space`, `enter`, `esc`. Multi-key sequences such as `gg` are not supported.

### Actions

```
quit  down  up  half_down  half_up  page_down  page_up  top  bottom
next_file  prev_file  next_hunk  prev_hunk
cycle_layout  toggle_numbers  search  next_match  prev_match
help  command  reload
```

### Defaults

```
j k ↓ ↑      line                 space b         page
g G          top / bottom         } {             next / prev hunk
tab ]        next file            shift+tab [     prev file
s            split → unified → new
/            search               n N             next / prev match
t            line numbers         :               command
R            reload config        ?               palette
q            quit
```

`?` opens the command palette, and its list always shows the live binding beside each action.

## In-viewer command line

`:command` opens a line at the bottom for the few things worth typing:

```
:theme <name>    :layout split|unified|new    :numbers    :reload    :q
```

It exists so a theme can be switched without the palette's fixed list, and so `R` has a companion
for scripted muscle memory.
