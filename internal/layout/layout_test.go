package layout

import (
	"testing"

	"github.com/fschrhunt/drift/internal/patch"
)

func changeset(lines ...patch.Line) *patch.ChangeSet {
	return &patch.ChangeSet{Files: []*patch.File{{
		NewPath: "a.go",
		Status:  patch.Modified,
		Hunks:   []patch.Hunk{{OldStart: 1, NewStart: 1, Lines: lines}},
	}}}
}

func TestSplitPairsUnequalRuns(t *testing.T) {
	cs := changeset(
		patch.Line{Kind: patch.Context, Text: "same", OldNo: 1, NewNo: 1},
		patch.Line{Kind: patch.Del, Text: "old one", OldNo: 2},
		patch.Line{Kind: patch.Del, Text: "old two", OldNo: 3},
		patch.Line{Kind: patch.Add, Text: "new one", NewNo: 2},
		patch.Line{Kind: patch.Add, Text: "new two", NewNo: 3},
		patch.Line{Kind: patch.Add, Text: "new three", NewNo: 4},
	)
	rows := Build(cs, Split)

	var lines []Row
	for _, r := range rows {
		if r.Kind == LineRow {
			lines = append(lines, r)
		}
	}
	if len(lines) != 4 {
		t.Fatalf("got %d line rows, want 4", len(lines))
	}
	if lines[0].Left.Text != "same" || lines[0].Right.Text != "same" {
		t.Errorf("context row = %+v", lines[0])
	}
	if lines[1].Left == nil || lines[1].Right == nil {
		t.Fatalf("first pair should have both sides: %+v", lines[1])
	}
	if lines[1].Left.Text != "old one" || lines[1].Right.Text != "new one" {
		t.Errorf("pair 1 = %q / %q", lines[1].Left.Text, lines[1].Right.Text)
	}
	if lines[3].Left != nil {
		t.Errorf("the extra addition should have no left side")
	}
	if lines[3].Right == nil || lines[3].Right.Text != "new three" {
		t.Errorf("extra addition = %+v", lines[3].Right)
	}
}

func TestNewModeDropsDeletions(t *testing.T) {
	cs := changeset(
		patch.Line{Kind: patch.Context, Text: "keep", OldNo: 1, NewNo: 1},
		patch.Line{Kind: patch.Del, Text: "gone", OldNo: 2},
		patch.Line{Kind: patch.Add, Text: "added", NewNo: 2},
	)
	for _, row := range Build(cs, New) {
		if row.Kind == LineRow && row.Sign == '-' {
			t.Fatalf("new mode kept a deletion: %+v", row)
		}
	}
}

func TestInlineMarksOnlyChangedWords(t *testing.T) {
	a, b := "return x + 1", "return x + 2"
	as, bs := inline(a, b)

	if len(as) != 1 || a[as[0].Start:as[0].End] != "1" {
		t.Fatalf("left spans = %v", as)
	}
	if len(bs) != 1 || b[bs[0].Start:bs[0].End] != "2" {
		t.Fatalf("right spans = %v", bs)
	}
}

func TestInlineGivesUpOnUnrelatedLines(t *testing.T) {
	if as, bs := inline("a completely different line", "zzz"); as != nil || bs != nil {
		t.Fatalf("expected no emphasis, got %v / %v", as, bs)
	}
}

func TestExpandTabs(t *testing.T) {
	if got := Expand("\tx\t"); got != "    x    " {
		t.Fatalf("Expand = %q", got)
	}
	if got := Expand("plain"); got != "plain" {
		t.Fatalf("Expand = %q", got)
	}
}

func TestHunkHeader(t *testing.T) {
	h := patch.Hunk{OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4, Section: "func main()"}
	if got := HunkHeader(&h); got != "@@ -1,3 +1,4 @@ func main()" {
		t.Fatalf("HunkHeader = %q", got)
	}
}
