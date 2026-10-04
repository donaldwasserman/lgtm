// Package significance rates how strongly a change to one existing symbol
// can affect code outside it, after the ChangeDistiller taxonomy (Fluri &
// Gall 2006). It is specified by alloy/significance.als.
//
// A changed symbol is split into parts - signature, visibility, supertypes,
// body (conditions, error flow, other statements) and anything else - and
// each part is compared on a canonical form in which locally bound names are
// replaced by their binding order. Renaming a local therefore changes
// nothing, and renaming a parameter is not a signature change. The level is
// the highest level among the parts that differ.
package significance

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/model"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// Part is one aspect of a symbol a change can touch (alloy/significance.als).
type Part string

const (
	Rename       Part = "rename"       // only locals renamed: data flow unchanged
	Statement    Part = "statement"    // statements in a body
	Condition    Part = "condition"    // the predicate of an if, loop or switch
	ErrorFlow    Part = "error-flow"   // catch/rescue/except, throw/raise, panic, ?
	Signature    Part = "signature"    // parameters, results, type parameters
	Visibility   Part = "visibility"   // modifiers or export status
	Supertypes   Part = "supertypes"   // extends/implements, embedding, mixins, bounds
	Unclassified Part = "unclassified" // changed, but none of the above
)

// partLevel mirrors partLevel in alloy/significance.als.
func partLevel(p Part, exportedBefore, exportedAfter bool) eval.Level {
	switch p {
	case Rename:
		return eval.None
	case Statement:
		return eval.Low
	case Condition, ErrorFlow, Unclassified:
		return eval.Medium
	case Supertypes:
		return eval.Crucial
	case Signature:
		if exportedBefore {
			return eval.High
		}
		return eval.Medium
	case Visibility:
		if exportedBefore && !exportedAfter {
			return eval.Crucial
		}
		return eval.Medium
	}
	return eval.Medium // an unknown part is never below medium
}

// Level mirrors level in alloy/significance.als: the highest part level.
func Level(parts []Part, exportedBefore, exportedAfter bool) eval.Level {
	l := eval.None
	for _, p := range parts {
		l = max(l, partLevel(p, exportedBefore, exportedAfter))
	}
	return l
}

