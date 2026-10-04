package cognitive

import (
	"sort"

	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// Contributor is one function behind a score, for the report.
type Contributor struct {
	Symbol string `json:"symbol"`
	File   string `json:"file"`
	Value  int    `json:"value"`
}

// Result holds the two cognitive complexity scores.
type Result struct {
	// Delta is the largest increase among functions that exist on both
	// sides; 0 when none got harder to read.
	Delta int
	// NewMax is the largest complexity among functions the change adds.
	NewMax int
	// Deltas lists the functions whose complexity rose, largest first;
	// News lists new functions, most complex first.
	Deltas, News []Contributor
}

// TopN bounds each contributor list in the report.
const TopN = 5

// Measure scores the changed functions. Unchanged and deleted functions do
// not count. A function that gets simpler has a negative delta, which never
// lowers the score below 0.
func Measure(pairs []symbols.Pair) Result {
	var r Result
	for _, p := range pairs {
		if !p.Changed() || p.Head == nil {
			continue
		}
		head, ok := Of(p.Head)
		if !ok {
			continue
		}
		if p.Base == nil {
			r.NewMax = max(r.NewMax, head)
			r.News = append(r.News, contributor(p.Head, head))
			continue
		}
		base, ok := Of(p.Base)
		if !ok {
			// It gained a body (an interface method became concrete, say):
			// all of its complexity is new.
			base = 0
		}
		if d := head - base; d > 0 {
			r.Delta = max(r.Delta, d)
			r.Deltas = append(r.Deltas, contributor(p.Head, d))
		}
	}
	r.Deltas = top(r.Deltas)
	r.News = top(r.News)
	return r
}

func contributor(s *symbols.Symbol, v int) Contributor {
	return Contributor{Symbol: s.Display(), File: s.File, Value: v}
}

func top(cs []Contributor) []Contributor {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Value != cs[j].Value {
			return cs[i].Value > cs[j].Value
		}
		return cs[i].Symbol < cs[j].Symbol
	})
	if len(cs) > TopN {
		cs = cs[:TopN]
	}
	return cs
}
