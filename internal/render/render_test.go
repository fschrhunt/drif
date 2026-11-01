package render

import (
	"strings"
	"testing"

	"github.com/fschrhunt/drif/internal/layout"
	"github.com/fschrhunt/drif/internal/patch"
)

func changeset() *patch.ChangeSet {
	return &patch.ChangeSet{
		Base:      "main",
		MergeBase: "abc",
		Head:      "def",
		Files: []*patch.File{{
			NewPath:   "a.go",
			Status:    patch.Modified,
			Additions: 1,
			Deletions: 1,
			Hunks: []patch.Hunk{{
				OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2,
				Lines: []patch.Line{
					{Kind: patch.Context, Text: "same", OldNo: 1, NewNo: 1},
					{Kind: patch.Del, Text: "\told", OldNo: 2},
					{Kind: patch.Add, Text: "\tnew", NewNo: 2},
				},
			}},
		}},
	}
}

func TestPlainLinesHaveNoPaddingsOrColour(t *testing.T) {
	got := strings.Join(PlainLines(changeset(), ""), "\n")
	want := " a.go  +1 -1\n" +
		" @@ -1,2 +1,2 @@\n" +
		" same\n" +
		"-\told\n" +
		"+\tnew"
	if got != want {
		t.Fatalf("plain output:\n%s\nwant:\n%s", got, want)
	}
}

func TestListLinesAreTabSeparated(t *testing.T) {
	got := ListLines(changeset(), "")
	want := "M\ta.go\t+1\t-1"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestJSONKeepsTabsAndReportsCounts(t *testing.T) {
	out, err := JSON(changeset())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"version": 1`, `"path": "a.go"`, `"additions": 1`, `"text": "\told"`} {
		if !strings.Contains(s, want) {
			t.Errorf("json missing %s:\n%s", want, s)
		}
	}
}

func TestColorFGBGDecidesThePalette(t *testing.T) {
	cases := []struct {
		value string
		light bool
		ok    bool
	}{
		{"15;0", false, true},      // black background
		{"0;15", true, true},       // white background
		{"0;7", true, true},        // light grey background
		{"0;8", false, true},       // bright black is still dark
		{"", false, false},         // unset
		{"nonsense", false, false}, // unparseable
	}
	for _, c := range cases {
		t.Setenv("COLORFGBG", c.value)
		light, ok := colorFGBG()
		if light != c.light || ok != c.ok {
			t.Errorf("colorFGBG(%q) = %v,%v want %v,%v", c.value, light, ok, c.light, c.ok)
		}
	}
}

func TestPlainOutputNeutralisesTerminalEscapes(t *testing.T) {
	cs := changeset()
	cs.Files[0].Hunks[0].Lines[2].Text = "evil\x1b]52;c;aGk=\x07 \x1b[2J"
	got := strings.Join(PlainLines(cs, ""), "\n")
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Fatalf("plain output still carries terminal control bytes: %q", got)
	}
	if !strings.Contains(got, "␛") {
		t.Fatalf("escape was not made visible: %q", got)
	}
}

func TestListOutputNeutralisesPaths(t *testing.T) {
	cs := changeset()
	cs.Files[0].NewPath = "evil\x1b[2Jname.go"
	got := strings.Join(ListLines(cs, ""), "\n")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("list output carries an escape: %q", got)
	}
}

func TestHeightMatchesWrappedLines(t *testing.T) {
	long := strings.Repeat("a long line of code that has to wrap ", 4)
	cs := &patch.ChangeSet{Files: []*patch.File{{
		NewPath: "f.go", Status: patch.Modified, Additions: 1, Deletions: 1,
		Hunks: []patch.Hunk{{
			OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2,
			Lines: []patch.Line{
				{Kind: patch.Context, Text: "short", OldNo: 1, NewNo: 1},
				{Kind: patch.Del, Text: long, OldNo: 2},
				{Kind: patch.Add, Text: long + "changed", NewNo: 2},
			},
		}},
	}}}
	for _, mode := range []layout.Mode{layout.Unified, layout.Split} {
		for _, width := range []int{40, 90} {
			r := &Renderer{Pal: Dark(), Width: width, Mode: mode, Numbers: true}
			for _, row := range layout.Build(cs, mode) {
				if got, want := len(r.SplitLines(row)), r.Height(row); got != want {
					t.Errorf("mode=%v width=%d row=%v: %d visual lines, height %d", mode, width, row.Kind, got, want)
				}
			}
		}
	}
}
