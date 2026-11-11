// Package metrics computes the scores that come straight from the path-based
// comparison of the two trees.
package metrics

import (
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
