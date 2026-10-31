// Package config reads drift's configuration file. The file is optional and
// small on purpose: it holds the handful of choices worth remembering, the key
// bindings, and any colour overrides to layer on a theme.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/fschrhunt/drift/internal/keys"
	"github.com/fschrhunt/drift/internal/paths"
	"github.com/fschrhunt/drift/internal/theme"
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

// Default is what drift does with no config file at all: follow the terminal,
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
		if errors.Is(err, os.ErrNotExist) && explicit == "" && os.Getenv("DRIFT_CONFIG") == "" {
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

// Validate rejects values drift cannot honour, so a typo fails loudly instead
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

// Template is the commented starter file written by `drift init`.
func Template() string {
	dir, _ := paths.Dir()
	return `# drift configuration
#
# Everything here is optional. Delete a line to get the built-in behaviour back.
# This file is read from:
#   ` + dir + `/config.toml
#
# Override the location with --config or $DRIFT_CONFIG.

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
