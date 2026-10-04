// Package symbols finds the symbols in a scanned source tree and pairs base
// symbols with head symbols.
//
// A symbol is identified by its language module, its enclosing declaration
// and its name - not by the file it lives in (docs/adr/0001). Moving a
// function between two files of one Go package therefore leaves it the same
// symbol, and moving it to another package does not.
//
// The newer measures (cognitive complexity, significance, blast radius) are
// built on this comparison. Edit depth and file breadth keep the path-based
// differ in internal/diff, whose results are frozen.
package symbols

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strings"

	"github.com/donaldwasserman/lgtm/internal/model"
)

// Kind is what sort of declaration a symbol is.
type Kind string

const (
	KindFunc   Kind = "func"   // a free function
	KindMethod Kind = "method" // a function belonging to a type
	KindType   Kind = "type"   // class, struct, interface, trait, enum, alias
	KindModule Kind = "module" // Ruby module, Rust mod
)

// Symbol is one named declaration.
type Symbol struct {
	// Module is the language module the symbol belongs to: Go package
	// directory, Java package, Python dotted module, Rust module path,
	// JavaScript/TypeScript file, Ruby constant path.
	Module string
	// Container is the chain of enclosing declarations inside the module,
	// joined by ".", such as a class or a Go receiver type.
	Container string
	Name      string
	Kind      Kind
	// Exported: callers outside the module can depend on the symbol.
	Exported bool
	File     string
	Lang     string
	// Node is the declaration. For a member declared through a wrapper (a
	// Python decorator, a JS export statement), it is the inner declaration.
	Node *model.Node
	// Members are the symbols declared inside this one (a class's methods).
	// Their subtrees are excluded from this symbol's own fingerprint.
	Members []*Symbol
	// wrapper is the node around Node that belongs to this symbol (an export
	// statement, a decorated definition), if any.
	wrapper *model.Node
	fp      string // cached Fingerprint
}

// ID is the symbol's identity across base and head.
func (s *Symbol) ID() string {
	return s.Module + "|" + s.Container + "|" + s.Name
}

// Display names the symbol for people: its module, then Container.Name.
func (s *Symbol) Display() string {
	name := s.Name
	if s.Container != "" {
		name = s.Container + "." + name
	}
	if s.Module == "" || s.Module == "." {
		return name
	}
	return s.Module + " " + name
}

// Callable reports whether the symbol has a body of statements.
func (s *Symbol) Callable() bool {
	return (s.Kind == KindFunc || s.Kind == KindMethod) && Body(s.Node) != nil
}

// Body returns a function declaration's body, or nil when it has none (an
// interface method, a Rust trait signature).
func Body(n *model.Node) *model.Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Field == "body" {
			return c
		}
	}
	// Ruby methods keep their statements in an unnamed body_statement.
	for _, c := range n.Children {
		if c.Type == "body_statement" {
			return c
		}
	}
	return nil
}

// Table is every symbol on one side of a pull request.
type Table struct {
	// Symbols in a stable order: by file, then source position.
	Symbols []*Symbol
	// Packages maps each file to the package it belongs to, for breadth:
	// the Java package, or the file's directory elsewhere.
	Packages map[string]string
}

// Extract finds the symbols in every parsed file. Files that failed to parse
// contribute nothing: the gate already forces review for them.
func Extract(files map[string]*model.File) *Table {
	t := &Table{Packages: map[string]string{}}
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := files[p]
		t.Packages[p] = packageOf(f)
		if f.Root == nil || f.Error != nil {
			continue
		}
		ex := extractors[f.Lang]
		if ex == nil {
			continue
		}
		for _, s := range ex(f) {
			flatten(s, &t.Symbols)
		}
	}
	return t
}

func flatten(s *Symbol, out *[]*Symbol) {
	*out = append(*out, s)
	for _, m := range s.Members {
		flatten(m, out)
	}
}

// packageOf is the package a file belongs to, for module breadth.
func packageOf(f *model.File) string {
	dir := path.Dir(toSlash(f.Path))
	if f.Lang == "java" && f.Root != nil {
		for _, c := range f.Root.Children {
			if c.Type == "package_declaration" && len(c.Children) > 0 {
				return c.Children[0].Text
			}
		}
	}
	return dir
}

func toSlash(p string) string { return strings.ReplaceAll(p, "\\", "/") }

// Fingerprint hashes the symbol's own content: its structure and leaf text,
// leaving out comments and its member symbols. Two symbols with equal
// fingerprints are unchanged as far as any measure is concerned, whatever
// whitespace, comments or members around them changed.
func (s *Symbol) Fingerprint() string {
	if s.fp == "" {
		s.fp = s.fingerprint()
	}
	return s.fp
}

