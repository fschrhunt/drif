// Package theme turns colour descriptions into a render palette. A theme is
// either one of the two built-ins, a file in the user's themes directory, or
// the automatic choice made from the terminal's own background.
//
// Everything is layered: a theme names its base with extends, then overrides
// whatever it cares about, and the user's config can override the result again.
// That is what makes a one-line theme possible.
package theme

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/fschrhunt/drif/internal/paths"
	"github.com/fschrhunt/drif/internal/render"
)

// ColorSet is the complete, optional description of a palette. Every field is
// a "#rrggbb" string, and an empty field means "inherit".
type ColorSet struct {
	Text   string `toml:"text"`
	Dim    string `toml:"dim"`
	Accent string `toml:"accent"`

	FileFG  string `toml:"file_fg"`
	FileBG  string `toml:"file_bg"`
	FileDim string `toml:"file_dim"`
	HunkFG  string `toml:"hunk_fg"`

	AddFG       string `toml:"add_fg"`
	AddBG       string `toml:"add_bg"`
	AddEmphasis string `toml:"add_emphasis"`

	DelFG       string `toml:"del_fg"`
	DelBG       string `toml:"del_bg"`
	DelEmphasis string `toml:"del_emphasis"`
}

// File is the on-disk shape of a theme.
type File struct {
	// Extends names the base theme: "dark", "light", or "auto" (the default)
	// to follow the terminal.
	Extends string   `toml:"extends"`
	Colors  ColorSet `toml:"colors"`
}

// Resolve produces the palette for a theme name, then applies the config's
// overrides on top.
func Resolve(name string, overrides ColorSet) (*render.Palette, error) {
	p, err := resolve(name, nil)
	if err != nil {
		return nil, err
	}
	if err := Apply(p, overrides); err != nil {
		return nil, err
	}
	return p, nil
}

func resolve(name string, chain []string) (*render.Palette, error) {
	switch name {
	case "", "auto":
		return render.DetectPalette(), nil
	case "dark":
		return render.Dark(), nil
	case "light":
		return render.Light(), nil
	}
	for _, seen := range chain {
		if seen == name {
			return nil, fmt.Errorf("theme %q extends itself", name)
		}
	}

	path, err := Find(name)
	if err != nil {
		return nil, err
	}
	var f File
	md, err := toml.DecodeFile(path, &f)
	if err != nil {
		return nil, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}

	base, err := resolve(f.Extends, append(chain, name))
	if err != nil {
		return nil, err
	}
	if err := Apply(base, f.Colors); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return base, nil
}

// Apply writes every non-empty colour in cs onto p. Errors name the field so a
// typo in a theme file is easy to find.
func Apply(p *render.Palette, cs ColorSet) error {
	fields := []struct {
		name string
		val  string
		set  func(render.Color)
	}{
		{"text", cs.Text, func(c render.Color) { p.Text = c }},
		{"dim", cs.Dim, func(c render.Color) { p.Dim = c }},
		{"accent", cs.Accent, func(c render.Color) { p.Accent = c }},

		{"file_fg", cs.FileFG, func(c render.Color) { p.FileFg = c }},
		{"file_bg", cs.FileBG, func(c render.Color) { p.FileBg = c }},
		{"file_dim", cs.FileDim, func(c render.Color) { p.FileDim = c }},
		{"hunk_fg", cs.HunkFG, func(c render.Color) { p.HunkFg = c }},

		{"add_fg", cs.AddFG, func(c render.Color) { p.AddFg = c }},
		{"add_bg", cs.AddBG, func(c render.Color) { p.AddBg = c }},
		{"add_emphasis", cs.AddEmphasis, func(c render.Color) { p.AddEmphBg = c }},
		{"del_fg", cs.DelFG, func(c render.Color) { p.DelFg = c }},
		{"del_bg", cs.DelBG, func(c render.Color) { p.DelBg = c }},
		{"del_emphasis", cs.DelEmphasis, func(c render.Color) { p.DelEmphBg = c }},
	}
	for _, f := range fields {
		if f.val == "" {
			continue
		}
		c := render.Hex(strings.TrimSpace(f.val))
		if !c.Set {
			return fmt.Errorf("%s: %q is not a #rrggbb colour", f.name, f.val)
		}
		f.set(c)
	}
	return nil
}

// ToColorSet renders a palette back into a theme file, so a user can dump the
// built-ins and start from something real.
func ToColorSet(p *render.Palette) ColorSet {
	h := func(c render.Color) string {
		if !c.Set {
			return ""
		}
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return ColorSet{
		Text: h(p.Text), Dim: h(p.Dim), Accent: h(p.Accent),
		FileFG: h(p.FileFg), FileBG: h(p.FileBg), FileDim: h(p.FileDim), HunkFG: h(p.HunkFg),
		AddFG: h(p.AddFg), AddBG: h(p.AddBg), AddEmphasis: h(p.AddEmphBg),
		DelFG: h(p.DelFg), DelBG: h(p.DelBg), DelEmphasis: h(p.DelEmphBg),
	}
}

// Find returns the path of a theme file by name.
func Find(name string) (string, error) {
	dir, err := paths.ThemesDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".toml")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("theme %q: %w", name, err)
	}
	return path, nil
}

// List returns the built-in theme names and every theme file installed.
func List() ([]string, error) {
	names := []string{"auto", "dark", "light"}
	dir, err := paths.ThemesDir()
	if err != nil {
		return names, nil // a missing config directory is not an error
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return names, nil
		}
		return names, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".toml"))
	}
	sort.Strings(names[3:])
	return names, nil
}
