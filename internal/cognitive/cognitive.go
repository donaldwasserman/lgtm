// Package cognitive computes G. Ann Campbell's Cognitive Complexity
// (SonarSource, "Cognitive Complexity: a new way of measuring
// understandability", version 1.7) for one function, and the two scores built
// on it: the largest increase among existing functions, and the largest
// complexity among new ones.
//
// The rules, applied in one pass over the function body:
//
//   - +1 plus the current nesting level for each if, ternary, switch/match,
//     loop and catch/rescue/except. Each also raises nesting for its body.
//   - +1 with no nesting penalty for else if / elif / elsif and else, and
//     for goto and labeled break/continue.
//   - +1 once if the function calls itself ("each method in a recursion
//     cycle"), however many self-calls it makes. Only direct recursion is
//     seen: a cycle through another function is not.
//   - +1 for each run of like boolean operators: a && b && c || d scores 2.
//   - Nested functions and lambdas raise nesting but score nothing.
//
// Deliberate choices where implementations differ:
//
//   - Boolean runs are read through parentheses: (a || b) && c scores 2,
//     a && (b && c) scores 1.
//   - Null-coalescing (??) and optional chaining (?.) score nothing.
//   - Python comprehension conditions and Rust's ? operator score nothing.
//   - A Ruby statement modifier (x if y, x while y) scores like the
//     statement it abbreviates.
//   - Recursion counts once per function and includes a method calling
//     itself through its receiver (self.f, this.f, Go's r.f). gocognit, the
//     Go reference used in tests, counts each bare f(...) call instead and
//     ignores receiver calls; the two agree on every function that does not
//     call itself.
package cognitive

import (
	"strings"

	"github.com/donaldwasserman/lgtm/internal/model"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// table is one language's node types for the rules above.
type table struct {
	// ifs are if-like nodes. An if in else position scores as else-if.
	// Python's elif_clause and Ruby's elsif are listed here too: they are
	// always in else position.
	ifs map[string]bool
	// structural nodes other than ifs: loops, switches, catches, ternaries.
	structural map[string]bool
	// lambdas raise nesting without scoring.
	lambdas map[string]bool
	// boolNodes are binary nodes that may carry a boolean operator.
	boolNodes map[string]bool
	// jumps score +1 when they carry a label: a child of a labels type.
	jumps  map[string]bool
	labels map[string]bool
	// alwaysJump scores +1 unconditionally (goto).
	alwaysJump map[string]bool
	// parens are wrappers a boolean run is read through.
	parens map[string]bool
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

var cStyleBool = set("&&", "||")

var tables = map[string]*table{
	"go": {
		ifs:        set("if_statement"),
		structural: set("for_statement", "expression_switch_statement", "type_switch_statement", "select_statement"),
		lambdas:    set("func_literal"),
		boolNodes:  set("binary_expression"),
		jumps:      set("break_statement", "continue_statement"),
		labels:     set("label_name"),
		alwaysJump: set("goto_statement"),
		parens:     set("parenthesized_expression"),
	},
	"python": {
		ifs: set("if_statement", "elif_clause"),
		structural: set("for_statement", "while_statement", "except_clause", "except_group_clause",
			"conditional_expression", "match_statement"),
		lambdas:   set("lambda", "function_definition", "class_definition"),
		boolNodes: set("boolean_operator"),
		parens:    set("parenthesized_expression"),
	},
	"ruby": {
		ifs: set("if", "unless", "elsif", "if_modifier", "unless_modifier"),
		structural: set("while", "until", "for", "while_modifier", "until_modifier",
			"case", "case_match", "rescue", "rescue_modifier", "conditional"),
		lambdas:   set("block", "do_block", "lambda", "method", "singleton_method"),
		boolNodes: set("binary"),
		parens:    set("parenthesized_statements"),
	},
	"javascript": jsTable,
	"typescript": jsTable,
	"tsx":        jsTable,
	"java": {
		ifs: set("if_statement"),
		structural: set("for_statement", "enhanced_for_statement", "while_statement", "do_statement",
			"switch_expression", "switch_statement", "catch_clause", "ternary_expression"),
		lambdas:   set("lambda_expression", "class_body"),
		boolNodes: set("binary_expression"),
		jumps:     set("break_statement", "continue_statement"),
		labels:    set("identifier"),
		parens:    set("parenthesized_expression"),
	},
	"rust": {
		ifs:        set("if_expression"),
		structural: set("for_expression", "while_expression", "loop_expression", "match_expression"),
		lambdas:    set("closure_expression", "function_item"),
		boolNodes:  set("binary_expression"),
		jumps:      set("break_expression", "continue_expression"),
		labels:     set("label"),
		parens:     set("parenthesized_expression"),
	},
}

var jsTable = &table{
	ifs: set("if_statement"),
	structural: set("for_statement", "for_in_statement", "while_statement", "do_statement",
		"switch_statement", "catch_clause", "ternary_expression"),
	lambdas: set("arrow_function", "function_expression", "function", "function_declaration",
		"generator_function", "generator_function_declaration", "class_body"),
	boolNodes: set("binary_expression"),
	jumps:     set("break_statement", "continue_statement"),
	labels:    set("statement_identifier"),
	parens:    set("parenthesized_expression"),
}

// Supported reports whether the measure is implemented for a language.
func Supported(lang string) bool { return tables[lang] != nil }

// headerFields are parts of a structure read before its body: they sit at
// the structure's own nesting level. Everything else is its body.
var headerFields = set("condition", "value", "left", "right", "initializer", "init",
	"update", "pattern", "iterator", "subject")

// Of returns the cognitive complexity of a callable symbol. ok is false when
// the language is unsupported or the symbol has no body.
func Of(s *symbols.Symbol) (score int, ok bool) {
	t := tables[s.Lang]
	body := symbols.Body(s.Node)
	if t == nil || body == nil {
		return 0, false
	}
	w := newWalker(s)
	w.visit(body, 0)
	return w.score, true
}

func newWalker(s *symbols.Symbol) *walker {
	name := s.Name
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:] // Ruby "self.x"
	}
	w := &walker{t: tables[s.Lang], name: name, selves: set("self", "this"),
		// In Go, Python, JS/TS and Rust a method calls itself only through
		// its receiver; a bare name(...) is a free function. Java and Ruby
		// methods call themselves bare.
		bareIsSelf: s.Kind != symbols.KindMethod || s.Lang == "java" || s.Lang == "ruby"}
	if s.Lang == "go" {
		if r := goReceiverVar(s.Node); r != "" {
			w.selves[r] = true
		}
	}
	return w
}

