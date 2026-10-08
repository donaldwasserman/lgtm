package symbols

import (
	"path"
	"strings"
	"unicode"

	"github.com/donaldwasserman/lgtm/internal/model"
)

// extractors finds the top-level and member symbols of one file, by language
// id (internal/parse). Function bodies are never searched: a nested function
// belongs to its enclosing one.
var extractors = map[string]func(*model.File) []*Symbol{
	"go":         goSymbols,
	"python":     pythonSymbols,
	"ruby":       rubySymbols,
	"javascript": jsSymbols,
	"typescript": jsSymbols,
	"tsx":        jsSymbols,
	"java":       javaSymbols,
	"rust":       rustSymbols,
}

func newSymbol(f *model.File, module, container, name string, kind Kind, exported bool, n *model.Node) *Symbol {
	return &Symbol{Module: module, Container: container, Name: name, Kind: kind,
		Exported: exported, File: f.Path, Lang: f.Lang, Node: n}
}

func join(container, name string) string {
	if container == "" {
		return name
	}
	return container + "." + name
}

func stripExt(p string) string {
	if e := path.Ext(p); e != "" {
		return strings.TrimSuffix(p, e)
	}
	return p
}

// ---- Go ----
//
// Module: the package directory. Exported: the name is capitalized.

func goSymbols(f *model.File) []*Symbol {
	mod := path.Dir(toSlash(f.Path))
	var out []*Symbol
	for _, n := range f.Root.Children {
		switch n.Type {
		case "function_declaration":
			name := fieldText(n, "name")
			out = append(out, newSymbol(f, mod, "", name, KindFunc, goExported(name), n))
		case "method_declaration":
			name := fieldText(n, "name")
			recv := goReceiverType(field(n, "receiver"))
			out = append(out, newSymbol(f, mod, recv, name, KindMethod, goExported(name), n))
		case "type_declaration":
			for _, spec := range n.Children {
				if spec.Type != "type_spec" && spec.Type != "type_alias" {
					continue
				}
				name := fieldText(spec, "name")
				t := newSymbol(f, mod, "", name, KindType, goExported(name), spec)
				// Interface methods are members of the interface.
				if it := field(spec, "type"); it != nil && it.Type == "interface_type" {
					for _, m := range it.Children {
						if m.Type == "method_elem" {
							mn := fieldText(m, "name")
							t.Members = append(t.Members,
								newSymbol(f, mod, name, mn, KindMethod, t.Exported && goExported(mn), m))
						}
					}
				}
				out = append(out, t)
			}
		}
	}
	return out
}

func goExported(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}

// goReceiverType reads T from receivers like (t T), (t *T), (t *T[K]).
func goReceiverType(recv *model.Node) string {
	if recv == nil {
		return ""
	}
	var find func(n *model.Node) string
	find = func(n *model.Node) string {
		if n.Type == "type_identifier" {
			return n.Text
		}
		for _, c := range n.Children {
			if c.Field == "name" && n.Type == "parameter_declaration" {
				continue // the receiver variable, not its type
			}
			if t := find(c); t != "" {
				return t
			}
		}
		return ""
	}
	return find(recv)
}

// ---- Python ----
//
// Module: the dotted module path. Exported: no leading underscore (dunder
// names are public), and named in __all__ when the module defines it.

func pythonSymbols(f *model.File) []*Symbol {
	mod := strings.ReplaceAll(stripExt(toSlash(f.Path)), "/", ".")
	mod = strings.TrimSuffix(mod, ".__init__")
	all := pythonAll(f.Root)
	var walk func(body *model.Node, container string, containerExported bool) []*Symbol
	walk = func(body *model.Node, container string, containerExported bool) []*Symbol {
		var out []*Symbol
		for _, n := range body.Children {
			decl, wrapper := n, (*model.Node)(nil)
			if n.Type == "decorated_definition" {
				decl, wrapper = field(n, "definition"), n
				if decl == nil {
					continue
				}
			}
			name := fieldText(decl, "name")
			exported := containerExported && pythonPublic(name)
			if container == "" && all != nil {
				exported = all[name]
			}
			switch decl.Type {
			case "function_definition":
				kind := KindFunc
				if container != "" {
					kind = KindMethod
				}
				s := newSymbol(f, mod, container, name, kind, exported, decl)
				s.wrapper = wrapper
				out = append(out, s)
			case "class_definition":
				s := newSymbol(f, mod, container, name, KindType, exported, decl)
				s.wrapper = wrapper
				if b := field(decl, "body"); b != nil {
					s.Members = walk(b, join(container, name), exported)
				}
				out = append(out, s)
			}
		}
		return out
	}
	return walk(f.Root, "", true)
}

