package keys

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildBindsDefaults(t *testing.T) {
	m, err := Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Key["j"]; got != Down {
		t.Errorf("j = %q, want %q", got, Down)
	}
	if got := m.Key[" "]; got != PageDown {
		t.Errorf("space = %q, want %q", got, PageDown)
	}
	if got := m.Action[NextFile]; !reflect.DeepEqual(got, []string{"tab", "]"}) {
		t.Errorf("next_file keys = %v", got)
	}
}

func TestOverrideReplacesEveryKeyOfAnAction(t *testing.T) {
	m, err := Build(map[string][]string{Quit: {"z"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Key["z"]; got != Quit {
		t.Errorf("z = %q, want quit", got)
	}
	if _, still := m.Key["q"]; still {
		t.Errorf("q should no longer be bound to quit")
	}
}

func TestUnknownActionIsRejected(t *testing.T) {
	_, err := Build(map[string][]string{"scroll_sideways": {"h"}})
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("err = %v", err)
	}
}

func TestConflictingBindingIsRejected(t *testing.T) {
	_, err := Build(map[string][]string{Quit: {"j"}})
	if err == nil || !strings.Contains(err.Error(), "bound to both") {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"space":      " ",
		"escape":     "esc",
		"CTRL+d":     "ctrl+d",
		"ctrl+Shift": "ctrl+shift",
		"G":          "G",
		"pgdown":     "pgdown",
	}
	for in, want := range cases {
		got, err := Normalize(in)
		if err != nil {
			t.Errorf("Normalize(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeRejectsSequences(t *testing.T) {
	if _, err := Normalize("gg"); err == nil {
		t.Fatal("expected an error for a multi-key sequence")
	}
}
