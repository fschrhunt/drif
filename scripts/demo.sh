#!/bin/sh
# Builds a repository for looking at drif.
#
#   scripts/demo.sh            a realistic branch: a feature, tests, docs, a
#                              rename, a deletion and a one-line fix
#   scripts/demo.sh --edges    the awkward shapes that break diff viewers:
#                              binary, symlink, CRLF, unicode, mode-only
#
# Default directory: /tmp/drif-demo.
set -eu

mode=real
dir=${TMPDIR:-/tmp}/drif-demo
while [ $# -gt 0 ]; do
  case $1 in
    --edges) mode=edges ;;
    --dir=*) dir=${1#--dir=} ;;
    -h|--help) sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "usage: scripts/demo.sh [--edges] [--dir=DIR]" >&2; exit 2 ;;
  esac
  shift
done

rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"
git init -q -b main
git config user.email demo@example.com
git config user.name demo

if [ "$mode" = edges ]; then
  # ---------------------------------------------------------------- edges --
  mkdir -p src "docs with space" vendor
  cat > src/main.go <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("hello")
}
EOF
  printf 'one\ntwo\nthree\n' > src/delete-me.sh
  printf '#!/bin/sh\necho hi\n' > src/script.sh
  printf 'a\n' > "docs with space/notes.md"
  printf 'crlf\r\nlines\r\n' > src/crlf.txt
  printf '' > src/empty.txt
  ln -s main.go src/link.go
  printf '\211PNG\r\n\032\n\000\000\000\rIHDR' > src/logo.png
  printf 'package main\n\nfunc helper() {}\n' > src/helper.go
  git add -A && git commit -qm "initial"

  git mv src/helper.go src/renamed-helper.go
  sed -i 's/func helper() {}/func helper() { \/\/ renamed and edited }/' src/renamed-helper.go
  sed -i 's/fmt.Println("hello")/fmt.Println("hello, world")/' src/main.go
  git rm -q src/delete-me.sh
  chmod +x src/script.sh
  ln -sfn main.go src/link.go
  printf 'naïve café\n' > "docs with space/naïve café.txt"
  printf 'a\nb\n' > "docs with space/notes.md"
  printf 'crlf\r\nlines\r\nmore\r\n' > src/crlf.txt
  printf '\211PNG\r\n\032\n\000\000\000\rIHDR\000\000' > src/logo.png
  printf 'new\n' > src/empty.txt
  awk 'BEGIN { for (i = 0; i < 4000; i++) print "line " i }' > src/big.txt
  printf 'untracked\n' > src/untracked.txt
  printf '\000\001\002\003' > src/untracked.bin

  echo "built $dir (awkward shapes)"
  echo
  git status --short
  echo
  echo "now run:  cd $dir && drif"
  exit 0
fi

# ---------------------------------------------------------------- realistic --
# A small, plausible Go program. The changes below read like one branch of real
# work: a feature, its tests, a rename, a deletion, a doc update and a fix.
mkdir -p cmd/notes internal/store internal/render docs assets

cat > go.mod <<'EOF'
module example.com/notes

go 1.22
EOF

cat > cmd/notes/main.go <<'EOF'
// Command notes stores short notes in a JSON file.
package main

import (
	"fmt"
	"os"

	"example.com/notes/internal/render"
	"example.com/notes/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	s := store.New("notes.json")

	switch os.Args[1] {
	case "list":
		items, err := s.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(render.Format(items))
	case "add":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		if err := s.Add(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage: notes add <text> | notes list")
}
EOF

cat > internal/store/store.go <<'EOF'
// Package store keeps notes in a single JSON file.
package store

import (
	"encoding/json"
	"errors"
	"os"
)

// ErrNotFound is returned when a note does not exist.
var ErrNotFound = errors.New("note not found")

// Store reads and writes notes from a JSON file.
type Store struct {
	path string
}

// New returns a store backed by path.
func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) load() ([]string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []string
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) save(items []string) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Add appends one note.
func (s *Store) Add(text string) error {
	items, err := s.load()
	if err != nil {
		return err
	}
	return s.save(append(items, text))
}

// List returns every note.
func (s *Store) List() ([]string, error) {
	return s.load()
}

// Page returns at most n notes starting at offset.
func (s *Store) Page(offset, n int) ([]string, error) {
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	end := offset + n
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], nil
}
EOF

cat > internal/store/legacy.go <<'EOF'
package store

// LoadAll is the old entry point, kept for callers that have not migrated.
func LoadAll(path string) ([]string, error) {
	return New(path).List()
}
EOF

cat > internal/store/store_test.go <<'EOF'
package store

import (
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "notes.json"))
}

func TestAddAndList(t *testing.T) {
	s := newStore(t)
	if err := s.Add("first"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("second"); err != nil {
		t.Fatal(err)
	}
	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[1] != "second" {
		t.Fatalf("items = %v", items)
	}
}
EOF

cat > internal/render/output.go <<'EOF'
// Package render turns notes into terminal text.
package render

import (
	"fmt"
	"strings"
)

// Format numbers each note and puts one on every line.
func Format(items []string) string {
	if len(items) == 0 {
		return "no notes\n"
	}
	var b strings.Builder
	for i, item := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, item)
	}
	return b.String()
}
EOF

