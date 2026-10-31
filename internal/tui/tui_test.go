package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fschrhunt/drift/internal/git"
	"github.com/fschrhunt/drift/internal/layout"
	"github.com/fschrhunt/drift/internal/patch"
	"github.com/fschrhunt/drift/internal/render"
)

func fixture() *patch.ChangeSet {
	return &patch.ChangeSet{
		Base: "main",
		Files: []*patch.File{{
			NewPath:   "internal/render/render.go",
			Status:    patch.Modified,
			Additions: 2,
			Deletions: 1,
			Hunks: []patch.Hunk{{
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Section: "func main()",
				Lines: []patch.Line{
					{Kind: patch.Context, Text: "package main", OldNo: 1, NewNo: 1},
					{Kind: patch.Del, Text: "\toldValue := 1", OldNo: 2},
					{Kind: patch.Add, Text: "\tnewValue := 2", NewNo: 2},
					{Kind: patch.Add, Text: "\tprintln(newValue)", NewNo: 3},
					{Kind: patch.Context, Text: "}", OldNo: 3, NewNo: 4},
				},
			}},
		}},
	}
}

func newModel(mode layout.Mode) *Model {
	m := New(fixture(), Options{
		Mode:    mode,
		Numbers: true,
		Repo:    &git.Repo{},
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	return m
}

func TestViewRendersEveryRow(t *testing.T) {
	m := newModel(layout.Unified)
	m.r.Depth = render.TrueColor
	view := m.View()
	if view == "" {
		t.Fatal("empty view")
	}
	plain := stripANSI(view)
	for _, want := range []string{"internal/render/", "render.go", "newValue", "package main"} {
		if !strings.Contains(plain, want) {
			t.Errorf("view is missing %q:\n%s", want, plain)
		}
	}
	t.Log("\n" + plain)
}

func TestSplitShowsBothSides(t *testing.T) {
	m := newModel(layout.Split)
	m.r.Depth = render.TrueColor
	plain := stripANSI(m.View())
	t.Log("\n" + plain)
	line := ""
	for _, l := range strings.Split(plain, "\n") {
		if strings.Contains(l, "oldValue") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("old side missing:\n%s", plain)
	}
	if !strings.Contains(line, "newValue") {
		t.Errorf("split row should show both sides: %q", line)
	}
	if !strings.Contains(line, "│") {
		t.Errorf("split row should have a divider: %q", line)
	}
}

func TestModeCycleVisitsAllThree(t *testing.T) {
	m := newModel(layout.Split)
	want := []layout.Mode{layout.Unified, layout.New, layout.Split}
	for _, w := range want {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
		if m.mode != w {
			t.Fatalf("mode = %v, want %v", m.mode, w)
		}
	}
}

func TestNewModeHidesDeletions(t *testing.T) {
	m := newModel(layout.New)
	plain := stripANSI(m.View())
	if strings.Contains(plain, "oldValue") {
		t.Errorf("new mode should hide deletions:\n%s", plain)
	}
	if !strings.Contains(plain, "newValue") {
		t.Errorf("new mode should keep additions:\n%s", plain)
	}
}

func TestSearchFindsAndJumps(t *testing.T) {
	m := newModel(layout.Unified)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "newValue" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.query != "newValue" {
		t.Fatalf("query = %q", m.query)
	}
	if len(m.matches) == 0 {
		t.Fatal("no matches")
	}
	if !strings.Contains(stripANSI(m.View()), "1/") {
		t.Errorf("status bar should show match position")
	}
}

func TestScrollClampsAtEdges(t *testing.T) {
	m := newModel(layout.Unified)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.top != 0 {
		t.Fatalf("top = %d after scrolling up past the start", m.top)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if m.top != m.maxTop() {
		t.Fatalf("top = %d, want %d", m.top, m.maxTop())
	}
}

// stripANSI removes escape sequences for assertions that look at text only.
func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc && r == 'm':
			esc = false
		case !esc:
			b.WriteRune(r)
		}
	}
	return b.String()
}
