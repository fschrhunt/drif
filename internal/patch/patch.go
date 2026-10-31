// Package patch parses git's unified diff output into a change set the
// renderers walk. The parser is deliberately lenient: a line it does not
// understand is ignored rather than failing the whole patch, because drift
// renders whatever git hands it, including patches produced by other tools.
package patch

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Status is what happened to a file between the base revision and the working
// tree.
type Status uint8

const (
	Modified Status = iota
	Added
	Deleted
	Renamed
	Copied
	TypeChanged
	Unmerged
)

// Letter is the one-character status used by --list and the file rail.
func (s Status) Letter() byte {
	switch s {
	case Added:
		return 'A'
	case Deleted:
		return 'D'
	case Renamed:
		return 'R'
	case Copied:
		return 'C'
	case TypeChanged:
		return 'T'
	case Unmerged:
		return 'U'
	}
	return 'M'
}

// String names the status for --json.
func (s Status) String() string {
	switch s {
	case Added:
		return "added"
	case Deleted:
		return "deleted"
	case Renamed:
		return "renamed"
	case Copied:
		return "copied"
	case TypeChanged:
		return "type-changed"
	case Unmerged:
		return "unmerged"
	}
	return "modified"
}

// LineKind distinguishes the lines of a hunk.
type LineKind uint8

const (
	Context LineKind = iota
	Add
	Del
	NoNewline
)

// Line is one line of a hunk. Text never includes the leading sign or the
// trailing newline; NoNewline lines have no text at all.
type Line struct {
	Kind  LineKind
	Text  string
	OldNo int
	NewNo int
}

// Hunk is one contiguous block of changes.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Section            string
	Lines              []Line
}

// File is one changed path.
type File struct {
	OldPath, NewPath string
	Status           Status
	OldMode, NewMode string
	Binary           bool
	Additions        int
	Deletions        int
	Hunks            []Hunk
	// Note carries a short explanation for a file whose contents are not
	// shown, such as a large untracked file.
	Note string
}

// Path is the path the file has after the change, or before it for deletions.
func (f *File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// ChangeSet is a parsed patch with the revisions it describes filled in by the
// caller. MergeBase is the commit old-side content is compared against.
type ChangeSet struct {
	Base, MergeBase, Head, Scope string
	Files                        []*File
}

// Parse reads a unified diff. It never reports malformed input as an error;
// only a failing reader stops it.
func Parse(r io.Reader) ([]*File, error) {
	p := &parser{br: bufio.NewReaderSize(r, 1<<20)}
	return p.run()
}

type parser struct {
	br    *bufio.Reader
	files []*File
	cur   *File
	hunk  *Hunk

	oldNo, newNo     int
	oldLeft, newLeft int
	inHunk           bool
	skipBinary       bool
}

func (p *parser) run() ([]*File, error) {
	for {
		line, err := p.readLine()
		if line == "" && err != nil {
			break
		}
		p.line(line)
		if err != nil {
			break
		}
	}
	p.flush()
	return p.files, nil
}

func (p *parser) readLine() (string, error) {
	s, err := p.br.ReadString('\n')
	s = strings.TrimSuffix(s, "\n")
	// Patches produced with CRLF content would otherwise leave a stray
	// carriage return that moves the terminal cursor mid-line.
	s = strings.TrimSuffix(s, "\r")
	// Git colours its output when it thinks it is writing to a terminal, which
	// is exactly what happens when drift is configured as core.pager.
	if strings.IndexByte(s, 0x1b) >= 0 {
		s = stripANSI(s)
	}
	return s, err
}

// stripANSI removes CSI escape sequences such as colour changes.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		if s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= '@' && s[i] <= '~') {
				i++
			}
			continue
		}
		i++
	}
	return b.String()
}

func (p *parser) line(line string) {
	if p.inHunk {
		if p.hunkLine(line) {
			return
		}
		p.endHunk()
	}
	if p.skipBinary {
		if strings.HasPrefix(line, "diff --git ") {
			p.skipBinary = false
		} else {
			return
		}
	}
	p.header(line)
}