// Classify lists the parts in which two versions of one symbol differ.
// Called only on a pair whose fingerprints or export status differ, so the
// result is never empty: a difference found nowhere else is Rename.
func Classify(base, head *symbols.Symbol) []Part {
	l := langs[head.Lang]
	if l == nil {
		return []Part{Unclassified}
	}
	parts := map[Part]bool{}
	if base.Exported != head.Exported {
		parts[Visibility] = true
	}
	// Locals renamed and nothing else: the whole declaration is equal once
	// every bound name is numbered in binding order.
	if len(parts) == 0 && whole(base, l) == whole(head, l) {
		return []Part{Rename}
	}
	// Otherwise compare part by part. Only parameters are numbered here:
	// numbering locals would let one added local renumber every later one
	// and make untouched conditions look changed.
	b := decompose(base, l)
	h := decompose(head, l)
	for _, p := range []Part{Signature, Visibility, Supertypes, Condition, ErrorFlow, Statement, Unclassified} {
		if b[p] != h[p] {
			parts[p] = true
		}
	}
	if len(parts) == 0 {
		parts[Rename] = true
	}
	var out []Part
	for p := range parts {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func newCanon(s *symbols.Symbol, l *lang, paramsOnly bool) *canon {
	c := &canon{l: l, bound: map[string]int{}, skip: map[*model.Node]bool{},
		transparent: map[*model.Node]bool{}, paramsOnly: paramsOnly}
	for _, m := range s.Members {
		c.skip[m.Node] = true
	}
	c.bind(s.Node)
	return c
}

// whole hashes the entire declaration with every bound name numbered.
func whole(s *symbols.Symbol, l *lang) string {
	c := newCanon(s, l, false)
	h := c.hash(s.Node, nil)
	if w := s.Decorators(); w != nil {
		h += c.hash(w, map[*model.Node]bool{s.Node: true})
	}
	return h
}

// decompose hashes each part of a symbol, with parameters numbered.
func decompose(s *symbols.Symbol, l *lang) map[Part]string {
	c := newCanon(s, l, true)

	acc := map[Part][]string{}
	add := func(p Part, n *model.Node) { acc[p] = append(acc[p], c.hash(n, nil)) }
	callable := s.Kind == symbols.KindFunc || s.Kind == symbols.KindMethod
	for _, ch := range s.Node.Children {
		switch {
		case c.skip[ch] || symbols.IsComment(ch.Type):
		case ch.Field == "name":
			// identity, not content
		case l.signature[ch.Field] || l.signatureTypes[ch.Type]:
			add(Signature, ch)
		case l.supertypeFields[ch.Field] || l.supertypeTypes[ch.Type]:
			add(Supertypes, ch)
		case l.modifierTypes[ch.Type]:
			vis, other := l.splitModifiers(ch)
			if vis != "" {
				acc[Visibility] = append(acc[Visibility], vis)
			}
			if other != "" {
				acc[Signature] = append(acc[Signature], other)
			}
		case callable && isBody(ch, l):
			c.body(ch, acc)
		case !callable && ch.Field == "type" && !l.bodyTypes[ch.Type]:
			add(Signature, ch) // a type alias or defined type's target
		case !callable && isBody(ch, l):
			c.typeBody(ch, acc)
		default:
			add(Unclassified, ch)
		}
	}
	// Decorators around the declaration.
	if w := s.Decorators(); w != nil {
		acc[Unclassified] = append(acc[Unclassified], c.hash(w, map[*model.Node]bool{s.Node: true}))
	}
	out := map[Part]string{}
	for p, hs := range acc {
		sort.Strings(hs) // multisets: order of conditions etc. is the statement part's concern
		out[p] = strings.Join(hs, ",")
	}
	return out
}

func isBody(n *model.Node, l *lang) bool {
	return n.Field == "body" || n.Field == "type" || l.bodyTypes[n.Type]
}

// canon hashes subtrees with locally bound identifiers replaced by their
// binding order.
type canon struct {
	l     *lang
	bound map[string]int
	skip  map[*model.Node]bool
	// transparent nodes add error flow around an expression (Rust's ?): the
	// statement around them is compared as if they were not there.
	transparent map[*model.Node]bool
	// paramsOnly numbers only names bound in the signature.
	paramsOnly bool
}

// bind records every name bound inside the declaration: parameters, local
// variables, loop and catch variables, lambda parameters.
func (c *canon) bind(root *model.Node) {
	var walk func(n *model.Node, binding bool)
	walk = func(n *model.Node, binding bool) {
		if n == nil || c.skip[n] {
			return
		}
		if binding && n.Field == "type" {
			return // a parameter's type is not a binding
		}
		if binding && n.Field != "" && c.l.valueFields[n.Field] {
			binding = false // the right-hand side of x = value
		}
		if binding && len(n.Children) == 0 && c.l.identTypes[n.Type] {
			if _, ok := c.bound[n.Text]; !ok {
				c.bound[n.Text] = len(c.bound)
			}
		}
		for _, ch := range n.Children {
			if c.paramsOnly && n == root && !c.l.signature[ch.Field] {
				continue // locals keep their names
			}
			walk(ch, binding || c.l.binds(n.Type, ch.Field))
		}
	}
	walk(root, false)
}

// hash writes a subtree's canonical form. Nodes in mask are written as a
// placeholder, so the caller can hash "everything except" some parts; a
// transparent node is written as its children alone.
func (c *canon) hash(n *model.Node, mask map[*model.Node]bool) string {
	h := sha256.New()
	// field is the field name to write for n: its own, or that of the
	// transparent wrapper it stands in for.
	var walk func(n *model.Node, field string)
	walk = func(n *model.Node, field string) {
		if n == nil || c.skip[n] || symbols.IsComment(n.Type) {
			return
		}
		if mask[n] {
			h.Write([]byte{3})
			return
		}
		if c.transparent[n] {
			for _, ch := range n.Children {
				walk(ch, field)
			}
			return
		}
		h.Write([]byte(n.Type))
		h.Write([]byte{0})
		h.Write([]byte(field))
		h.Write([]byte{0})
		if len(n.Children) == 0 {
			if i, ok := c.bound[n.Text]; ok && c.l.identTypes[n.Type] {
				h.Write([]byte{4, byte(i), byte(i >> 8)})
			} else {
				h.Write([]byte(n.Text))
			}
		} else {
			h.Write([]byte(symbols.Tokens(n)))
		}
		h.Write([]byte{1})
		for _, ch := range n.Children {
			walk(ch, ch.Field)
		}
		h.Write([]byte{2})
	}
	walk(n, n.Field)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// body splits a function body into conditions, error flow, and the rest.
func (c *canon) body(b *model.Node, acc map[Part][]string) {
	mask := map[*model.Node]bool{}
	var walk func(n *model.Node, parent *model.Node)
	walk = func(n *model.Node, parent *model.Node) {
		if n == nil || c.skip[n] {
			return
		}
		switch {
		case c.l.wrapsExpression[n.Type]:
			// Count the wrapper as error flow, then keep walking: the
			// expression inside is still a statement.
			acc[ErrorFlow] = append(acc[ErrorFlow], n.Type+":"+c.hash(n, nil))
			c.transparent[n] = true
		case c.l.errorFlow(n):
			acc[ErrorFlow] = append(acc[ErrorFlow], c.hash(n, nil))
			mask[n] = true
			return
		case n.Field == "condition" || (parent != nil && c.l.switches[parent.Type] && (n.Field == "value" || n.Field == "subject" || n.Field == "condition")):
			// Conditions may hold lambdas with their own error flow; the
			// whole predicate is one condition.
			acc[Condition] = append(acc[Condition], c.hash(n, nil))
			mask[n] = true
			return
		}
		for _, ch := range n.Children {
			walk(ch, n)
		}
	}
	walk(b, nil)
	acc[Statement] = append(acc[Statement], c.hash(b, mask))
}

// typeBody splits a type's own body (members excluded): embedded types and
// mixins are supertypes, the rest is statements.
func (c *canon) typeBody(b *model.Node, acc map[Part][]string) {
	mask := map[*model.Node]bool{}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || c.skip[n] {
			return
		}
		if c.l.embedded(n) {
			acc[Supertypes] = append(acc[Supertypes], c.hash(n, nil))
			mask[n] = true
			return
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(b)
	acc[Statement] = append(acc[Statement], c.hash(b, mask))
}