type walker struct {
	t      *table
	name   string          // the function's own name, for recursion
	selves map[string]bool // receivers that make name(...) a self-call
	// bareIsSelf: an unqualified name(...) calls this function.
	bareIsSelf bool
	recursive  bool
	score      int
}

func (w *walker) visit(n *model.Node, nest int) {
	if n == nil || symbols.IsComment(n.Type) {
		return
	}
	switch {
	case w.t.ifs[n.Type]:
		w.visitIf(n, nest, false)
		return
	case w.t.structural[n.Type]:
		w.score += 1 + nest
		for _, c := range n.Children {
			if headerFields[c.Field] {
				w.visit(c, nest)
			} else {
				w.visit(c, nest+1)
			}
		}
		return
	case w.t.lambdas[n.Type]:
		for _, c := range n.Children {
			w.visit(c, nest+1)
		}
		return
	case w.t.boolNodes[n.Type] && isBool(operator(n)):
		w.visitBool(n, nest)
		return
	case w.t.alwaysJump[n.Type]:
		w.score++
	case w.t.jumps[n.Type] && w.hasLabel(n):
		w.score++
	}
	if n.Callee == w.name && !w.recursive && w.selfCall(n) {
		w.recursive = true
		w.score++ // recursion: once per function, however many calls
	}
	for _, c := range n.Children {
		w.visit(c, nest)
	}
}

// visitIf scores an if and walks its else chain. An if in else position is
// an else-if: +1 with no nesting penalty, at the same nesting as the if it
// continues.
func (w *walker) visitIf(n *model.Node, nest int, elseIf bool) {
	if elseIf {
		w.score++
	} else {
		w.score += 1 + nest
	}
	for _, c := range n.Children {
		switch {
		case c.Field == "alternative":
			w.visitElse(c, nest)
		case headerFields[c.Field]:
			w.visit(c, nest)
		default:
			w.visit(c, nest+1)
		}
	}
}

