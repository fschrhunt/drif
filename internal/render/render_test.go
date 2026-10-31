package render

import (
	"strings"
	"testing"

	"github.com/fschrhunt/drift/internal/patch"
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
