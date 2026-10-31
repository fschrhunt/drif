package expand

import (
	"strings"
	"testing"
)

func TestExpandTabs(t *testing.T) {
	if got := Expand("\tx\t"); got != "    x    " {
		t.Fatalf("Expand = %q", got)
	}
}

func TestExpandNeutralisesControlCharacters(t *testing.T) {
	// A file that tries to clear the screen, change colours or set the
	// clipboard through OSC 52 must arrive as text.
	hostile := "before\x1b[2J\x1b]52;c;aGk=\x07after"
	got := Expand(hostile)
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Fatalf("control bytes survived: %q", got)
	}
	if !strings.Contains(got, "␛") {
		t.Fatalf("escape was not made visible: %q", got)
	}
	// Braces are not controls and must survive untouched.
	if got != "before␛[2J␛]52;c;aGk=␇after" {
		t.Fatalf("got %q", got)
	}
}

func TestExpandBounds(t *testing.T) {
	cases := map[string]string{
		"\x00": "␀",
		"\x07": "␇",
		"\r":   "␍",
		"\x1f": "␟",
		"\x7f": "␡",
	}
	for in, want := range cases {
		if got := Expand(in); got != want {
			t.Errorf("Expand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandKeepsTextAndWidth(t *testing.T) {
	const plain = "func main() { println(\"héllo\") }"
	if got := Expand(plain); got != plain {
		t.Fatalf("plain text changed: %q", got)
	}
	// Each control picture is one column, so a line does not reflow.
	if got := Expand("\x01\x02"); len([]rune(got)) != 2 {
		t.Fatalf("width changed: %q", got)
	}
}

func TestBytes(t *testing.T) {
	if got := string(Bytes([]byte("a\tb"))); got != "a    b" {
		t.Fatalf("Bytes = %q", got)
	}
	if got := string(Bytes([]byte("a\x1bb"))); strings.Contains(got, "\x1b") {
		t.Fatalf("Bytes left an escape: %q", got)
	}
}
