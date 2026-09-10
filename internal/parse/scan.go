package parse

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"lgtm/internal/model"
)

// Scan walks a directory and parses every supported source file into a
// model.File keyed by its relative path. Unsupported files are skipped and
// unparseable files are recorded with an Error.
func Scan(ctx context.Context, dir string) (map[string]*model.File, error) {
	files := map[string]*model.File{}
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
		rel := strings.TrimPrefix(path, dir)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		p := sitter.NewParser()
		p.SetLanguage(spec.GetLang())
		tree, perr := p.ParseCtx(ctx, nil, src)
		if perr != nil {
			files[rel] = &model.File{Path: rel, Lang: spec.ID,
				Error: &model.FileErr{Msg: perr.Error()}}
			return nil
		}
		root := spec.build(tree.RootNode(), src, 0)
		computeCallDepth(root, spec)
		files[rel] = &model.File{Path: rel, Lang: spec.ID, Root: root}
		return nil
	})
	return files, err
}

// skipDir reports whether a directory should be excluded from scanning.
func skipDir(path string) bool {
	base := filepath.Base(path)
	switch base {
	case ".git", "node_modules", "vendor", ".venv", "venv", "__pycache__",
		"dist", "build", ".tox", ".idea", ".vscode":
		return true
	}
	return false
}

// build converts a Tree-sitter node into the language-agnostic named-node
// model, recursing only into named children and extracting a definition name
// where the node type declares one.
func (s *Spec) build(n *sitter.Node, src []byte, depth int) *model.Node {
	start, end := n.StartByte(), n.EndByte()
	name := ""
	if field, ok := s.nameField[n.Type()]; ok {
		if c := n.ChildByFieldName(field); c != nil && c.Type() == "identifier" {
			name = c.Content(src)
		}
	}
	callee := ""
	if s.IsCall(n.Type()) {
		if c := n.ChildByFieldName("function"); c != nil {
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
		Nest:   depth,
	}
	nc := n.NamedChildCount()
	for i := 0; i < int(nc); i++ {
		child := n.NamedChild(i)
		if child == nil {
			continue
		}
		// skip generated error / missing nodes
		node.Children = append(node.Children, s.build(child, src, depth+1))
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
	if n == nil {
		return
	}
	mine := 0
	if _, ok := s.nameField[n.Type]; ok && n.Name != "" {
		curDef = n.Name
		mine = longestPath(curDef, graph)
	} else if curDef != "" {
		mine = longestPath(curDef, graph)
	}
	n.Call = mine
	n.Depth = n.Nest + n.Call
	for _, c := range n.Children {
		attach(c, graph, s, curDef)
	}
}

// longestPath returns the length of the longest call chain starting at def,
// guarding against cycles by capping the visited-depth. Every edge counts 1.
func longestPath(def string, graph map[string]map[string]bool) int {
	visited := map[string]int{}
	var dfs func(string) int
	dfs = func(name string) int {
		if v, ok := visited[name]; ok {
			return v
		}
		best := 0
		for callee := range graph[name] {
			if _, seen := visited[callee]; seen {
				continue // cycle: stop to avoid infinite recursion
			}
			d := 1 + dfs(callee)
			if d > best {
				best = d
			}
		}
		visited[name] = best
		return best
	}
	return dfs(def)
}
