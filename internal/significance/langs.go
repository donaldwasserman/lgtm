package significance

import (
	"sort"
	"strings"

	"github.com/donaldwasserman/lgtm/internal/model"
)

// lang is one language's view of a declaration's parts.
type lang struct {
	// identTypes are leaf types that name a variable.
	identTypes map[string]bool
	// signature: fields (and node types) that make up a callable's or a
	// type's signature.
	signature, signatureTypes map[string]bool
	// supertypes: fields and node types naming what a type extends.
	supertypeFields, supertypeTypes map[string]bool
	// modifierTypes hold visibility keywords among other modifiers.
	modifierTypes   map[string]bool
	visibilityWords map[string]bool
	// bodyTypes are the node types of a type's own body when it sits in a
	// "type" field (Go struct and interface types).
	bodyTypes map[string]bool
	// bindings: parent node type -> child fields whose identifiers are bound
	// names. "*" binds every child.
	bindings map[string]map[string]bool
	// valueFields are never bindings, even under a binding parent.
	valueFields map[string]bool
	// switches carry their predicate in a value or subject field.
	switches map[string]bool
	// errorTypes are error-flow nodes; errorCalls are error-flow callees.
	errorTypes, errorCalls map[string]bool
	// wrapsExpression are error-flow nodes around an expression (Rust's ?).
	wrapsExpression map[string]bool
	// embedded reports a supertype written inside a type's body: a Go
	// embedded field, a Ruby include.
	embedded func(n *model.Node) bool
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func binds(pairs ...string) map[string]map[string]bool {
	m := map[string]map[string]bool{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if m[pairs[i]] == nil {
			m[pairs[i]] = map[string]bool{}
		}
		m[pairs[i]][pairs[i+1]] = true
	}
	return m
}

func (l *lang) binds(parentType, childField string) bool {
	f := l.bindings[parentType]
	return f != nil && (f["*"] || (childField != "" && f[childField]))
}

func (l *lang) errorFlow(n *model.Node) bool {
	if l.errorTypes[n.Type] {
		return true
	}
	return n.Callee != "" && l.errorCalls[n.Callee] && len(n.Children) > 0
}

func (l *lang) splitModifiers(n *model.Node) (vis, other string) {
	if l.visibilityWords == nil {
		return n.Text, ""
	}
	var v, o []string
	for _, w := range strings.Fields(n.Text) {
		if l.visibilityWords[w] {
			v = append(v, w)
		} else {
			o = append(o, w)
		}
	}
	sort.Strings(v)
	sort.Strings(o)
	return strings.Join(v, " "), strings.Join(o, " ")
}

func never(*model.Node) bool { return false }

var valueFields = set("value", "right", "type", "default", "return_type")

var langs = map[string]*lang{
	"go": {
		identTypes:      set("identifier"),
		signature:       set("parameters", "result", "type_parameters", "receiver"),
		signatureTypes:  set(),
		supertypeFields: set(),
		supertypeTypes:  set(),
		modifierTypes:   set(),
		bodyTypes:       set("struct_type", "interface_type"),
		bindings: binds("parameter_declaration", "name", "variadic_parameter_declaration", "name",
			"short_var_declaration", "left", "var_spec", "name", "const_spec", "name",
			"range_clause", "left", "receive_statement", "left", "type_switch_statement", "alias"),
		valueFields: valueFields,
		switches:    set("expression_switch_statement", "type_switch_statement"),
		errorTypes:  set("defer_statement"),
		errorCalls:  set("panic", "recover"),
		embedded: func(n *model.Node) bool {
			// struct { Base }: a field with a type and no name.
			if n.Type == "field_declaration" {
				for _, c := range n.Children {
					if c.Field == "name" {
						return false
					}
				}
				return true
			}
			return n.Type == "type_elem" // interface { Other }
		},
	},
	"python": {
		identTypes:      set("identifier"),
		signature:       set("parameters", "return_type", "type_parameters"),
		signatureTypes:  set(),
		supertypeFields: set("superclasses"),
		supertypeTypes:  set(),
		modifierTypes:   set(),
		bodyTypes:       set(),
		bindings: binds("parameters", "*", "lambda_parameters", "*", "typed_parameter", "*",
			"default_parameter", "name", "typed_default_parameter", "name",
			"list_splat_pattern", "*", "dictionary_splat_pattern", "*",
			"assignment", "left", "for_statement", "left", "for_in_clause", "left",
			"as_pattern", "alias", "named_expression", "name", "pattern_list", "*", "tuple_pattern", "*"),
		valueFields: valueFields,
		switches:    set("match_statement"),
		errorTypes:  set("except_clause", "except_group_clause", "finally_clause", "raise_statement"),
		errorCalls:  set(),
		embedded:    never,
	},
	"ruby": {
		identTypes:      set("identifier"),
		signature:       set("parameters"),
		signatureTypes:  set(),
		supertypeFields: set("superclass"),
		supertypeTypes:  set(),
		modifierTypes:   set(),
		bodyTypes:       set(),
		bindings: binds("method_parameters", "*", "block_parameters", "*", "lambda_parameters", "*",
			"optional_parameter", "name", "keyword_parameter", "name", "splat_parameter", "name",
			"block_parameter", "name", "hash_splat_parameter", "name",
			"assignment", "left", "for", "pattern", "exception_variable", "*", "left_assignment_list", "*"),
		valueFields: valueFields,
		switches:    set("case", "case_match"),
		errorTypes:  set("rescue", "ensure", "rescue_modifier"),
		errorCalls:  set("raise", "fail"),
		embedded: func(n *model.Node) bool {
			if n.Type != "call" {
				return false
			}
			for _, c := range n.Children {
				if c.Field == "method" {
					return c.Text == "include" || c.Text == "extend" || c.Text == "prepend"
				}
			}
			return false
		},
	},
	"javascript": jsLang,
	"typescript": jsLang,
	"tsx":        jsLang,
	"java": {
		identTypes:      set("identifier"),
		signature:       set("parameters", "type", "type_parameters", "dimensions"),
		signatureTypes:  set("throws"),
		supertypeFields: set("superclass", "interfaces"),
		supertypeTypes:  set("extends_interfaces", "super_interfaces"),
		modifierTypes:   set("modifiers"),
		visibilityWords: set("public", "private", "protected"),
		bodyTypes:       set(),
		bindings: binds("formal_parameter", "name", "spread_parameter", "*", "variable_declarator", "name",
			"catch_formal_parameter", "name", "enhanced_for_statement", "name",
			"lambda_expression", "parameters", "inferred_parameters", "*", "resource", "name"),
		valueFields: valueFields,
		switches:    set(),
		errorTypes:  set("catch_clause", "finally_clause", "throw_statement"),
		errorCalls:  set(),
		embedded:    never,
	},
	"rust": {
		identTypes:      set("identifier"),
		signature:       set("parameters", "return_type", "type_parameters"),
		signatureTypes:  set("where_clause"),
		supertypeFields: set("bounds"),
		supertypeTypes:  set(),
		modifierTypes:   set("visibility_modifier"),
		visibilityWords: nil, // the whole modifier is visibility
		bodyTypes:       set(),
		bindings: binds("parameter", "pattern", "let_declaration", "pattern", "for_expression", "pattern",
			"closure_parameters", "*", "match_arm", "pattern", "let_condition", "pattern",
			"tuple_pattern", "*", "match_pattern", "*", "ref_pattern", "*", "mut_pattern", "*"),
		valueFields:     valueFields,
		switches:        set("match_expression"),
		errorTypes:      set(),
		wrapsExpression: set("try_expression"),
		errorCalls:      set(),
		embedded:        never,
	},
}

var jsLang = &lang{
	identTypes:      set("identifier", "shorthand_property_identifier_pattern"),
	signature:       set("parameters", "parameter", "return_type", "type_parameters"),
	signatureTypes:  set(),
	supertypeFields: set(),
	supertypeTypes:  set("class_heritage", "extends_type_clause"),
	modifierTypes:   set("accessibility_modifier"),
	visibilityWords: set("public", "private", "protected"),
	bodyTypes:       set(),
	bindings: binds("formal_parameters", "*", "required_parameter", "pattern", "optional_parameter", "pattern",
		"rest_pattern", "*", "assignment_pattern", "left", "object_pattern", "*", "array_pattern", "*",
		"pair_pattern", "value", "variable_declarator", "name", "for_in_statement", "left",
		"catch_clause", "parameter", "arrow_function", "parameter"),
	valueFields: set("right", "type", "default", "return_type"),
	switches:    set("switch_statement"),
	errorTypes:  set("catch_clause", "finally_clause", "throw_statement"),
	errorCalls:  set(),
	embedded:    never,
}
