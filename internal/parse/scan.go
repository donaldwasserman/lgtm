package parse

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/donaldwasserman/lgtm/internal/model"
)

// Scan walks a directory and parses every supported source file into a
// model.File keyed by its relative path. Unsupported files are skipped and
// unparseable files are recorded with an Error.
func Scan(ctx context.Context, dir string) (map[string]*model.File, error) {
	files := map[string]*model.File{}
	// One parser per language, reused across files and closed on the way out;
	// the binding allocates C memory that is not reclaimed otherwise.
	parsers := map[string]*sitter.Parser{}
	defer func() {
		for _, p := range parsers {
			p.Close()
		}
	}()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		spec := LanguageFor(path)
		if spec == nil {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		p, ok := parsers[spec.ID]
		if !ok {
			p = sitter.NewParser()
			p.SetLanguage(spec.GetLang())
			parsers[spec.ID] = p
		}
		tree, perr := p.ParseCtx(ctx, nil, src)
		if perr != nil {
			files[rel] = &model.File{Path: rel, Lang: spec.ID,
				Error: &model.FileErr{Msg: perr.Error()}}
			return nil
		}
		f := &model.File{Path: rel, Lang: spec.ID}
		// Tree-sitter reports malformed source as ERROR/MISSING nodes rather
		// than a parse failure, so this - not perr - is the branch that fires
		// on broken input.
		if tree.RootNode().HasError() {
			f.Error = &model.FileErr{Msg: "syntax error"}
		}
		f.Root = spec.build(tree.RootNode(), src, 0, 0)
		computeCallDepth(f.Root, spec)
		files[rel] = f
		return nil
	})
	return files, err
}

// skipDir reports whether a directory should be excluded from scanning.
func skipDir(path string) bool {
	base := filepath.Base(path)
	switch base {
	case ".git", "node_modules", "vendor", ".venv", "venv", "__pycache__",
		"dist", "build", "target", ".tox", ".idea", ".vscode":
		return true
	}
	return false
}

// build converts a Tree-sitter node into the language-agnostic named-node
// model, recursing only into named children and extracting a definition name
// where the node type declares one.
func (s *Spec) build(n *sitter.Node, src []byte, depth, index int) *model.Node {
	start, end := n.StartByte(), n.EndByte()
	name := ""
	if field, ok := s.nameField[n.Type()]; ok {
		if c := n.ChildByFieldName(field); c != nil && s.isNameType(c.Type()) {
			name = c.Content(src)
		}
	}
	callee := ""
	if s.IsCall(n.Type()) {
		if c := n.ChildByFieldName("function"); c != nil {
			// Rust turbofish: parse::<i32>(s) names parse, not i32.
			if c.Type() == "generic_function" {
				if f := c.ChildByFieldName("function"); f != nil {
					c = f
				}
			}
			callee = calleeName(c, src)
		}
	}
	node := &model.Node{
		Type:   n.Type(),
		Name:   name,
		Callee: callee,
		Text:   string(src[start:end]),
		Start:  start,
		End:    end,
		Index:  index,
		Nest:   depth,
	}
	// Walk all children rather than only the named ones: the field a child
	// fills (parameters, condition, superclass, ...) is indexed by its
	// position among all children. Only named children are kept, exactly as
	// before, so alignment and depth are unaffected.
	cc := int(n.ChildCount())
	for i := 0; i < cc; i++ {
		child := n.Child(i)
		if child == nil || !child.IsNamed() {
			continue
		}
		// skip generated error / missing nodes: they carry no real structure
		// and would otherwise pollute alignment keys and depth
		if child.IsError() || child.IsMissing() {
			continue
		}
		c := s.build(child, src, depth+1, len(node.Children))
		c.Field = n.FieldNameForChild(i)
		node.Children = append(node.Children, c)
	}
	return node
}

// calleeName extracts the final identifier segment of a call target's source
// text, e.g. "pkg.Func" -> "Func", "self.method" -> "method".
func calleeName(n *sitter.Node, src []byte) string {
	content := n.Content(src)
	// take the last identifier-like token
	last := ""
	cur := ""
	for _, r := range content {
		if isIdentRune(r) {
			cur += string(r)
		} else {
			if cur != "" {
				last = cur
				cur = ""
			}
		}
	}
	if cur != "" {
		last = cur
	}
	return last
}

