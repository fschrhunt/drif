package render

import (
	"encoding/json"

	"github.com/fschrhunt/drif/internal/patch"
)

// JSONSchema is bumped whenever the shape below changes incompatibly, so
// consumers can refuse a document they do not understand.
const JSONSchema = 1

type jsonDoc struct {
	Version   int        `json:"version"`
	Base      string     `json:"base,omitempty"`
	MergeBase string     `json:"mergeBase,omitempty"`
	Head      string     `json:"head,omitempty"`
	Scope     string     `json:"scope,omitempty"`
	Files     []jsonFile `json:"files"`
}

type jsonFile struct {
	Path      string     `json:"path"`
	OldPath   string     `json:"oldPath,omitempty"`
	Status    string     `json:"status"`
	Binary    bool       `json:"binary,omitempty"`
	Note      string     `json:"note,omitempty"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Hunks     []jsonHunk `json:"hunks,omitempty"`
}

type jsonHunk struct {
	OldStart int        `json:"oldStart"`
	OldCount int        `json:"oldCount"`
	NewStart int        `json:"newStart"`
	NewCount int        `json:"newCount"`
	Section  string     `json:"section,omitempty"`
	Lines    []jsonLine `json:"lines"`
}

type jsonLine struct {
	Kind string `json:"kind"`
	Old  int    `json:"old,omitempty"`
	New  int    `json:"new,omitempty"`
	Text string `json:"text"`
}

// JSON encodes a change set for machines. It is deliberately faithful to the
// patch: no tabs expanded, no colour, no intra-line styling, and a stable
// order, because an agent may need to reconstruct exactly what changed.
func JSON(cs *patch.ChangeSet) ([]byte, error) {
	doc := jsonDoc{
		Version:   JSONSchema,
		Base:      cs.Base,
		MergeBase: cs.MergeBase,
		Head:      cs.Head,
		Scope:     cs.Scope,
		Files:     make([]jsonFile, 0, len(cs.Files)),
	}
	for _, f := range cs.Files {
		jf := jsonFile{
			Path:      f.Path(),
			OldPath:   f.OldPath,
			Status:    f.Status.String(),
			Binary:    f.Binary,
			Note:      f.Note,
			Additions: f.Additions,
			Deletions: f.Deletions,
		}
		if f.Status != patch.Renamed && f.Status != patch.Copied {
			jf.OldPath = ""
		}
		for _, h := range f.Hunks {
			jh := jsonHunk{
				OldStart: h.OldStart,
				OldCount: h.OldCount,
				NewStart: h.NewStart,
				NewCount: h.NewCount,
				Section:  h.Section,
				Lines:    make([]jsonLine, 0, len(h.Lines)),
			}
			for _, l := range h.Lines {
				kind := "context"
				switch l.Kind {
				case patch.Add:
					kind = "add"
				case patch.Del:
					kind = "del"
				case patch.NoNewline:
					kind = "nonewline"
				}
				jh.Lines = append(jh.Lines, jsonLine{Kind: kind, Old: l.OldNo, New: l.NewNo, Text: l.Text})
			}
			jf.Hunks = append(jf.Hunks, jh)
		}
		doc.Files = append(doc.Files, jf)
	}
	return json.MarshalIndent(doc, "", "  ")
}
