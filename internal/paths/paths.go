// Package paths answers where drift keeps its files. One place, so the config,
// the theme loader and `drift init` cannot disagree about the directory.
package paths

import (
	"os"
	"path/filepath"
)

// Dir is drift's configuration directory. DRIFT_CONFIG_DIR wins, then
// XDG_CONFIG_HOME, then ~/.config — the same place a developer expects to find
// a tool like this on every platform.
func Dir() (string, error) {
	if d := os.Getenv("DRIFT_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "drift"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "drift"), nil
}

// ConfigFile is the config path: explicit flag, then DRIFT_CONFIG, then
// config.toml inside Dir.
func ConfigFile(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if p := os.Getenv("DRIFT_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// ThemesDir is where named themes live.
func ThemesDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "themes"), nil
}