func pythonPublic(name string) bool {
	if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return true
	}
	return !strings.HasPrefix(name, "_")
}

// pythonAll reads a module-level `__all__ = [...]` of string literals, or
// returns nil when there is none.
func pythonAll(root *model.Node) map[string]bool {
	for _, st := range root.Children {
		if st.Type != "expression_statement" || len(st.Children) == 0 {
			continue
		}
		a := st.Children[0]
		if a.Type != "assignment" || fieldText(a, "left") != "__all__" {
			continue
		}
		names := map[string]bool{}
		if r := field(a, "right"); r != nil {
			for _, e := range r.Children {
				if e.Type == "string" {
					names[strings.Trim(e.Text, `"'`)] = true
				}
			}
		}
		return names
	}
	return nil
}

// ---- Ruby ----
//
// Module: the constant path of the enclosing classes and modules, so a class
// reopened in another file is the same module. Top-level methods live on
// Object and are callable everywhere. Exported: public visibility, tracking
// `private`/`protected`/`public` sections, `private :name` and `private def`.

func rubySymbols(f *model.File) []*Symbol {
	var walk func(body *model.Node, mod string, modExported bool) []*Symbol
	walk = func(body *model.Node, mod string, modExported bool) []*Symbol {
		var out []*Symbol
		visibility := "public"
		byName := map[string]*Symbol{}
		addMethod := func(n *model.Node, vis string) {
			name := fieldText(n, "name")
			if n.Type == "singleton_method" {
				name = "self." + name
				vis = "public" // section keywords do not apply to def self.x
			}
			s := newSymbol(f, moduleOrMain(mod), "", name, KindMethod,
				modExported && vis == "public", n)
			byName[name] = s
			out = append(out, s)
		}
		for _, n := range body.Children {
			switch n.Type {
			case "class", "module":
				name := fieldText(n, "name")
				kind := KindType
				if n.Type == "module" {
					kind = KindModule
				}
				s := newSymbol(f, moduleOrMain(mod), "", name, kind, modExported, n)
				if b := field(n, "body"); b != nil {
					s.Members = walk(b, joinRuby(mod, name), modExported)
				}
				out = append(out, s)
			case "method", "singleton_method":
				addMethod(n, visibility)
			case "identifier":
				switch n.Text {
				case "private", "protected", "public":
					visibility = n.Text
				}
			case "call":
				method := fieldText(n, "method")
				if method != "private" && method != "protected" && method != "public" {
					continue
				}
				args := field(n, "arguments")
				if args == nil {
					visibility = method
					continue
				}
				for _, a := range args.Children {
					switch a.Type {
					case "method", "singleton_method": // private def x
						addMethod(a, method)
					case "simple_symbol": // private :x
						if s := byName[strings.TrimPrefix(a.Text, ":")]; s != nil {
							s.Exported = modExported && method == "public"
						}
					}
				}
			}
		}
		return out
	}
	return walk(f.Root, "", true)
}

func moduleOrMain(mod string) string {
	if mod == "" {
		return "Object"
	}
	return mod
}

func joinRuby(mod, name string) string {
	if strings.HasPrefix(name, "::") {
		return strings.TrimPrefix(name, "::")
	}
	if mod == "" {
		return name
	}
	return mod + "::" + name
}

// ---- JavaScript / TypeScript ----
//
// Module: the file. Exported: declared under `export`, named in an
// `export { ... }` clause, or assigned to module.exports / exports.x.
// Class members are exported with their class unless private (#name, or a
// TypeScript private/protected modifier).

func jsSymbols(f *model.File) []*Symbol {
	mod := stripExt(toSlash(f.Path))
	exportedNames := jsExportedNames(f.Root)
	var out []*Symbol
	for _, n := range f.Root.Children {
		decl, wrapper, exported := n, (*model.Node)(nil), false
		if n.Type == "export_statement" {
			exported, wrapper = true, n
			decl = field(n, "declaration")
			if decl == nil {
				decl = field(n, "value") // export default <expression>
			}
			if decl == nil {
				continue
			}
		}
		for _, s := range jsDeclaration(f, mod, decl, exported, exportedNames) {
			s.wrapper = wrapper
			out = append(out, s)
		}
		// CommonJS: exports.x = function () {}, module.exports.x = ...
		if n.Type == "expression_statement" && len(n.Children) == 1 &&
			n.Children[0].Type == "assignment_expression" {
			a := n.Children[0]
			left, right := field(a, "left"), field(a, "right")
			if left != nil && right != nil && jsIsFunction(right.Type) {
				if name, ok := cjsExportTarget(left.Text); ok {
					out = append(out, newSymbol(f, mod, "", name, KindFunc, true, right))
				}
			}
		}
	}
	return out
}

