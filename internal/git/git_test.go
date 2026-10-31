package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fschrhunt/drif/internal/patch"
)

// newRepo builds a throwaway repository with one commit on main.
func newRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "test")
	write(t, dir, "keep.txt", "one\ntwo\n")
	write(t, dir, "gone.txt", "bye\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "init")
	return &Repo{Root: dir}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadComparesMergeBaseToWorkingTree(t *testing.T) {
	repo := newRepo(t)
	runGit(t, repo.Root, "checkout", "-qb", "feature")
	write(t, repo.Root, "keep.txt", "one\nchanged\n")

	cs, err := repo.Load(Options{Base: "main", Context: 3, Untracked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(cs.Files), paths(cs))
	}
	f := cs.Files[0]
	if f.Path() != "keep.txt" || f.Additions != 1 || f.Deletions != 1 {
		t.Fatalf("file = %+v", f)
	}
}

func TestLoadIncludesUntrackedFiles(t *testing.T) {
	repo := newRepo(t)
	write(t, repo.Root, "notes.txt", "hello\n")

	cs, err := repo.Load(Options{Base: "main", Context: 3, Untracked: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Files) != 1 || cs.Files[0].Path() != "notes.txt" || cs.Files[0].Status != patch.Added {
		t.Fatalf("files = %+v", paths(cs))
	}

	cs, err = repo.Load(Options{Base: "main", Context: 3, Untracked: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Files) != 0 {
		t.Fatalf("untracked should be excluded: %+v", paths(cs))
	}
}

func TestLoadScopesToTheInvocationDirectory(t *testing.T) {
	repo := newRepo(t)
	write(t, repo.Root, "sub/a.txt", "a\n")
	runGit(t, repo.Root, "add", ".")
	runGit(t, repo.Root, "commit", "-qm", "add sub")
	runGit(t, repo.Root, "checkout", "-qb", "feature")
	write(t, repo.Root, "sub/a.txt", "changed\n")
	repo.Prefix = "sub/"

	cs, err := repo.Load(Options{Base: "main", Context: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs.Files) != 1 || cs.Files[0].Path() != "sub/a.txt" {
		t.Fatalf("files = %+v", paths(cs))
	}
	if got := repo.Display("sub/a.txt"); got != "a.txt" {
		t.Fatalf("Display = %q", got)
	}
}

func TestResolveBasePrefersOriginHead(t *testing.T) {
	repo := newRepo(t)
	got, err := repo.ResolveBase("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "main" {
		t.Fatalf("base = %q, want main", got)
	}
	if got, _ := repo.ResolveBase("HEAD~1"); got != "HEAD~1" {
		t.Fatalf("explicit base ignored: %q", got)
	}
}

// paths lists a change set's file paths for failure messages.
func paths(cs *patch.ChangeSet) []string {
	out := make([]string, 0, len(cs.Files))
	for _, f := range cs.Files {
		out = append(out, f.Path())
	}
	return out
}
