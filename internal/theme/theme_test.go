package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/drif/internal/render"
)

func hex(s string) render.Color { return render.Hex(s) }

func withThemes(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DRIF_CONFIG_DIR", dir)
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeTheme(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "themes", name+".toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestThemeExtendsAndOverrides(t *testing.T) {
	dir := withThemes(t)
	writeTheme(t, dir, "mine", `
extends = "light"

[colors]
add_bg = "#010203"

[colors.syntax]
keyword = "#040506"
`)
	p, err := Resolve("mine", ColorSet{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Text != hex("#1f2328") {
		t.Errorf("text = %v, want the light base", p.Text)
	}
	if p.AddBg != hex("#010203") {
		t.Errorf("add_bg = %v", p.AddBg)
	}
	if got := ToColorSet(p); got.Syntax.Keyword != "#040506" {
		t.Errorf("keyword = %q", got.Syntax.Keyword)
	}
}

func TestDefaultBaseFollowsTheTerminal(t *testing.T) {
	dir := withThemes(t)
	writeTheme(t, dir, "bare", "[colors]\nadd_bg = \"#112233\"\n")
	p, err := Resolve("bare", ColorSet{})
	if err != nil {
		t.Fatal(err)
	}
	if p.AddBg != hex("#112233") {
		t.Errorf("add_bg = %v", p.AddBg)
	}
	if !p.Text.Set {
		t.Error("an inherited base should provide text")
	}
}

func TestConfigOverridesApplyLast(t *testing.T) {
	p, err := Resolve("dark", ColorSet{Text: "#000001", Syntax: SyntaxSet{Comment: "#000002"}})
	if err != nil {
		t.Fatal(err)
	}
	got := ToColorSet(p)
	if got.Text != "#000001" || got.Syntax.Comment != "#000002" {
		t.Errorf("overrides = %+v", got)
	}
}

func TestBadColourNamesTheField(t *testing.T) {
	_, err := Resolve("dark", ColorSet{AddBG: "greenish"})
	if err == nil || !strings.Contains(err.Error(), "add_bg") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingThemeIsAnError(t *testing.T) {
	withThemes(t)
	_, err := Resolve("nope", ColorSet{})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v", err)
	}
}

func TestExtendsCycleIsDetected(t *testing.T) {
	dir := withThemes(t)
	writeTheme(t, dir, "a", "extends = \"b\"\n")
	writeTheme(t, dir, "b", "extends = \"a\"\n")
	if _, err := Resolve("a", ColorSet{}); err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownThemeKeyIsAnError(t *testing.T) {
	dir := withThemes(t)
	writeTheme(t, dir, "typo", "[colors]\naddd_bg = \"#112233\"\n")
	if _, err := Resolve("typo", ColorSet{}); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("err = %v", err)
	}
}

func TestListIncludesFiles(t *testing.T) {
	dir := withThemes(t)
	writeTheme(t, dir, "zeta", "")
	writeTheme(t, dir, "alpha", "")
	names, err := List()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"auto", "dark", "light", "alpha", "zeta"} {
		if !strings.Contains(joined, want) {
			t.Errorf("list %v is missing %q", names, want)
		}
	}
	if names[len(names)-1] != "zeta" {
		t.Errorf("file themes should be sorted: %v", names)
	}
}
