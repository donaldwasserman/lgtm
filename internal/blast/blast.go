// Package blast measures how much existing code depends on what a change
// modified: the affected set (changed symbols plus everything that reaches
// them within a fixed number of hops on the base reference graph) and the
// blast radius (the affected set minus the changed symbols). It is specified
// by alloy/blast_radius.als.
//
// The graph is built by name, without type information: a symbol references
// another when its source mentions that name. Mentions resolve to the same
// file first, then the same module, then anywhere - but a name defined in
// more than MaxDefinitions places, or on the stoplist, resolves to nothing
// at the global tier. A missed reference only shrinks the radius
// (MonotoneInEdges in the model): the measure errs lenient, never strict.
package blast

import (
	"sort"

	"github.com/donaldwasserman/lgtm/internal/model"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

const (
	// Hops is how far references are followed from a changed symbol.
	Hops = 3
	// MaxDefinitions bounds global name resolution: a name defined in more
	// places than this is too ambiguous to attribute.
	MaxDefinitions = 5
)

// stoplist names are so common across languages that a global match says
// nothing about who depends on what.
var stoplist = map[string]bool{
	"String": true, "Error": true, "Close": true, "Read": true, "Write": true,
	"Len": true, "Less": true, "Swap": true, "Get": true, "Set": true,
	"get": true, "set": true, "new": true, "New": true, "init": true, "main": true,
	"toString": true, "equals": true, "hashCode": true, "constructor": true,
	"__init__": true, "__str__": true, "__repr__": true, "__eq__": true,
	"to_s": true, "initialize": true, "call": true, "run": true, "fmt": true,
}

// mentionTypes are leaf node types that name something.
var mentionTypes = map[string]bool{
	"identifier": true, "field_identifier": true, "property_identifier": true,
	"type_identifier": true, "constant": true, "shorthand_property_identifier": true,
	"private_property_identifier": true, "package_identifier": true,
}

// Graph is the reference graph over base symbols.
type Graph struct {
	Syms  []*symbols.Symbol
	index map[*symbols.Symbol]int
	// callers[v] lists the symbols that reference v, sorted, without v.
	callers [][]int
}

// Build resolves every mention in every base symbol.
func Build(t *symbols.Table) *Graph {
	g := &Graph{Syms: t.Symbols, index: map[*symbols.Symbol]int{}}
	defs := map[string][]int{}
	for i, s := range t.Symbols {
		g.index[s] = i
		defs[shortName(s.Name)] = append(defs[shortName(s.Name)], i)
	}
	sets := make([]map[int]bool, len(t.Symbols))
	for u, s := range t.Symbols {
		for name := range mentions(s) {
			for _, v := range resolve(t.Symbols, defs[name], u, name) {
				if sets[v] == nil {
					sets[v] = map[int]bool{}
				}
				sets[v][u] = true
			}
		}
	}
	g.callers = make([][]int, len(t.Symbols))
	for v, set := range sets {
		for u := range set {
			g.callers[v] = append(g.callers[v], u)
		}
		sort.Ints(g.callers[v])
	}
	return g
}

// resolve picks which definitions a mention in symbol u refers to.
func resolve(syms []*symbols.Symbol, cands []int, u int, name string) []int {
	var sameFile, sameModule, other []int
	for _, c := range cands {
		if c == u {
			continue
		}
		switch {
		case syms[c].File == syms[u].File:
			sameFile = append(sameFile, c)
		case syms[c].Module == syms[u].Module:
			sameModule = append(sameModule, c)
		default:
			other = append(other, c)
		}
	}
	switch {
	case len(sameFile) > 0:
		return sameFile
	case len(sameModule) > 0:
		return sameModule
	case len(cands) <= MaxDefinitions && !stoplist[name]:
		return other
	}
	return nil
}

// mentions collects the names a symbol's own source uses: everything but
// its member symbols and the identifier that names it.
func mentions(s *symbols.Symbol) map[string]bool {
	skip := map[*model.Node]bool{}
	for _, m := range s.Members {
		skip[m.Node] = true
	}
	if n := symbols.Field(s.Node, "name"); n != nil {
		skip[n] = true
	}
	out := map[string]bool{}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || skip[n] || symbols.IsComment(n.Type) {
			return
		}
		if len(n.Children) == 0 && mentionTypes[n.Type] {
			out[n.Text] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(s.Node)
	if w := s.Decorators(); w != nil {
		skip[s.Node] = true
		walk(w)
	}
	return out
}

// shortName is the name callers use: Ruby's "self.x" is called as x.
func shortName(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i+1:]
		}
	}
	return name
}

// Callers returns the symbols that reference s directly. s must be a base
// symbol.
func (g *Graph) Callers(s *symbols.Symbol) []*symbols.Symbol {
	i, ok := g.index[s]
	if !ok {
		return nil
	}
	out := make([]*symbols.Symbol, len(g.callers[i]))
	for k, u := range g.callers[i] {
		out[k] = g.Syms[u]
	}
	return out
}

// Affected returns the changed vertices plus every vertex that reaches one
// within k reverse hops, sorted. callers[v] lists the vertices with an edge
// into v. It is affected3 in alloy/blast_radius.als when k = 3.
func Affected(callers [][]int, changed []int, k int) []int {
	seen := map[int]bool{}
	frontier := []int{}
	for _, c := range changed {
		if !seen[c] {
			seen[c] = true
			frontier = append(frontier, c)
		}
	}
	for hop := 0; hop < k && len(frontier) > 0; hop++ {
		var next []int
		for _, v := range frontier {
			for _, u := range callers[v] {
				if !seen[u] {
					seen[u] = true
					next = append(next, u)
				}
			}
		}
		frontier = next
	}
	out := make([]int, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// Contributor is one changed symbol behind the score, for the report.
type Contributor struct {
	Symbol string `json:"symbol"`
	File   string `json:"file"`
	// Value is the number of symbols that reference it directly.
	Value int `json:"value"`
}

// Result is the blast radius of a set of changed base symbols.
type Result struct {
	Radius       int
	Contributors []Contributor
}

// TopN bounds the contributor list in the report.
const TopN = 5

// Measure computes the blast radius of the changed base symbols.
func Measure(g *Graph, changed []*symbols.Symbol) Result {
	var idx []int
	isChanged := map[int]bool{}
	for _, s := range changed {
		if i, ok := g.index[s]; ok && !isChanged[i] {
			idx = append(idx, i)
			isChanged[i] = true
		}
	}
	r := Result{Contributors: []Contributor{}}
	for _, v := range Affected(g.callers, idx, Hops) {
		if !isChanged[v] {
			r.Radius++
		}
	}
	for _, i := range idx {
		if n := len(g.callers[i]); n > 0 {
			s := g.Syms[i]
			r.Contributors = append(r.Contributors, Contributor{Symbol: s.Display(), File: s.File, Value: n})
		}
	}
	sort.SliceStable(r.Contributors, func(i, j int) bool {
		if r.Contributors[i].Value != r.Contributors[j].Value {
			return r.Contributors[i].Value > r.Contributors[j].Value
		}
		return r.Contributors[i].Symbol < r.Contributors[j].Symbol
	})
	if len(r.Contributors) > TopN {
		r.Contributors = r.Contributors[:TopN]
	}
	return r
}
