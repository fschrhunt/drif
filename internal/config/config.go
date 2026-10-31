// Package config reads drif's configuration file. The file is optional and
// small on purpose: it holds the handful of choices worth remembering, the key
// bindings, and any colour overrides to layer on a theme.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/fschrhunt/drif/internal/keys"
	"github.com/fschrhunt/drif/internal/paths"
	"github.com/fschrhunt/drif/internal/theme"
)

// KeyList accepts either "q" or ["q", "ctrl+c"] in the [keys] table, because
// making someone write a one-element array is a pointless tax.
type KeyList []string

// UnmarshalTOML implements toml.Unmarshaler.
func (k *KeyList) UnmarshalTOML(v any) error {
	switch t := v.(type) {
	case string:
		*k = KeyList{t}
		return nil
	case []any:
		out := make(KeyList, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("key bindings must be strings")
			}
			out = append(out, s)
		}
		*k = out
		return nil
	}
	return fmt.Errorf("key bindings must be a string or a list of strings")
}

// Config is the whole configuration. Zero values mean "use the default", so
// decoding always starts from Default.
type Config struct {
	Theme     string             `toml:"theme"`
	Layout    string             `toml:"layout"`
	Numbers   bool               `toml:"numbers"`
	Context   int                `toml:"context"`
	Untracked bool               `toml:"untracked"`
	Watch     bool               `toml:"watch"`
	Base      string             `toml:"base"`
	Keys      map[string]KeyList `toml:"keys"`
	Colors    theme.ColorSet     `toml:"colors"`
}

// Layouts are the accepted values of layout.
var Layouts = []string{"auto", "split", "unified", "new"}

// Default is what drif does with no config file at all: follow the terminal,
// pick the layout from the width, and include everything uncommitted.
func Default() *Config {
	return &Config{
		Theme:     "auto",
		Layout:    "auto",
		Numbers:   true,
		Context:   3,
		Untracked: true,
		Watch:     false,
		Keys:      map[string]KeyList{},
	}
}

// Load reads the config, returning the effective values and the path that was
// used. A missing file at the default location is not an error; a missing file
// named explicitly is, because the user asked for it by name.
func Load(explicit string) (*Config, string, error) {
	path, err := paths.ConfigFile(explicit)
	if err != nil {
		return nil, "", err
	}
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && explicit == "" && os.Getenv("DRIF_CONFIG") == "" {
			return cfg, path, nil
		}
		return nil, path, err
	}

	md, err := toml.Decode(string(data), cfg)
	if err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, path, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}
	if err := cfg.Validate(); err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, path, nil
}

// Validate rejects values drif cannot honour, so a typo fails loudly instead
// of being silently ignored.
func (c *Config) Validate() error {
	if c.Theme == "" {
		return errors.New("theme must not be empty")
	}
	layout := strings.ToLower(c.Layout)
	ok := false
	for _, l := range Layouts {
		if layout == l {
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("layout %q is not one of %s", c.Layout, strings.Join(Layouts, ", "))
	}
	if c.Context < 0 {
		return fmt.Errorf("context must be zero or more, got %d", c.Context)
	}
	if _, err := c.KeyMap(); err != nil {
		return err
	}
	return nil
}

// KeyMap resolves the bindings against the defaults.
func (c *Config) KeyMap() (*keys.Map, error) {
	overrides := make(map[string][]string, len(c.Keys))
	for action, list := range c.Keys {
		overrides[action] = list
	}
	return keys.Build(overrides)
}

// Template is the commented starter file written by `drif init`.
func Template() string {
	dir, _ := paths.Dir()
	return `# drif configuration
#
# Everything here is optional. Delete a line to get the built-in behaviour back.
# This file is read from:
#   ` + dir + `/config.toml
#
# Override the location with --config or $DRIF_CONFIG.

# Theme: "auto" follows the terminal's background. "dark" and "light" are
# built in. Any other name is a file at ` + dir + `/themes/<name>.toml.
theme = "auto"

# Layout: auto chooses split on a wide terminal, unified on a narrow one.
layout = "auto"

# Lines of context around each change.
context = 3

numbers = true
untracked = true
watch = false

# Compare against something other than the detected base branch.
# base = "origin/main"

# Rebind anything. The name on the left is an action; the keys on the right are
# written the way your terminal reports them: "j", "G", "ctrl+d", "tab",
# "shift+tab", "pgdown", "space", "enter", "esc".
#
# Replacing an action's list replaces all of that action's keys.
#
# [keys]
# quit = ["q", "ctrl+c"]
# down = ["j", "down"]
# next_file = ["tab", "]"]
# cycle_layout = ["s"]

# Override any colour from the active theme. Colours are #rrggbb.
#
# [colors]
# add_bg = "#10241a"
# del_bg = "#2a1417"
#
# [colors.syntax]
# keyword = "#ff7b72"
# string = "#a5d6ff"
`
}

// RepoFileName is the repository-level config, and it lives only at the
// repository root. Nesting it per directory would make "which file won" a
// question nobody wants to ask while reading a diff.
const RepoFileName = ".drif.toml"

// Repo is what a repository is allowed to declare: what to compare against.
//
// Not how to compare it. Themes, colours, keys, layout and context are personal
// preferences and stay in the user config, because a review tool whose
// presenter can be set by the thing being reviewed is a review tool that can be
// made to lie.
type Repo struct {
	Base string `toml:"base"`
}

// personal names the settings that are rejected in a repository file, so the
// error can explain why instead of calling a familiar word "unknown".
var personal = map[string]bool{
	"theme": true, "colors": true, "keys": true, "layout": true,
	"numbers": true, "context": true, "untracked": true, "watch": true,
}

// LoadRepo reads <root>/.drif.toml. A missing file is not an error.
func LoadRepo(root string) (*Repo, string, error) {
	path := filepath.Join(root, RepoFileName)
	var r Repo
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &r, "", nil
		}
		return nil, path, err
	}
	md, err := toml.DecodeFile(path, &r)
	if err != nil {
		return nil, path, err
	}
	for _, key := range md.Undecoded() {
		name := key.String()
		if personal[name] {
			return nil, path, fmt.Errorf(
				"%s: %s is a personal preference, not a repository setting; move it to your user config",
				path, name)
		}
		return nil, path, fmt.Errorf("%s: unknown key %q", path, name)
	}
	return &r, path, nil
}

// RepoTemplate is the file `drif init --repo` writes.
func RepoTemplate() string {
	return `# Repository settings for drif.
#
# A repository may say what to compare against. It may not say how the diff is
# painted: themes, colours, keys and layout are personal and belong in
# ~/.config/drif/config.toml.

# The revision the working tree is compared against. When this is unset drif
# uses origin/HEAD, then main, then master.
# base = "main"
`
}

// MergeRepo takes the repository's base, but only when the user has not
// already chosen one. A personal setting always outranks a repository's.
func (c *Config) MergeRepo(r *Repo) {
	if c.Base == "" && r != nil && r.Base != "" {
		c.Base = r.Base
	}
}
