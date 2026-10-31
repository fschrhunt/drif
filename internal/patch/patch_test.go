package patch

import (
	"strings"
	"testing"
)

func parse(t *testing.T, s string) []*File {
	t.Helper()
	files, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return files
}

func TestParseModifiedFile(t *testing.T) {
	src := "diff --git a/main.go b/main.go\n" +
		"index 111..222 100644\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1,3 +1,4 @@ func main() {\n" +
		" package main\n" +
		"-old line\n" +
		"+new line\n" +
		"+added line\n" +
		" }\n"

	files := parse(t, src)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	f := files[0]
	if f.Path() != "main.go" || f.Status != Modified {
		t.Fatalf("path=%q status=%v", f.Path(), f.Status)
	}
	if f.Additions != 2 || f.Deletions != 1 {
		t.Fatalf("additions=%d deletions=%d", f.Additions, f.Deletions)
	}
	if len(f.Hunks) != 1 || f.Hunks[0].Section != "func main() {" {
		t.Fatalf("hunks=%+v", f.Hunks)
	}
	want := []Line{
		{Kind: Context, Text: "package main", OldNo: 1, NewNo: 1},
		{Kind: Del, Text: "old line", OldNo: 2},
		{Kind: Add, Text: "new line", NewNo: 2},
		{Kind: Add, Text: "added line", NewNo: 3},
		{Kind: Context, Text: "}", OldNo: 3, NewNo: 4},
	}
	got := f.Hunks[0].Lines
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseNewAndDeletedFiles(t *testing.T) {
	src := "diff --git a/new.txt b/new.txt\n" +
		"new file mode 100644\n" +
		"index 000..111\n" +
		"--- /dev/null\n" +
		"+++ b/new.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+one\n" +
		"+two\n" +
		"diff --git a/gone.txt b/gone.txt\n" +
		"deleted file mode 100644\n" +
		"--- a/gone.txt\n" +
		"+++ /dev/null\n" +
		"@@ -1,1 +0,0 @@\n" +
		"-bye\n"

	files := parse(t, src)
	if len(files) != 2 {
		t.Fatalf("got %d files", len(files))
	}
	if files[0].Status != Added || files[0].Path() != "new.txt" || files[0].Additions != 2 {
		t.Errorf("added file = %+v", files[0])
	}
	if files[1].Status != Deleted || files[1].Path() != "gone.txt" || files[1].Deletions != 1 {
		t.Errorf("deleted file = %+v", files[1])
	}
}

func TestParseRenameWithEdit(t *testing.T) {
	src := "diff --git a/old/name.go b/new/name.go\n" +
		"similarity index 90%\n" +
		"rename from old/name.go\n" +
		"rename to new/name.go\n" +
		"index 111..222 100644\n" +
		"--- a/old/name.go\n" +
		"+++ b/new/name.go\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-a\n" +
		"+b\n"

	f := parse(t, src)[0]
	if f.Status != Renamed || f.OldPath != "old/name.go" || f.NewPath != "new/name.go" {
		t.Fatalf("got %+v", f)
	}
	if f.Additions != 1 || f.Deletions != 1 {
		t.Errorf("counts %d/%d", f.Additions, f.Deletions)
	}
}

func TestParseModeChangeOnly(t *testing.T) {
	src := "diff --git a/run.sh b/run.sh\n" +
		"old mode 100644\n" +
		"new mode 100755\n"

	f := parse(t, src)[0]
	if f.OldMode != "100644" || f.NewMode != "100755" {
		t.Fatalf("modes = %q %q", f.OldMode, f.NewMode)
	}
	if len(f.Hunks) != 0 {
		t.Errorf("unexpected hunks: %+v", f.Hunks)
	}
}

func TestParseBinaryAndGITBinaryPatch(t *testing.T) {
	src := "diff --git a/logo.png b/logo.png\n" +
		"index 111..222 100644\n" +
		"Binary files a/logo.png and b/logo.png differ\n" +
		"diff --git a/other.bin b/other.bin\n" +
		"index 333..444 100644\n" +
		"GIT binary patch\n" +
		"literal 12\n" +
		"TcmZ?wbhEHbRA2d0H0000\n" +
		"\n" +
		"literal 0\n" +
		"HcmV?d00001\n" +
		"diff --git a/after.txt b/after.txt\n" +
		"--- a/after.txt\n" +
		"+++ b/after.txt\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-x\n" +
		"+y\n"

	files := parse(t, src)
	if len(files) != 3 {
		t.Fatalf("got %d files", len(files))
	}
	if !files[0].Binary || !files[1].Binary {
		t.Errorf("binary flags = %v %v", files[0].Binary, files[1].Binary)
	}
	// The base85 payload must not leak into the next file.
	if files[2].Path() != "after.txt" || files[2].Additions != 1 {
		t.Errorf("file after binary patch = %+v", files[2])
	}
}

func TestParseNoNewlineMarker(t *testing.T) {
	src := "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-a\n" +
		"\\ No newline at end of file\n" +
		"+b\n" +
		"\\ No newline at end of file\n"

	f := parse(t, src)[0]
	kinds := []LineKind{}
	for _, l := range f.Hunks[0].Lines {
		kinds = append(kinds, l.Kind)
	}
	want := []LineKind{Del, NoNewline, Add, NoNewline}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
}

func TestParseQuotedAndSpacedPaths(t *testing.T) {
	src := "diff --git \"a/tab\\there.txt\" \"b/tab\\there.txt\"\n" +
		"--- \"a/tab\\there.txt\"\n" +
		"+++ \"b/tab\\there.txt\"\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n" +
		"diff --git a/with space.txt b/with space.txt\n" +
		"--- a/with space.txt\n" +
		"+++ b/with space.txt\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n"

	files := parse(t, src)
	if files[0].Path() != "tab\there.txt" {
		t.Errorf("quoted path = %q", files[0].Path())
	}
	if files[1].Path() != "with space.txt" {
		t.Errorf("spaced path = %q", files[1].Path())
	}
}

func TestParseLongLine(t *testing.T) {
	long := strings.Repeat("x", 200_000)
	src := "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+" + long + "\n"

	f := parse(t, src)[0]
	if got := f.Hunks[0].Lines[1].Text; len(got) != len(long) {
		t.Fatalf("long line truncated to %d", len(got))
	}
}

func TestParseEmptyContextLine(t *testing.T) {
	// Some tools emit a completely empty context line instead of a single
	// space; a hunk must not end there.
	src := "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
		"@@ -1,3 +1,3 @@\n" +
		" a\n" +
		"\n" +
		" b\n"

	f := parse(t, src)[0]
	if len(f.Hunks[0].Lines) != 3 {
		t.Fatalf("lines = %+v", f.Hunks[0].Lines)
	}
}

func TestParseFirstHunkAfterPreamble(t *testing.T) {
	src := "commit abc\nAuthor: A\n\n    message\n\n" +
		"diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
		"@@ -1 +1 @@\n-a\n+b\n"

	files := parse(t, src)
	if len(files) != 1 || files[0].Path() != "f" {
		t.Fatalf("files = %+v", files)
	}
}

func TestUntrackedFile(t *testing.T) {
	f := UntrackedFile("notes.txt", []byte("one\ntwo\n"), false)
	if f.Status != Added || f.Additions != 2 || f.Path() != "notes.txt" {
		t.Fatalf("got %+v", f)
	}
	if f.Hunks[0].Lines[1].NewNo != 2 {
		t.Errorf("line numbers = %+v", f.Hunks[0].Lines)
	}

	b := UntrackedFile("img.png", []byte{0x89, 0x50, 0x00}, true)
	if !b.Binary || b.Additions != 0 {
		t.Errorf("binary = %+v", b)
	}
}
