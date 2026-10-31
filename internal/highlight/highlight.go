// Package highlight produces syntax spans for whole files, then trims them to
// the lines a diff happens to show. Tokenising the file rather than the hunk
// is what keeps block comments and multi-line strings correct: the lexer sees
// the state that leads into the change instead of starting cold at the hunk.
package highlight

import (
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/fschrhunt/drift/internal/git"
	"github.com/fschrhunt/drift/internal/patch"
)

// Class is the coarse token category a renderer maps to a colour.
type Class uint8

const (
	Text Class = iota
	Keyword
	String
	Comment
	Number
	Function
	Type
	Constant
	Operator
	Punctuation
	Tag
	Attribute
	Error
)

// Span is a byte range of one line carrying one class.
type Span struct {
	Start, End int
	Class      Class
}

// Table holds the spans of every line of a file, indexed from zero.
type Table [][]Span

// cacheLimit bounds how many files are kept highlighted at once.
const cacheLimit = 128

// Cache lexes files on demand and remembers the result.
type Cache struct {
	repo *git.Repo
	base string // revision old-side content is read from

	mu     sync.Mutex
	tables map[string]Table
	gen    uint64
}

// New returns a cache that reads new-side content from the working tree and
// old-side content from base.
func New(repo *git.Repo, base string) *Cache {
	return &Cache{repo: repo, base: base, tables: map[string]Table{}}
}

// Reset forgets every table, which is what a reload after a file changes
// needs: the cache is keyed by path, so stale content would otherwise keep its
// old colours.
func (c *Cache) Reset(base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = base
	c.tables = map[string]Table{}
	c.gen++
}

// Key identifies one side of one file.
func Key(f *patch.File, old bool) string {
	if old {
		if f.OldPath != "" {
			return "old:" + f.OldPath
		}
		return "old:" + f.Path()
	}
	return "new:" + f.NewPath
}

// Ready reports whether the table is already cached.
func (c *Cache) Ready(f *patch.File, old bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.tables[Key(f, old)]
	return ok
}

// Table returns the syntax spans for one side of a file, loading it if needed.
// A nil table means "render plain", which is always safe.
func (c *Cache) Table(f *patch.File, old bool) Table {
	k := Key(f, old)
	c.mu.Lock()
	if t, ok := c.tables[k]; ok {
		c.mu.Unlock()
		return t
	}
	base, gen := c.base, c.gen
	c.mu.Unlock()

	var t Table
	if content, path, err := c.content(base, f, old); err == nil {
		t = tokenize(path, content)
	}

	c.mu.Lock()
	// A reload may have landed while we were lexing; that table describes
	// content nobody is looking at any more.
	if c.gen == gen {
		if len(c.tables) >= cacheLimit {
			c.tables = map[string]Table{}
		}
		c.tables[k] = t
	}
	c.mu.Unlock()
	return t
}

func (c *Cache) content(base string, f *patch.File, old bool) ([]byte, string, error) {
	if old {
		path := f.OldPath
		if path == "" {
			path = f.Path()
		}
		b, err := c.repo.Content(base, path)
		return b, path, err
	}
	b, err := c.repo.ReadWorkingTree(f.NewPath)
	return b, f.NewPath, err
}

func tokenize(path string, content []byte) Table {
	text := string(expandTabs(content))
	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		return nil
	}
	iter, err := chroma.Coalesce(lexer).Tokenise(nil, text)
	if err != nil {
		return nil
	}

	var table Table
	var row []Span
	col := 0
	for _, tok := range iter.Tokens() {
		class := classOf(tok.Type)
		val := tok.Value
		for {
			i := strings.IndexByte(val, '\n')
			if i < 0 {
				if val != "" {
					row = append(row, Span{Start: col, End: col + len(val), Class: class})
					col += len(val)
				}
				break
			}
			if i > 0 {
				row = append(row, Span{Start: col, End: col + i, Class: class})
			}
			table = append(table, row)
			row = nil
			col = 0
			val = val[i+1:]
		}
	}
	if len(row) > 0 {
		table = append(table, row)
	}
	return table
}

// classOf folds chroma's token hierarchy into the categories a terminal theme
// can distinguish.
func classOf(t chroma.TokenType) Class {
	switch {
	case t.InCategory(chroma.Error):
		return Error
	case t.InCategory(chroma.Comment):
		return Comment
	case t.InCategory(chroma.String), t.InCategory(chroma.LiteralString):
		return String
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t.InCategory(chroma.Number):
		return Number
	case t.InCategory(chroma.NameFunction):
		return Function
	case t.InCategory(chroma.NameClass):
		return Type
	case t.InCategory(chroma.NameConstant):
		return Constant
	case t.InCategory(chroma.NameTag):
		return Tag
	case t.InCategory(chroma.NameAttribute):
		return Attribute
	case t.InCategory(chroma.Operator):
		return Operator
	case t.InCategory(chroma.Punctuation):
		return Punctuation
	}
	return Text
}

// tabWidth is how many columns a tab occupies. Source that indents with tabs
// would otherwise jump to the terminal's tab stops after the gutter, which
// makes every indentation level look distorted.
const tabWidth = 4

// expandTabs replaces tabs with spaces. Line content and syntax spans both go
// through it, so byte offsets stay consistent.
func expandTabs(b []byte) []byte {
	if !strings.ContainsRune(string(b), '\t') {
		return b
	}
	out := make([]byte, 0, len(b)+len(b)/4)
	for _, c := range b {
		if c == '\t' {
			out = append(out, ' ', ' ', ' ', ' ')
			continue
		}
		out = append(out, c)
	}
	return out
}