// visitElse handles one alternative of an if: another if (Go, Java), an
// elif/elsif clause, an else_clause wrapping one if (JS, Rust), or a plain
// else. An if inside an else *block* - Go's else { if ... }, or Ruby's else
// holding an if statement - is genuinely nested and is not unwrapped.
func (w *walker) visitElse(alt *model.Node, nest int) {
	if w.t.ifs[alt.Type] {
		w.visitIf(alt, nest, true)
		return
	}
	var kids []*model.Node
	for _, c := range alt.Children {
		if !symbols.IsComment(c.Type) {
			kids = append(kids, c)
		}
	}
	if alt.Type == "else_clause" && len(kids) == 1 && w.t.ifs[kids[0].Type] {
		w.visitIf(kids[0], nest, true)
		return
	}
	w.score++ // else
	for _, c := range kids {
		w.visit(c, nest+1)
	}
}

// visitBool scores a boolean expression: +1 per run of like operators, read
// in source order through nested boolean nodes and parentheses. Operands are
// then walked for anything else they contain.
func (w *walker) visitBool(n *model.Node, nest int) {
	var ops []string
	var operands []*model.Node
	var flatten func(x *model.Node)
	flatten = func(x *model.Node) {
		if w.t.parens[x.Type] && len(x.Children) == 1 {
			inner := x.Children[0]
			if w.t.boolNodes[inner.Type] && isBool(operator(inner)) {
				flatten(inner)
				return
			}
		}
		if w.t.boolNodes[x.Type] {
			if op := operator(x); isBool(op) {
				l, r := operands2(x)
				if l != nil && r != nil {
					flatten(l)
					ops = append(ops, normalize(op))
					flatten(r)
					return
				}
			}
		}
		operands = append(operands, x)
	}
	flatten(n)
	for i, op := range ops {
		if i == 0 || op != ops[i-1] {
			w.score++
		}
	}
	for _, o := range operands {
		w.visit(o, nest)
	}
}

// operands2 returns a binary node's two operands: its left and right fields,
// or its first and last children.
func operands2(n *model.Node) (l, r *model.Node) {
	for _, c := range n.Children {
		switch c.Field {
		case "left":
			l = c
		case "right":
			r = c
		}
	}
	if l == nil && r == nil && len(n.Children) >= 2 {
		l, r = n.Children[0], n.Children[len(n.Children)-1]
	}
	return l, r
}

// operator reads a binary node's operator: the source text between its
// operands. Tree-sitter keeps operators as anonymous tokens, which the model
// drops.
func operator(n *model.Node) string {
	l, r := operands2(n)
	if l == nil || r == nil || l.End < n.Start || r.Start > n.End || l.End > r.Start {
		return ""
	}
	return strings.TrimSpace(n.Text[l.End-n.Start : r.Start-n.Start])
}

func isBool(op string) bool {
	switch op {
	case "&&", "||", "and", "or":
		return true
	}
	return false
}

// normalize maps Python/Ruby words to their symbols, so `and` and `&&` form
// one run in Ruby, where both exist.
func normalize(op string) string {
	switch op {
	case "and":
		return "&&"
	case "or":
		return "||"
	}
	return op
}

// selfCall reports whether a call node calls the enclosing function itself:
// f(...), or self.f / this.f / recv.f, but not pkg.f, which only shares the
// name.
func (w *walker) selfCall(call *model.Node) bool {
	target := ""
	for _, c := range call.Children {
		switch c.Field {
		case "function": // Go, JS/TS, Rust, Python
			target = c.Text
		case "object", "receiver": // Java, Ruby
			target = c.Text + "." + w.name
		}
	}
	if target == "" || target == w.name {
		return w.bareIsSelf
	}
	if i := strings.LastIndex(target, "."); i >= 0 && target[i+1:] == w.name {
		return w.selves[target[:i]]
	}
	return false
}

// goReceiverVar reads t from a Go method's receiver (t *T).
func goReceiverVar(fn *model.Node) string {
	recv := symbols.Field(fn, "receiver")
	if recv == nil {
		return ""
	}
	for _, p := range recv.Children {
		if name := symbols.Field(p, "name"); name != nil {
			return name.Text
		}
	}
	return ""
}

// hasLabel reports whether a break/continue names a label.
func (w *walker) hasLabel(n *model.Node) bool {
	for _, c := range n.Children {
		if w.t.labels[c.Type] {
			return true
		}
	}
	return false
}