// hunkLine consumes a line if it belongs to the current hunk.
func (p *parser) hunkLine(line string) bool {
	if strings.HasPrefix(line, `\`) {
		p.hunk.Lines = append(p.hunk.Lines, Line{Kind: NoNewline})
		return true
	}
	if line == "" {
		if p.oldLeft > 0 && p.newLeft > 0 {
			p.append(Context, "")
			return true
		}
		return false
	}
	switch line[0] {
	case ' ':
		if p.oldLeft > 0 && p.newLeft > 0 {
			p.append(Context, line[1:])
			return true
		}
	case '-':
		if p.oldLeft > 0 {
			p.append(Del, line[1:])
			return true
		}
	case '+':
		if p.newLeft > 0 {
			p.append(Add, line[1:])
			return true
		}
	}
	return false
}

func (p *parser) append(kind LineKind, text string) {
	l := Line{Kind: kind, Text: text}
	switch kind {
	case Context:
		l.OldNo, l.NewNo = p.oldNo, p.newNo
		p.oldNo++
		p.newNo++
		p.oldLeft--
		p.newLeft--
	case Del:
		l.OldNo = p.oldNo
		p.oldNo++
		p.oldLeft--
		p.cur.Deletions++
	case Add:
		l.NewNo = p.newNo
		p.newNo++
		p.newLeft--
		p.cur.Additions++
	}
	p.hunk.Lines = append(p.hunk.Lines, l)
}

func (p *parser) endHunk() {
	if p.hunk != nil {
		p.cur.Hunks = append(p.cur.Hunks, *p.hunk)
		p.hunk = nil
	}
	p.inHunk = false
}

func (p *parser) flush() {
	p.endHunk()
	if p.cur != nil {
		// Git reports a file becoming a symlink (or a submodule) as a type
		// change; a permission change is still an ordinary modification with a
		// note attached.
		if p.cur.Status == Modified &&
			len(p.cur.OldMode) == 6 && len(p.cur.NewMode) == 6 &&
			p.cur.OldMode[:3] != p.cur.NewMode[:3] {
			p.cur.Status = TypeChanged
		}
		// Without mode lines, as in `diff -u`, the /dev/null side is the only
		// signal that a file appeared or vanished.
		switch {
		case p.cur.OldPath == "" && p.cur.NewPath != "":
			p.cur.Status = Added
		case p.cur.NewPath == "" && p.cur.OldPath != "":
			p.cur.Status = Deleted
		}
		p.files = append(p.files, p.cur)
		p.cur = nil
	}
}

func (p *parser) header(line string) {
	// A patch from plain `diff -u` has no "diff --git" line, so its "---" is
	// what starts a file. A second one after a file that already has hunks
	// starts the next file.
	if strings.HasPrefix(line, "--- ") && (p.cur == nil || len(p.cur.Hunks) > 0) {
		p.flush()
		p.cur = &File{}
	}

	switch {
	case strings.HasPrefix(line, "diff --git "):
		p.flush()
		p.cur = &File{}
		p.cur.OldPath, p.cur.NewPath = splitDiffHeader(line[len("diff --git "):])

	case p.cur == nil:
		// Preamble: commit headers, "diff --cc", conflict diffs and anything
		// else we do not model. Ignore it rather than guessing.

	case strings.HasPrefix(line, "old mode "):
		p.cur.OldMode = strings.TrimSpace(line[len("old mode "):])

	case strings.HasPrefix(line, "new mode "):
		p.cur.NewMode = strings.TrimSpace(line[len("new mode "):])

	case strings.HasPrefix(line, "new file mode "):
		p.cur.Status = Added
		p.cur.NewMode = strings.TrimSpace(line[len("new file mode "):])

	case strings.HasPrefix(line, "deleted file mode "):
		p.cur.Status = Deleted
		p.cur.OldMode = strings.TrimSpace(line[len("deleted file mode "):])

	case strings.HasPrefix(line, "rename from "):
		p.cur.Status = Renamed
		p.cur.OldPath = unquote(strings.TrimPrefix(line[len("rename from "):], "a/"))

	case strings.HasPrefix(line, "rename to "):
		p.cur.Status = Renamed
		p.cur.NewPath = unquote(strings.TrimPrefix(line[len("rename to "):], "b/"))

	case strings.HasPrefix(line, "copy from "):
		p.cur.Status = Copied
		p.cur.OldPath = unquote(strings.TrimPrefix(line[len("copy from "):], "a/"))

	case strings.HasPrefix(line, "copy to "):
		p.cur.Status = Copied
		p.cur.NewPath = unquote(strings.TrimPrefix(line[len("copy to "):], "b/"))

	case strings.HasPrefix(line, "Binary files "):
		p.cur.Binary = true

	case line == "GIT binary patch":
		p.cur.Binary = true
		p.skipBinary = true

	case strings.HasPrefix(line, "--- "):
		// /dev/null decodes to "", which is meaningful: it clears the path the
		// diff header guessed, and that is how a deleted file stops claiming
		// to have a new name.
		p.cur.OldPath = diffPath(line[len("--- "):])

	case strings.HasPrefix(line, "+++ "):
		p.cur.NewPath = diffPath(line[len("+++ "):])

	case strings.HasPrefix(line, "@@ "):
		if h, ok := parseHunkHeader(line); ok {
			p.hunk = &h
			p.oldNo, p.newNo = h.OldStart, h.NewStart
			p.oldLeft, p.newLeft = h.OldCount, h.NewCount
			p.inHunk = true
		}
	}
}

// splitDiffHeader recovers the two paths from the text after "diff --git ".
// Git quotes paths that need escaping but leaves spaces literal, so a plain
// split on " b/" is ambiguous; the "a/x b/x" symmetry resolves the common case
// and the separator search covers renames.
func splitDiffHeader(rest string) (string, string) {
	if strings.HasPrefix(rest, `"`) {
		a, tail := readQuoted(rest)
		b, _ := readQuoted(strings.TrimLeft(tail, " "))
		return stripPrefix(a), stripPrefix(b)
	}
	if n := len(rest); n%2 == 1 && rest[n/2] == ' ' {
		left, right := rest[:n/2], rest[n/2+1:]
		if strings.HasPrefix(left, "a/") && strings.HasPrefix(right, "b/") && left[2:] == right[2:] {
			return left[2:], right[2:]
		}
	}
	if i := strings.Index(rest, " b/"); i >= 0 && strings.HasPrefix(rest, "a/") {
		return rest[2:i], rest[i+3:]
	}
	return "", ""
}

