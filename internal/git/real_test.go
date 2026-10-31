package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/drift/internal/patch"
)

// These tests run real git and parse its real output. The parser has unit
// tests with hand-written patches; this file exists because the bugs live in
// the seam between the two, and every case here is one git has surprised a
// diff viewer with before.

func loadChanges(t *testing.T, repo *Repo) *patch.ChangeSet {
	t.Helper()
	cs, err := repo.Load(Options{Base: "main", Context: 3, Untracked: true})
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func find(t *testing.T, cs *patch.ChangeSet, path string) *patch.File {
	t.Helper()
	for _, f := range cs.Files {
		if f.Path() == path {
			return f
		}
	}
	t.Fatalf("no file %q in %v", path, paths(cs))
	return nil
}

func TestRealRenameIsOneFile(t *testing.T) {
	repo := newRepo(t)
	write(t, repo.Root, "original.go", strings.Repeat("// a line of the file\n", 40))
	runGit(t, repo.Root, "add", ".")
	runGit(t, repo.Root, "commit", "-qm", "add original")
	runGit(t, repo.Root, "mv", "original.go", "renamed.go")

	cs := loadChanges(t, repo)
	if len(cs.Files) != 1 {
		t.Fatalf("got %d files, want one rename: %v", len(cs.Files), paths(cs))
	}
	f := cs.Files[0]
	if f.Status != patch.Renamed || f.OldPath != "original.go" || f.NewPath != "renamed.go" {
		t.Fatalf("file = %+v", f)
	}
}

func TestRealBinaryIsFlaggedNotRendered(t *testing.T) {
	repo := newRepo(t)
	bin := func() []byte { return []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 1, 2, 3} }
	if err := os.WriteFile(filepath.Join(repo.Root, "logo.png"), bin(), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo.Root, "add", ".")
	runGit(t, repo.Root, "commit", "-qm", "add png")

	changed := append(bin(), 0xff)
	if err := os.WriteFile(filepath.Join(repo.Root, "logo.png"), changed, 0o644); err != nil {
		t.Fatal(err)
	}
	f := find(t, loadChanges(t, repo), "logo.png")
	if !f.Binary || len(f.Hunks) != 0 {
		t.Fatalf("file = %+v", f)
	}
}

func TestRealModeOnlyChange(t *testing.T) {
	repo := newRepo(t)
	write(t, repo.Root, "run.sh", "#!/bin/sh\necho hi\n")
	runGit(t, repo.Root, "add", ".")
	runGit(t, repo.Root, "commit", "-qm", "add script")
	if err := os.Chmod(filepath.Join(repo.Root, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}

	f := find(t, loadChanges(t, repo), "run.sh")
	if f.OldMode != "100644" || f.NewMode != "100755" {
		t.Fatalf("modes = %q -> %q", f.OldMode, f.NewMode)
	}
	if len(f.Hunks) != 0 || f.Status != patch.Modified {
		t.Fatalf("file = %+v", f)
	}
	if meta := strings.Join(metaLines(f), "|"); !strings.Contains(meta, "100755") {
		t.Fatalf("meta = %q", meta)
	}
}

func TestRealSymlinkTargetIsTheContent(t *testing.T) {
	repo := newRepo(t)
	write(t, repo.Root, "target-a", "a\n")
	write(t, repo.Root, "target-b", "b\n")
	if err := os.Symlink("target-a", filepath.Join(repo.Root, "link")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo.Root, "add", ".")
	runGit(t, repo.Root, "commit", "-qm", "add link")
	if err := os.Remove(filepath.Join(repo.Root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target-b", filepath.Join(repo.Root, "link")); err != nil {
		t.Fatal(err)
	}

	f := find(t, loadChanges(t, repo), "link")
	if len(f.Hunks) != 1 {
		t.Fatalf("hunks = %+v", f.Hunks)
	}
	var got []string
	for _, l := range f.Hunks[0].Lines {
		switch l.Kind {
		case patch.Add:
			got = append(got, "+"+l.Text)
		case patch.Del:
			got = append(got, "-"+l.Text)
		}
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "-target-a") || !strings.Contains(joined, "+target-b") {
		t.Fatalf("lines = %q", joined)
	}
}

func TestRealPathsWithSpacesAndUnicode(t *testing.T) {
	repo := newRepo(t)
	write(t, repo.Root, "docs with space/naïve café.txt", "one\n")
	runGit(t, repo.Root, "add", ".")
	runGit(t, repo.Root, "commit", "-qm", "add unicode path")
	write(t, repo.Root, "docs with space/naïve café.txt", "one\ntwo\n")

	cs := loadChanges(t, repo)
	f := find(t, cs, "docs with space/naïve café.txt")
	if f.Additions != 1 {
		t.Fatalf("file = %+v", f)
	}
}

func TestRealDeletedFile(t *testing.T) {
	repo := newRepo(t)
	runGit(t, repo.Root, "rm", "-q", "gone.txt")

	f := find(t, loadChanges(t, repo), "gone.txt")
	if f.Status != patch.Deleted || f.Deletions != 1 || f.NewPath != "" {
		t.Fatalf("file = %+v", f)
	}
}

func TestRealCrlfContent(t *testing.T) {
	repo := newRepo(t)
	write(t, repo.Root, "win.txt", "one\r\ntwo\r\n")
	runGit(t, repo.Root, "add", ".")
	runGit(t, repo.Root, "commit", "-qm", "crlf")
	write(t, repo.Root, "win.txt", "one\r\ntwo\r\nthree\r\n")

	f := find(t, loadChanges(t, repo), "win.txt")
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if strings.Contains(l.Text, "\r") {
				t.Fatalf("carriage return survived into %q", l.Text)
			}
		}
	}
	if f.Additions != 1 {
		t.Fatalf("additions = %d", f.Additions)
	}
}

func TestRealUnbornHead(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "test")
	write(t, dir, "first.txt", "hello\n")

	repo := &Repo{Root: dir}
	cs, err := repo.Load(Options{Context: 3, Untracked: true})
	if err != nil {
		t.Fatalf("a repository with no commits should still load: %v", err)
	}
	f := find(t, cs, "first.txt")
	if f.Status != patch.Added || f.Additions != 1 {
		t.Fatalf("file = %+v", f)
	}
}

// metaLines mirrors what the renderer shows for a file, so the assertion above
// is about the fact being available rather than about its formatting.
func metaLines(f *patch.File) []string {
	var out []string
	if f.OldMode != "" && f.NewMode != "" && f.OldMode != f.NewMode {
		out = append(out, f.OldMode+" "+f.NewMode)
	}
	if f.Binary {
		out = append(out, "binary")
	}
	return out
}