cat > README.md <<'EOF'
# notes

A small note store, used to demonstrate a diff viewer.

```
go run ./cmd/notes add "buy milk"
go run ./cmd/notes list
```
EOF

cat > docs/usage.md <<'EOF'
# Usage

Notes are stored in `notes.json` next to the working directory.

- `notes add <text>` appends a note.
- `notes list` prints every note, numbered.
EOF

printf '\211PNG\r\n\032\n\000\000\000\rIHDR\000\000\000\001' > assets/logo.png

git add -A
git commit -qm "Add the notes command"

# --- one branch of work ------------------------------------------------------
# A feature with its tests and docs, a rename, a dead file removed, and a
# one-line fix. This is what a review actually looks like.
python3 - <<'PY'
from pathlib import Path

p = Path("internal/store/store.go")
s = p.read_text()
s = s.replace('''// Page returns at most n notes starting at offset.
func (s *Store) Page(offset, n int) ([]string, error) {
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	end := offset + n''', '''// Delete removes the note at index.
func (s *Store) Delete(index int) error {
	items, err := s.load()
	if err != nil {
		return err
	}
	if index < 0 || index >= len(items) {
		return ErrNotFound
	}
	return s.save(append(items[:index], items[index+1:]...))
}

// Page returns at most n notes starting at offset.
func (s *Store) Page(offset, n int) ([]string, error) {
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	if offset < 0 || n < 0 {
		return nil, ErrNotFound
	}
	if offset > len(items) {
		return nil, nil
	}
	end := offset + n''')
p.write_text(s)

p = Path("cmd/notes/main.go")
s = p.read_text()
s = s.replace('''	case "add":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		if err := s.Add(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:''', '''	case "add":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		if err := s.Add(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "delete":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		var index int
		if _, err := fmt.Sscanf(os.Args[2], "%d", &index); err != nil {
			fmt.Fprintln(os.Stderr, "delete needs a number")
			os.Exit(2)
		}
		if err := s.Delete(index - 1); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:''')
s = s.replace('fmt.Println("usage: notes add <text> | notes list")',
              'fmt.Println("usage: notes add <text> | list | delete <number>")')
p.write_text(s)

p = Path("internal/store/store_test.go")
s = p.read_text()
s += '''
func TestDelete(t *testing.T) {
	s := newStore(t)
	for _, text := range []string{"a", "b", "c"} {
		if err := s.Add(text); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(1); err != nil {
		t.Fatal(err)
	}
	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0] != "a" || items[1] != "c" {
		t.Fatalf("items = %v", items)
	}
}

func TestDeleteOutOfRange(t *testing.T) {
	s := newStore(t)
	if err := s.Delete(0); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
'''
p.write_text(s)

p = Path("README.md")
s = p.read_text()
s = s.replace('''go run ./cmd/notes add "buy milk"
go run ./cmd/notes list
```''', '''go run ./cmd/notes add "buy milk"
go run ./cmd/notes list
go run ./cmd/notes delete 1
```''')
s = s.replace("A small note store, used to demonstrate a diff viewer.",
              "A small note store, used to demonstrate a diff viewer. Notes are kept in `notes.json`.")
p.write_text(s)

p = Path("docs/usage.md")
s = p.read_text()
s = s.replace("- `notes list` prints every note, numbered.",
              "- `notes list` prints every note, numbered.\n- `notes delete <n>` removes one by its number.")
p.write_text(s)
PY

# Rename a file with small edits, so git pairs it as a rename rather than a
# delete and an add.
git mv internal/render/output.go internal/render/table.go
python3 - <<'PY'
from pathlib import Path
p = Path("internal/render/table.go")
s = p.read_text()
s = s.replace("// Format numbers each note and puts one on every line.",
              "// Format numbers each note and puts one on every line.\n// Empty input is not an error; it renders as a single line.")
s = s.replace('return "no notes\\n"', 'return "(no notes)\\n"')
p.write_text(s)
PY

# Dead code, removed in the same branch.
git rm -q internal/store/legacy.go

printf '\211PNG\r\n\032\n\000\000\000\rIHDR\000\000\000\001\000\000\000\001' > assets/logo.png

echo "built $dir (a realistic branch)"
echo
git status --short
echo
cat <<'EOF'
What to look at:

  cmd/notes/main.go                      the delete subcommand
  internal/store/store.go                Delete, plus bounds checks in Page
  internal/store/store_test.go           two new tests
  internal/render/table.go               renamed from output.go, with an edit
  internal/store/legacy.go               deleted
  docs/usage.md, README.md               the docs that go with it
  assets/logo.png                        a binary that changed

now run:  cd DIR && drif
EOF
