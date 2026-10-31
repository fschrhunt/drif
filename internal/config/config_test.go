package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/drift/internal/keys"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadOverridesDefaults(t *testing.T) {
	path := writeConfig(t, `
theme = "mine"
layout = "split"
numbers = false
context = 7
untracked = false
watch = true
base = "origin/main"

[keys]
quit = "z"
down = ["e", "down"]
`)
	cfg, gotPath, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != path {
		t.Errorf("path = %q", gotPath)
	}
	if cfg.Theme != "mine" || cfg.Layout != "split" || cfg.Context != 7 {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.Numbers || cfg.Untracked || !cfg.Watch || cfg.Base != "origin/main" {
		t.Errorf("cfg = %+v", cfg)
	}
	// A bare string and a list must both work.
	if got := cfg.Keys[keys.Quit]; len(got) != 1 || got[0] != "z" {
		t.Errorf("quit keys = %v", got)
	}
	km, err := cfg.KeyMap()
	if err != nil {
		t.Fatal(err)
	}
	if km.Key["e"] != keys.Down {
		t.Errorf("e = %q", km.Key["e"])
	}
	if _, ok := km.Key["j"]; ok {
		t.Errorf("j should have been replaced")
	}
}

func TestUnknownKeyIsAnError(t *testing.T) {
	path := writeConfig(t, "theme = \"dark\"\nwat = true\n")
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `unknown key "wat"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestBadLayoutIsRejected(t *testing.T) {
	path := writeConfig(t, "layout = \"diagonal\"\n")
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "layout") {
		t.Fatalf("err = %v", err)
	}
}

func TestBadBindingIsRejected(t *testing.T) {
	path := writeConfig(t, "[keys]\nquit = [\"j\"]\n")
	_, _, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "bound to both") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingDefaultConfigIsNotAnError(t *testing.T) {
	t.Setenv("DRIFT_CONFIG_DIR", t.TempDir())
	t.Setenv("DRIFT_CONFIG", "")
	cfg, _, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "auto" || !cfg.Numbers || cfg.Context != 3 {
		t.Fatalf("defaults = %+v", cfg)
	}
}

func TestMissingExplicitConfigIsAnError(t *testing.T) {
	if _, _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestTemplateParses(t *testing.T) {
	path := writeConfig(t, Template())
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("the template does not parse: %v", err)
	}
	// Commented examples must not be in effect.
	if len(cfg.Keys) != 0 {
		t.Errorf("template set keys: %v", cfg.Keys)
	}
}