func jsDeclaration(f *model.File, mod string, decl *model.Node, exported bool, names map[string]bool) []*Symbol {
	name := fieldText(decl, "name")
	if exported && name == "" {
		name = "default"
	}
	exp := exported || names[name]
	switch decl.Type {
	case "function_declaration", "generator_function_declaration", "function_signature":
		return []*Symbol{newSymbol(f, mod, "", name, KindFunc, exp, decl)}
	case "class_declaration", "abstract_class_declaration", "class":
		s := newSymbol(f, mod, "", name, KindType, exp, decl)
		if b := field(decl, "body"); b != nil {
			for _, m := range b.Children {
				switch m.Type {
				case "method_definition", "abstract_method_signature", "method_signature":
					mn := fieldText(m, "name")
					s.Members = append(s.Members,
						newSymbol(f, mod, name, mn, KindMethod, exp && jsMemberPublic(m, mn), m))
				}
			}
		}
		return []*Symbol{s}
	case "interface_declaration":
		s := newSymbol(f, mod, "", name, KindType, exp, decl)
		if b := field(decl, "body"); b != nil {
			for _, m := range b.Children {
				if m.Type == "method_signature" {
					mn := fieldText(m, "name")
					s.Members = append(s.Members, newSymbol(f, mod, name, mn, KindMethod, exp, m))
				}
			}
		}
		return []*Symbol{s}
	case "type_alias_declaration", "enum_declaration":
		return []*Symbol{newSymbol(f, mod, "", name, KindType, exp, decl)}
	case "lexical_declaration", "variable_declaration":
		// const f = () => {}, const C = class {}
		var out []*Symbol
		for _, d := range decl.Children {
			if d.Type != "variable_declarator" {
				continue
			}
			v := field(d, "value")
			if v == nil || !(jsIsFunction(v.Type) || v.Type == "class") {
				continue
			}
			dn := fieldText(d, "name")
			kind := KindFunc
			if v.Type == "class" {
				kind = KindType
			}
			out = append(out, newSymbol(f, mod, "", dn, kind, exported || names[dn], v))
		}
		return out
	}
	return nil
}

func jsIsFunction(typ string) bool {
	switch typ {
	case "arrow_function", "function_expression", "function", "generator_function":
		return true
	}
	return false
}

func jsMemberPublic(m *model.Node, name string) bool {
	if strings.HasPrefix(name, "#") {
		return false
	}
	for _, c := range m.Children {
		if c.Type == "accessibility_modifier" && c.Text != "public" {
			return false
		}
	}
	return true
}

// jsExportedNames collects names exported after their declaration:
// `export { a, b as c }`, `export default a`, `module.exports = { a, b }`
// and `module.exports = a`.
func jsExportedNames(root *model.Node) map[string]bool {
	names := map[string]bool{}
	for _, n := range root.Children {
		switch n.Type {
		case "export_statement":
			for _, c := range n.Children {
				if c.Type == "export_clause" {
					for _, sp := range c.Children {
						if nm := fieldText(sp, "name"); nm != "" {
							names[nm] = true
						}
					}
				}
				if c.Field == "value" && c.Type == "identifier" {
					names[c.Text] = true
				}
			}
		case "expression_statement":
			if len(n.Children) != 1 || n.Children[0].Type != "assignment_expression" {
				continue
			}
			a := n.Children[0]
			if fieldText(a, "left") != "module.exports" {
				continue
			}
			r := field(a, "right")
			if r == nil {
				continue
			}
			switch r.Type {
			case "identifier":
				names[r.Text] = true
			case "object":
				for _, p := range r.Children {
					switch p.Type {
					case "shorthand_property_identifier":
						names[p.Text] = true
					case "pair":
						if v := field(p, "value"); v != nil && v.Type == "identifier" {
							names[v.Text] = true
						}
					}
				}
			}
		}
	}
	return names
}

// cjsExportTarget reads x from `exports.x` or `module.exports.x`.
func cjsExportTarget(left string) (string, bool) {
	for _, prefix := range []string{"module.exports.", "exports."} {
		if strings.HasPrefix(left, prefix) {
			name := strings.TrimPrefix(left, prefix)
			if name != "" && !strings.ContainsAny(name, ".[ ") {
				return name, true
			}
		}
	}
	return "", false
}

