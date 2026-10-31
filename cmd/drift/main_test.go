package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo builds a small repository and switches the test into it, so run sees
// the same working directory the user would.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-qm", "init")
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("one\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func stdinFrom(t *testing.T, content string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return f
}

func invoke(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("DRIFT_CONFIG_DIR", t.TempDir())
	t.Setenv("DRIFT_CONFIG", "")
	var out, errOut bytes.Buffer
	code := run(args, stdinFrom(t, stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestListIsTabSeparated(t *testing.T) {
	newRepo(t)
	code, out, errOut := invoke(t, "", "--list")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "M\tkeep.txt\t+1\t-1\n" {
		t.Fatalf("list = %q", out)
	}
}

func TestJSONIsVersionedAndFaithful(t *testing.T) {
	newRepo(t)
	code, out, errOut := invoke(t, "", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var doc struct {
		Version int    `json:"version"`
		Base    string `json:"base"`
		Files   []struct {
			Path      string `json:"path"`
			Additions int    `json:"additions"`
			Hunks     []struct {
				Lines []struct {
					Text string `json:"text"`
				} `json:"lines"`
			} `json:"hunks"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if doc.Version != 1 || doc.Base != "main" || len(doc.Files) != 1 {
		t.Fatalf("doc = %+v", doc)
	}
	var added []string
	for _, l := range doc.Files[0].Hunks[0].Lines {
		if l.Text == "changed" {
			added = append(added, l.Text)
		}
	}
	if len(added) != 1 {
		t.Fatalf("changed line missing from %+v", doc.Files[0].Hunks[0].Lines)
	}
}

func TestPlainOutputIsUncoloured(t *testing.T) {
	newRepo(t)
	_, out, _ := invoke(t, "", "--color", "never")
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain output contains escapes: %q", out)
	}
	if !strings.Contains(out, "+changed") {
		t.Fatalf("output = %q", out)
	}
}

func TestExitCode(t *testing.T) {
	dir := newRepo(t)
	if code, _, _ := invoke(t, "", "--exit-code", "--list"); code != 1 {
		t.Fatalf("dirty tree exit = %d, want 1", code)
	}
	if out, err := exec.Command("git", "-C", dir, "checkout", "--", ".").CombinedOutput(); err != nil {
		t.Fatalf("checkout: %v: %s", err, out)
	}
	if code, _, _ := invoke(t, "", "--exit-code", "--list"); code != 0 {
		t.Fatalf("clean tree exit = %d, want 0", code)
	}
}

func TestFlagBeatsConfig(t *testing.T) {
	newRepo(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("base = \"does-not-exist\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := invoke(t, "", "--config", cfg, "--list"); code != 2 {
		t.Fatalf("bad configured base should fail, exit = %d: %s", code, errOut)
	}
	if code, _, errOut := invoke(t, "", "--config", cfg, "--base", "main", "--list"); code != 0 {
		t.Fatalf("--base should win, exit = %d: %s", code, errOut)
	}
}

func TestFileFilter(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, out, _ := invoke(t, "", "--list", "--file", "other.txt")
	if out != "A\tother.txt\t+1\t-0\n" {
		t.Fatalf("list = %q", out)
	}
}

func TestPipedPatchIsRendered(t *testing.T) {
	t.Chdir(t.TempDir()) // no repository in sight
	patchText := "--- old.txt\n+++ new.txt\n@@ -1,1 +1,2 @@\n same\n+added\n"
	code, out, errOut := invoke(t, patchText, "--color", "never")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "+added") || !strings.Contains(out, "new.txt") {
		t.Fatalf("output = %q", out)
	}
}

func TestHelpAndBadFlag(t *testing.T) {
	newRepo(t)
	if code, _, _ := invoke(t, "", "--help"); code != 0 {
		t.Fatalf("--help exit = %d", code)
	}
	if code, _, _ := invoke(t, "", "--nonsense"); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
}

func TestOutsideRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	code, _, errOut := invoke(t, "")
	if code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errOut, "not a git repository") {
		t.Fatalf("stderr = %q", errOut)
	}
}
