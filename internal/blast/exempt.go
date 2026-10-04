package blast

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/donaldwasserman/lgtm/internal/parse"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// Exemption decides whether deleting an existing symbol is exempt: it was not
// exported, the graph finds no reference to it, and its name appears
// nowhere else in either tree as plain text (deletionExempt in
// alloy/blast_radius.als). The text check is what makes the exemption sound
// when the name-based graph misses a reference: any real reference names
// the symbol somewhere (ExemptionSoundUnderMissedCalls).
type Exemption struct {
	g     *Graph
	roots []string
	texts map[string][]byte // path -> contents, loaded on first use
}

// NewExemption checks deletions against g and the files under roots (the
// base and head trees).
func NewExemption(g *Graph, roots ...string) *Exemption {
	return &Exemption{g: g, roots: roots}
}

// MaxFileSize bounds the files searched for mentions.
const MaxFileSize = 2 << 20

// Exempt reports whether deleting s is exempt.
func (e *Exemption) Exempt(s *symbols.Symbol) bool {
	if s.Exported || len(e.g.Callers(s)) > 0 {
		return false // decided without reading any files
	}
	return exemptRule(s.Exported, false, e.mentionedElsewhere(s))
}

// exemptRule is deletionExempt in alloy/blast_radius.als.
func exemptRule(exported, called, mentioned bool) bool {
	return !exported && !called && !mentioned
}

func (e *Exemption) mentionedElsewhere(s *symbols.Symbol) bool {
	if e.texts == nil {
		e.load()
	}
	name := []byte(shortName(s.Name))
	// The declaration itself, in its base file, does not count.
	decl := s.Node
	if w := s.Wrapper(); w != nil {
		decl = w
	}
	for path, text := range e.texts {
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(e.roots[0], s.File)) {
			masked := append([]byte(nil), text...)
			if int(decl.End) <= len(masked) {
				for i := decl.Start; i < decl.End; i++ {
					masked[i] = ' '
				}
			}
			text = masked
		}
		if containsWord(text, name) {
			return true
		}
	}
	return false
}

func (e *Exemption) load() {
	e.texts = map[string][]byte{}
	for _, root := range e.roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && parse.SkipDir(path) {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileSize {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil || bytes.IndexByte(b, 0) >= 0 {
				return nil // unreadable or binary
			}
			e.texts[path] = b
			return nil
		})
	}
}

// containsWord reports whether name occurs in text as a whole identifier.
func containsWord(text, name []byte) bool {
	for i := 0; ; {
		j := bytes.Index(text[i:], name)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(name)
		if (start == 0 || !identByte(text[start-1])) && (end == len(text) || !identByte(text[end])) {
			return true
		}
		i = start + 1
	}
}

func identByte(b byte) bool {
	return b == '_' || b == '$' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