func isIdentRune(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// computeCallDepth builds an intra-file call graph from the definitions found
// in the tree and the call sites within each definition, then computes the
// longest call-chain length (with cycle guard) reachable under each node and
// folds it into each node's Depth = Nest + Call.
//
// v1 is intra-file only: definitions and calls are matched within one file.
func computeCallDepth(root *model.Node, s *Spec) {
	if root == nil {
		return
	}
	// def -> set of callee names it calls (call graph adjacency)
	graph := map[string]map[string]bool{}
	collectEdges(root, graph, s, "")

	// attach per-node call-chain length anchored at each definition's subtree
	attach(root, graph, s, "")
}

// collectEdges walks the tree tracking the innermost enclosing definition
// (curDef) and ascribes each call site to it, building the call graph.
func collectEdges(n *model.Node, graph map[string]map[string]bool, s *Spec, curDef string) {
	if n == nil {
		return
	}
	if _, ok := s.nameField[n.Type]; ok && n.Name != "" {
		if _, exists := graph[n.Name]; !exists {
			graph[n.Name] = map[string]bool{}
		}
		curDef = n.Name
	}
	if s.IsCall(n.Type) && n.Callee != "" && curDef != "" && n.Callee != curDef {
		if graph[curDef] == nil {
			graph[curDef] = map[string]bool{}
		}
		graph[curDef][n.Callee] = true
	}
	for _, c := range n.Children {
		collectEdges(c, graph, s, curDef)
	}
}

// attach walks the tree tracking the innermost enclosing definition (curDef).
// Within a definition's subtree, Call is the longest call chain reachable from
// that definition (its impact depth); Depth = structural Nest + Call.
func attach(n *model.Node, graph map[string]map[string]bool, s *Spec, curDef string) {
	attachWith(n, callChainLengths(graph), s, curDef)
}

func attachWith(n *model.Node, chain map[string]int, s *Spec, curDef string) {
	if n == nil {
		return
	}
	if _, ok := s.nameField[n.Type]; ok && n.Name != "" {
		curDef = n.Name
	}
	n.Call = 0
	if curDef != "" {
		n.Call = chain[curDef]
	}
	n.Depth = n.Nest + n.Call
	for _, c := range n.Children {
		attachWith(c, chain, s, curDef)
	}
}

// longestPath returns the length of the longest call chain starting at def.
func longestPath(def string, graph map[string]map[string]bool) int {
	return callChainLengths(graph)[def]
}

// callChainLengths returns, for every name in the call graph (callers and
// callees alike), the length of the longest call chain starting there. Every
// edge counts 1; a callee with no definition in the file is a leaf.
//
// Recursion makes the graph cyclic, so it is condensed into strongly connected
// components first (Tarjan). Calls inside a component add nothing - a cycle
// has no longest path - and every member of a component shares its length:
// the longest chain out of the component. On an acyclic graph this is exactly
// the longest path.
func callChainLengths(graph map[string]map[string]bool) map[string]int {
	// Deterministic node order and adjacency, so results never depend on map
	// iteration.
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for def, callees := range graph {
		add(def)
		for c := range callees {
			add(c)
		}
	}
	sort.Strings(names)
	idx := make(map[string]int, len(names))
	for i, n := range names {
		idx[n] = i
	}
	adj := make([][]int, len(names))
	for def, callees := range graph {
		for c := range callees {
			adj[idx[def]] = append(adj[idx[def]], idx[c])
		}
	}
	for _, a := range adj {
		sort.Ints(a)
	}

	comp := tarjan(adj)

	// Tarjan emits components in reverse topological order: every component
	// a component calls into is numbered before it. So one pass in component
	// order sees each callee's length before its callers need it.
	nComp := 0
	for _, c := range comp {
		nComp = max(nComp, c+1)
	}
	members := make([][]int, nComp)
	for v, c := range comp {
		members[c] = append(members[c], v)
	}
	length := make([]int, nComp)
	for c := 0; c < nComp; c++ {
		for _, v := range members[c] {
			for _, w := range adj[v] {
				if comp[w] != c {
					length[c] = max(length[c], 1+length[comp[w]])
				}
			}
		}
	}
	out := make(map[string]int, len(names))
	for i, n := range names {
		out[n] = length[comp[i]]
	}
	return out
}

// tarjan labels each vertex with its strongly connected component, numbering
// components in reverse topological order. It is iterative so that a long
// call chain cannot overflow the goroutine stack.
func tarjan(adj [][]int) []int {
	n := len(adj)
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	comp := make([]int, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	next, nComp := 0, 0

	type frame struct{ v, edge int }
	for root := 0; root < n; root++ {
		if index[root] >= 0 {
			continue
		}
		call := []frame{{root, 0}}
		index[root], low[root] = next, next
		next++
		stack = append(stack, root)
		onStack[root] = true
		for len(call) > 0 {
			f := &call[len(call)-1]
			v := f.v
			if f.edge < len(adj[v]) {
				w := adj[v][f.edge]
				f.edge++
				if index[w] < 0 {
					index[w], low[w] = next, next
					next++
					stack = append(stack, w)
					onStack[w] = true
					call = append(call, frame{w, 0})
				} else if onStack[w] {
					low[v] = min(low[v], index[w])
				}
				continue
			}
			// v is finished: close its component if it is a root, then
			// propagate its low-link to the caller frame.
			if low[v] == index[v] {
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp[w] = nComp
					if w == v {
						break
					}
				}
				nComp++
			}
			call = call[:len(call)-1]
			if len(call) > 0 {
				u := call[len(call)-1].v
				low[u] = min(low[u], low[v])
			}
		}
	}
	return comp
}