// ---- Java ----
//
// Module: the package. Exported: public or protected, and interface members,
// within an exported container.

func javaSymbols(f *model.File) []*Symbol {
	mod := packageOf(f)
	var walk func(body *model.Node, container string, containerExported, inInterface bool) []*Symbol
	walk = func(body *model.Node, container string, containerExported, inInterface bool) []*Symbol {
		var out []*Symbol
		for _, n := range body.Children {
			name := fieldText(n, "name")
			exported := containerExported && (inInterface || javaVisible(n))
			switch n.Type {
			case "class_declaration", "interface_declaration", "enum_declaration",
				"record_declaration", "annotation_type_declaration":
				s := newSymbol(f, mod, container, name, KindType, exported, n)
				if b := field(n, "body"); b != nil {
					s.Members = walk(b, join(container, name), exported,
						n.Type == "interface_declaration" || n.Type == "annotation_type_declaration")
				}
				out = append(out, s)
			case "method_declaration", "constructor_declaration", "compact_constructor_declaration":
				out = append(out, newSymbol(f, mod, container, name, KindMethod, exported, n))
			case "enum_body_declarations":
				out = append(out, walk(n, container, containerExported, inInterface)...)
			}
		}
		return out
	}
	return walk(f.Root, "", true, false)
}

func javaVisible(n *model.Node) bool {
	for _, c := range n.Children {
		if c.Type == "modifiers" {
			for _, w := range strings.Fields(c.Text) {
				if w == "public" || w == "protected" {
					return true
				}
			}
		}
	}
	return false
}

// ---- Rust ----
//
// Module: the module path from the file (src/lib.rs and src/main.rs are the
// crate root; src/a/b.rs and src/a/b/mod.rs are a::b) plus inline `mod`
// blocks. Exported: any `pub` visibility, inside public modules. Methods in
// an impl block are members of its type.

func rustSymbols(f *model.File) []*Symbol {
	var walk func(body *model.Node, mod string, modExported bool) []*Symbol
	walk = func(body *model.Node, mod string, modExported bool) []*Symbol {
		var out []*Symbol
		for _, n := range body.Children {
			name := fieldText(n, "name")
			exported := modExported && rustPub(n)
			switch n.Type {
			case "function_item", "function_signature_item":
				out = append(out, newSymbol(f, mod, "", name, KindFunc, exported, n))
			case "struct_item", "enum_item", "union_item", "type_item":
				out = append(out, newSymbol(f, mod, "", name, KindType, exported, n))
			case "trait_item":
				s := newSymbol(f, mod, "", name, KindType, exported, n)
				if b := field(n, "body"); b != nil {
					for _, m := range b.Children {
						if m.Type == "function_item" || m.Type == "function_signature_item" {
							s.Members = append(s.Members, newSymbol(f, mod, name,
								fieldText(m, "name"), KindMethod, exported, m))
						}
					}
				}
				out = append(out, s)
			case "mod_item":
				s := newSymbol(f, mod, "", name, KindModule, exported, n)
				if b := field(n, "body"); b != nil {
					s.Members = walk(b, joinRust(mod, name), exported)
				}
				out = append(out, s)
			case "impl_item":
				typ := rustTypeName(field(n, "type"))
				trait := field(n, "trait") != nil
				if b := field(n, "body"); b != nil {
					for _, m := range b.Children {
						if m.Type != "function_item" {
							continue
						}
						// A trait method is as visible as the type; an
						// inherent method carries its own `pub`.
						exp := modExported && (trait || rustPub(m))
						out = append(out, newSymbol(f, mod, typ, fieldText(m, "name"), KindMethod, exp, m))
					}
				}
			}
		}
		return out
	}
	return walk(f.Root, rustFileModule(f.Path), true)
}

func rustPub(n *model.Node) bool {
	for _, c := range n.Children {
		if c.Type == "visibility_modifier" {
			return true
		}
	}
	return false
}

func rustTypeName(t *model.Node) string {
	if t == nil {
		return ""
	}
	if t.Type == "generic_type" {
		return fieldText(t, "type")
	}
	return t.Text
}

func joinRust(mod, name string) string {
	if mod == "" {
		return name
	}
	return mod + "::" + name
}

func rustFileModule(p string) string {
	p = stripExt(toSlash(p))
	if i := strings.LastIndex(p, "src/"); i >= 0 {
		p = p[i+len("src/"):]
	}
	p = strings.TrimSuffix(p, "/mod")
	if p == "lib" || p == "main" {
		return ""
	}
	return strings.ReplaceAll(p, "/", "::")
}