// diffPath normalises the path after "--- " or "+++ ".
func diffPath(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 && !strings.HasPrefix(s, `"`) {
		s = s[:i] // some tools append a timestamp after a tab
	}
	if s == "/dev/null" {
		return ""
	}
	return stripPrefix(unquote(s))
}

func stripPrefix(p string) string {
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

// readQuoted decodes one C-style quoted string and returns it with the text
// that follows it.
func readQuoted(s string) (string, string) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			return b.String(), s[i+1:]
		case '\\':
			if i+1 >= len(s) {
				return b.String(), ""
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'a':
				b.WriteByte(7)
			case 'b':
				b.WriteByte(8)
			case 'f':
				b.WriteByte(12)
			case 'v':
				b.WriteByte(11)
			case '0', '1', '2', '3', '4', '5', '6', '7':
				v := int(s[i] - '0')
				for k := 0; k < 2 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7'; k++ {
					i++
					v = v*8 + int(s[i]-'0')
				}
				b.WriteByte(byte(v))
			default:
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), ""
}

func unquote(s string) string {
	if strings.HasPrefix(s, `"`) {
		v, _ := readQuoted(s)
		return v
	}
	return s
}

// parseHunkHeader reads "@@ -a,b +c,d @@ section". A missing count means one
// line, and the section is whatever git put after the closing marker.
func parseHunkHeader(s string) (Hunk, bool) {
	var h Hunk
	rest := strings.TrimPrefix(s, "@@ ")
	end := strings.Index(rest, " @@")
	if end < 0 {
		return h, false
	}
	ranges := strings.Fields(rest[:end])
	if len(ranges) != 2 {
		return h, false
	}
	var ok1, ok2 bool
	h.OldStart, h.OldCount, ok1 = parseRange(ranges[0], '-')
	h.NewStart, h.NewCount, ok2 = parseRange(ranges[1], '+')
	if !ok1 || !ok2 {
		return h, false
	}
	h.Section = strings.TrimPrefix(rest[end+3:], " ")
	return h, true
}

func parseRange(s string, sign byte) (start, count int, ok bool) {
	if len(s) < 2 || s[0] != sign {
		return 0, 0, false
	}
	s = s[1:]
	count = 1
	if i := strings.IndexByte(s, ','); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil {
			return 0, 0, false
		}
		count = n
		s = s[:i]
	}
	start, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, false
	}
	return start, count, true
}

// UntrackedFile builds the synthetic all-added entry for a file git does not
// track yet. content is the file's text with any NUL bytes in the first
// sniffed prefix signalling a binary file.
func UntrackedFile(path string, content []byte, binary bool) *File {
	f := &File{NewPath: path, Status: Added, Binary: binary}
	if binary {
		return f
	}
	text := strings.TrimSuffix(string(content), "\n")
	if text == "" {
		return f
	}
	lines := strings.Split(text, "\n")
	h := Hunk{NewStart: 1, NewCount: len(lines)}
	for i, line := range lines {
		h.Lines = append(h.Lines, Line{Kind: Add, Text: strings.TrimSuffix(line, "\r"), NewNo: i + 1})
	}
	f.Hunks = []Hunk{h}
	f.Additions = len(lines)
	return f
}