func (s *Symbol) fingerprint() string {
	skip := map[*model.Node]bool{}
	for _, m := range s.Members {
		skip[m.Node] = true
		// A wrapper (decorator, export statement) around a member is also
		// the member's.
		if m.Node != nil {
			skip[m.wrapper] = true
		}
	}
	// A node holding nothing but members and comments - a class body, say -
	// contributes nothing either, so a class that lost its last member to
	// another file reads the same as one that never had it.
	memo := map[*model.Node]bool{}
	var empty func(n *model.Node) bool
	empty = func(n *model.Node) bool {
		if n == nil || skip[n] || IsComment(n.Type) {
			return true
		}
		if len(n.Children) == 0 {
			return false
		}
		if v, ok := memo[n]; ok {
			return v
		}
		v := true
		for _, c := range n.Children {
			if !empty(c) {
				v = false
				break
			}
		}
		memo[n] = v
		return v
	}
	h := sha256.New()
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n != s.Node && empty(n) {
			return
		}
		h.Write([]byte(n.Type))
		h.Write([]byte{0})
		h.Write([]byte(n.Field))
		h.Write([]byte{0})
		if len(n.Children) == 0 {
			h.Write([]byte(n.Text))
		}
		h.Write([]byte{1})
		for _, c := range n.Children {
			walk(c)
		}
		h.Write([]byte{2})
	}
	walk(s.Node)
	return hex.EncodeToString(h.Sum(nil))
}

// IsComment reports whether a node type is a comment in any supported
// grammar (comment, line_comment, block_comment, ...).
func IsComment(typ string) bool { return strings.Contains(typ, "comment") }

// Pair is one symbol matched across the pull request. Base is nil for a
// symbol the change adds, Head is nil for one it deletes.
type Pair struct {
	Base, Head *Symbol
}

// Changed reports whether the pair differs: added, deleted, or modified.
func (p Pair) Changed() bool {
	if p.Base == nil || p.Head == nil {
		return true
	}
	return p.Base.Fingerprint() != p.Head.Fingerprint() || p.Base.Exported != p.Head.Exported
}

// Compare pairs base and head symbols by identity. Symbols sharing an
// identity on one side - Java overloads, a Ruby class reopened in two files -
// are paired first by identical content, then by identical parameter list,
// then in source order.
func Compare(base, head *Table) []Pair {
	b := groupByID(base.Symbols)
	h := groupByID(head.Symbols)
	ids := map[string]bool{}
	for id := range b {
		ids[id] = true
	}
	for id := range h {
		ids[id] = true
	}
	var sorted []string
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)

	var out []Pair
	for _, id := range sorted {
		out = append(out, pairGroup(b[id], h[id])...)
	}
	return out
}

func groupByID(syms []*Symbol) map[string][]*Symbol {
	m := map[string][]*Symbol{}
	for _, s := range syms {
		m[s.ID()] = append(m[s.ID()], s)
	}
	return m
}

func pairGroup(bs, hs []*Symbol) []Pair {
	var out []Pair
	usedB := make([]bool, len(bs))
	usedH := make([]bool, len(hs))
	match := func(key func(*Symbol) string) {
		for i, b := range bs {
			if usedB[i] {
				continue
			}
			for j, h := range hs {
				if !usedH[j] && key(b) == key(h) {
					usedB[i], usedH[j] = true, true
					out = append(out, Pair{b, h})
					break
				}
			}
		}
	}
	match((*Symbol).Fingerprint)
	match(func(s *Symbol) string { return fieldText(s.Node, "parameters") })
	match(func(*Symbol) string { return "" }) // whatever is left, in order
	for i, b := range bs {
		if !usedB[i] {
			out = append(out, Pair{Base: b})
		}
	}
	for j, h := range hs {
		if !usedH[j] {
			out = append(out, Pair{Head: h})
		}
	}
	return out
}

// field returns the first child filling the named field.
func field(n *model.Node, name string) *model.Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Field == name {
			return c
		}
	}
	return nil
}

func fieldText(n *model.Node, name string) string {
	if c := field(n, name); c != nil {
		return c.Text
	}
	return ""
}

// Field is exported for the measures built on symbols.
func Field(n *model.Node, name string) *model.Node { return field(n, name) }

// ModulesTouched counts the distinct packages among the given files.
func ModulesTouched(files []string, base, head *Table) int {
	seen := map[string]bool{}
	for _, f := range files {
		p, ok := head.Packages[f]
		if !ok {
			p, ok = base.Packages[f]
		}
		if !ok {
			p = path.Dir(toSlash(f))
		}
		seen[p] = true
	}
	return len(seen)
}
