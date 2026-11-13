// Package metrics computes the scores that come straight from the path-based
// comparison of the two trees.
package metrics

import (
	"bytes"
	"sort"
	"strings"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/diff"
	"github.com/donaldwasserman/lgtm/internal/model"
)

// TopN bounds each contributor list in the report.
const TopN = 5

// Compute converts a diff result into the depth scores:
//
//	EditDepth: max depth of modified + deleted (existing) AST nodes
//	NewDepth:  max depth of newly added nodes
//
// The other measures are left at their zero values; their own packages and
// the caller fill them in.
func Compute(res *diff.Result) eval.Scores {
	var m eval.Scores
	for _, fc := range res.Files {
		for _, c := range fc.Changes {
			switch c.Kind {
			case model.KindInsert:
				m.NewDepth = max(m.NewDepth, c.Depth)
			case model.KindDelete, model.KindModify:
				m.EditDepth = max(m.EditDepth, c.Depth)
			}
		}
	}
	return m
}

// Contributor is one place behind a score, for the report.
type Contributor struct {
	File  string `json:"file"`
	Line  int    `json:"line,omitempty"`
	Value int    `json:"value"`
	// Deleted: the file no longer exists on the head side.
	Deleted bool `json:"deleted,omitempty"`
}

// DepthContributors names the deepest change to existing code in each file,
// deepest first.
func DepthContributors(res *diff.Result) []Contributor {
	out := []Contributor{}
	for _, fc := range res.Files {
		best := Contributor{File: fc.Path, Deleted: fc.Kind == model.KindDelete}
		for _, c := range fc.Changes {
			if c.Kind != model.KindInsert && c.Depth > best.Value {
				best.Value, best.Line = c.Depth, c.Line
			}
		}
		if best.Value > 0 {
			out = append(out, best)
		}
	}
	return top(out)
}

// LinesAdded counts the non-blank lines the change adds to the files keep
// accepts. It compares the lines of all those files as one multiset, so moving
// code, within a file or to another one, adds nothing.
func LinesAdded(res *diff.Result, base, head map[string]*model.File, keep func(string) bool) (int, []Contributor) {
	have := map[string]int{}
	for _, fc := range res.Files {
		if keep(fc.Path) {
			remember(have, rootText(base[fc.Path]))
		}
	}
	total := 0
	out := []Contributor{}
	for _, fc := range res.Files {
		if !keep(fc.Path) {
			continue
		}
		n := added(have, rootText(head[fc.Path]))
		if n > 0 {
			total += n
			out = append(out, Contributor{File: fc.Path, Value: n})
		}
	}
	return total, top(out)
}

func rootText(f *model.File) string {
	if f == nil || f.Root == nil {
		return ""
	}
	return f.Root.Text
}

func remember(have map[string]int, base string) {
	for _, l := range strings.Split(base, "\n") {
		have[strings.TrimSpace(l)]++
	}
}

// added counts head's lines not in have, using up the ones it finds.
func added(have map[string]int, head string) int {
	n := 0
	for _, l := range strings.Split(head, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if have[l] > 0 {
			have[l]--
		} else {
			n++
		}
	}
	return n
}

// Top sorts contributors largest first and keeps the TopN.
func Top(cs []Contributor) []Contributor { return top(cs) }

func top(cs []Contributor) []Contributor {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Value != cs[j].Value {
			return cs[i].Value > cs[j].Value
		}
		return cs[i].File < cs[j].File
	})
	if len(cs) > TopN {
		cs = cs[:TopN]
	}
	return cs
}

// TextChange compares the non-blank lines of two versions of a file that
// lgtm cannot parse. It returns how many lines the head adds and how many it
// drops, so a reorder or a whitespace edit is neither.
func TextChange(base, head string) (added0, removed int) {
	have := map[string]int{}
	remember(have, base)
	delete(have, "") // blank lines don't count
	added0 = added(have, head)
	for _, n := range have {
		removed += n
	}
	return added0, removed
}

// TestLinesRemoved counts the non-blank lines that disappear from test
// files. base and head hold the changed test files on each side, by path.
// A line counts when it is in a base file and no head test file still has
// it: lines are compared trimmed, as a multiset across all files, so a test
// moved or renamed to another file counts for nothing and a rewritten one
// counts only the lines that actually went away. Contributors are per base
// file, largest first.
func TestLinesRemoved(base, head map[string][]byte) (int, []Contributor) {
	left := map[string]int{}
	for _, data := range head {
		for _, l := range testLines(data) {
			left[l]++
		}
	}
	paths := make([]string, 0, len(base))
	for p := range base {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	total := 0
	contributors := []Contributor{}
	for _, p := range paths {
		n := 0
		for _, l := range testLines(base[p]) {
			if left[l] > 0 {
				left[l]--
				continue
			}
			n++
		}
		if n > 0 {
			_, kept := head[p]
			contributors = append(contributors, Contributor{File: p, Value: n, Deleted: !kept})
			total += n
		}
	}
	sort.SliceStable(contributors, func(i, j int) bool { return contributors[i].Value > contributors[j].Value })
	if len(contributors) > TopN {
		contributors = contributors[:TopN]
	}
	return total, contributors
}

// testLines splits a file into trimmed non-blank lines. A file with a NUL
// byte is binary and has none.
func testLines(data []byte) []string {
	if bytes.IndexByte(data, 0) >= 0 {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
