package parse

import (
	"fmt"
	"path"
	"strings"
)

// Globs is a set of path patterns: what the scan leaves out, what counts as a
// test, which changes always need review. A nil Globs matches nothing.
//
// Patterns match slash-separated paths relative to the tree root, and follow
// gitignore's conventions where they are cheap to follow:
//
//   - A pattern without a slash ("*.pb.go", "testdata") matches a file or
//     directory of that name at any depth.
//   - A pattern with a slash ("docs/gen", "src/**/*.min.js") is anchored at
//     the root. A leading "/" is allowed and ignored.
//   - "**" as a whole path segment matches any number of segments.
//   - A pattern that matches a directory excludes everything under it.
//   - A trailing "/" is ignored.
type Globs struct {
	patterns []pattern
}

type pattern struct {
	segs     []string
	anywhere bool // no slash in the source: match at any depth
}

// NewGlobs compiles patterns. Blank entries are ignored; a malformed glob
// is an error rather than a pattern that silently matches nothing.
func NewGlobs(patterns []string) (*Globs, error) {
	e := &Globs{}
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		p = strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
		if p == "" {
			continue
		}
		segs := strings.Split(p, "/")
		for _, s := range segs {
			if _, err := path.Match(s, ""); err != nil {
				return nil, fmt.Errorf("path pattern %q: %w", raw, err)
			}
		}
		e.patterns = append(e.patterns, pattern{
			segs:     segs,
			anywhere: !strings.Contains(strings.TrimPrefix(strings.TrimSpace(raw), "/"), "/") && !strings.HasPrefix(strings.TrimSpace(raw), "/"),
		})
	}
	return e, nil
}

// Match reports whether rel (slash-separated, relative to the root) matches,
// either itself or through a parent directory.
func (e *Globs) Match(rel string) bool {
	if e == nil || len(e.patterns) == 0 {
		return false
	}
	segs := strings.Split(rel, "/")
	for _, p := range e.patterns {
		for n := 1; n <= len(segs); n++ {
			prefix := segs[:n]
			if p.anywhere {
				if matchSegs(p.segs, prefix[n-1:]) {
					return true
				}
			} else if matchSegs(p.segs, prefix) {
				return true
			}
		}
	}
	return false
}

// matchSegs matches pattern segments against path segments, "**" spanning
// zero or more segments.
func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}
