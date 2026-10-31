package render

import (
	"strings"

	"github.com/fschrhunt/drift/internal/layout"
	"github.com/fschrhunt/drift/internal/patch"
)

// PlainLines renders a change set as uncoloured text: the shape an agent or a
// pipe wants. It reads the patch directly rather than the laid-out rows so tabs
// and line content stay byte-for-byte as git produced them, which is what makes
// the output greppable and comparable.
func PlainLines(cs *patch.ChangeSet, prefix string) []string {
	out := make([]string, 0, len(cs.Files)*8)
	for _, f := range cs.Files {
		out = append(out, plainFile(f, prefix))
		for _, h := range f.Hunks {
			out = append(out, " "+layout.HunkHeader(&h))
			for _, l := range h.Lines {
				switch l.Kind {
				case patch.Context:
					out = append(out, " "+l.Text)
				case patch.Add:
					out = append(out, "+"+l.Text)
				case patch.Del:
					out = append(out, "-"+l.Text)
				}
			}
		}
	}
	return out
}

func plainFile(f *patch.File, prefix string) string {
	var b strings.Builder
	b.WriteString(" ")
	b.WriteString(strings.TrimPrefix(f.Path(), prefix))
	if f.Status != patch.Modified {
		b.WriteString("  ")
		b.WriteByte(f.Status.Letter())
	}
	var counts []string
	if f.Additions > 0 {
		counts = append(counts, "+"+itoa(f.Additions))
	}
	if f.Deletions > 0 {
		counts = append(counts, "-"+itoa(f.Deletions))
	}
	if len(counts) > 0 {
		b.WriteString("  ")
		b.WriteString(strings.Join(counts, " "))
	}
	return b.String()
}

// ListLines renders one line per file: the cheap index an agent reads before
// asking for detail with --file.
func ListLines(cs *patch.ChangeSet, prefix string) []string {
	out := make([]string, 0, len(cs.Files))
	for _, f := range cs.Files {
		out = append(out, strings.Join([]string{
			string(f.Status.Letter()),
			strings.TrimPrefix(f.Path(), prefix),
			"+" + itoa(f.Additions),
			"-" + itoa(f.Deletions),
		}, "\t"))
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
