// Package git answers everything drif needs to know about a repository.
// Every question goes to git itself: drif parses patches, it never
// re-implements history, rename detection or whitespace rules.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fschrhunt/drif/internal/patch"
)

// maxUntrackedSize bounds what drif is willing to read from a file git does
// not track yet. Beyond it the file is reported but not shown.
const maxUntrackedSize = 2 << 20

// maxBinarySniff is how much of a file is examined for NUL bytes when deciding
// whether it is binary.
const maxBinarySniff = 8 << 10

// Repo is an open repository plus the directory drif was invoked from.
type Repo struct {
	Root   string // absolute path to the working tree root
	Prefix string // path from the root to the invocation directory; "" at root
}

// Discover locates the repository containing the working directory.
func Discover() (*Repo, error) {
	root, err := git("", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	root = strings.TrimSpace(root)
	// Ask from the invocation directory, not the root, or the prefix is always
	// empty.
	prefix, err := git("", "rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	return &Repo{Root: root, Prefix: strings.TrimSpace(prefix)}, nil
}

// ResolveBase picks the revision to compare against. An explicit flag wins,
// then the environment, then git config, then the conventional branch names.
// Worktrees are usually detached, so origin/HEAD is the reliable default.
func (r *Repo) ResolveBase(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if v := os.Getenv("DRIF_BASE"); v != "" {
		return v, nil
	}
	if v, err := git(r.Root, "config", "--get", "drif.base"); err == nil {
		if v = strings.TrimSpace(v); v != "" {
			return v, nil
		}
	}
	for _, ref := range []string{"refs/remotes/origin/HEAD", "origin/main", "origin/master", "main", "master"} {
		if r.hasRef(ref) {
			return ref, nil
		}
	}
	return "", errors.New("cannot find a base branch; pass --base <ref>")
}

func (r *Repo) hasRef(ref string) bool {
	_, err := git(r.Root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// Head is the abbreviated commit drif is looking at.
func (r *Repo) Head() string {
	out, err := git(r.Root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Options controls how much of the working tree is loaded.
type Options struct {
	Base      string // revision to compare against
	Context   int    // lines of context in each hunk
	Untracked bool   // include files git does not track
}

// Load produces the change set for the whole working tree, comparing the merge
// base of base and HEAD against what is on disk. Comparing the merge base
// rather than the branch tip keeps changes that landed on the base branch
// after branching out of the picture.
func (r *Repo) Load(opts Options) (*patch.ChangeSet, error) {
	base := opts.Base
	var mb string
	if r.hasRef("HEAD") {
		if base == "" {
			var err error
			if base, err = r.ResolveBase(""); err != nil {
				return nil, err
			}
		}
		var err error
		if mb, err = r.mergeBase(base); err != nil {
			return nil, err
		}
	} else {
		// A repository with no commits has no base branch to resolve. Compare
		// the empty tree instead, so a project's very first files show up
		// rather than an error about branches.
		var err error
		if mb, err = r.emptyTree(); err != nil {
			return nil, err
		}
	}

	raw, err := gitOut(r.Root, "-c", "core.quotepath=false", "diff",
		"--no-color", "--no-ext-diff", "--no-textconv", "-M",
		fmt.Sprintf("--unified=%d", opts.Context), mb, "--", r.scope())
	if err != nil {
		return nil, err
	}
	files, err := patch.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if opts.Untracked {
		files = append(files, r.untracked()...)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path() < files[j].Path() })

	return &patch.ChangeSet{
		Base:      base,
		MergeBase: mb,
		Head:      r.Head(),
		Scope:     r.Prefix,
		Files:     files,
	}, nil
}

// emptyTree writes and returns the empty tree object of this repository's hash
// algorithm, which is the correct "before" side for an unborn branch.
func (r *Repo) emptyTree() (string, error) {
	out, err := git(r.Root, "hash-object", "-w", "-t", "tree", os.DevNull)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// MergeBase is the fork point drif compares the working tree against. It is
// exported so `drif config` can show where the diff actually comes from.
func (r *Repo) MergeBase(base string) (string, error) { return r.mergeBase(base) }

func (r *Repo) mergeBase(base string) (string, error) {
	mb, err := git(r.Root, "merge-base", base, "HEAD")
	if err != nil {
		// Unrelated histories or a bare ref: compare against the ref itself,
		// which is what the user asked for by naming it.
		if _, verifyErr := git(r.Root, "rev-parse", "--verify", "--quiet", base+"^{commit}"); verifyErr == nil {
			return base, nil
		}
		return "", err
	}
	return strings.TrimSpace(mb), nil
}

// untracked synthesizes an all-added file for everything git does not track.
// Reading the files directly is one process instead of one git call per path.
func (r *Repo) untracked() []*patch.File {
	out, err := git(r.Root, "ls-files", "--others", "--exclude-standard", "-z", "--", r.scope())
	if err != nil || out == "" {
		return nil
	}
	var files []*patch.File
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		full := filepath.Join(r.Root, rel)
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Size() > maxUntrackedSize {
			files = append(files, &patch.File{
				NewPath: rel,
				Status:  patch.Added,
				Note:    "large file, not shown",
			})
			continue
		}
		content, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		files = append(files, patch.UntrackedFile(rel, content, isBinary(content)))
	}
	return files
}

func isBinary(content []byte) bool {
	if len(content) > maxBinarySniff {
		content = content[:maxBinarySniff]
	}
	return bytes.IndexByte(content, 0) >= 0
}

// ReadWorkingTree returns the file's current contents.
func (r *Repo) ReadWorkingTree(path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.Root, path))
}

// scope is the pathspec that limits git to the directory drif was run in.
// Commands run with the repository root as their working directory, so a bare
// "." would mean the whole tree.
func (r *Repo) scope() string {
	if r.Prefix == "" {
		return "."
	}
	return r.Prefix
}

// Display shortens a root-relative path to one relative to the invocation
// directory, which is what the user typed and what reads best.
func (r *Repo) Display(path string) string {
	return strings.TrimPrefix(path, r.Prefix)
}

func git(dir string, args ...string) (string, error) {
	out, err := gitOut(dir, args...)
	return string(out), err
}

func gitOut(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}
