// Package parse scans source trees with Tree-sitter and produces
// language-agnostic named-node ASTs (internal/model).
package parse

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	tsx "github.com/smacker/go-tree-sitter/typescript/tsx"
	ts "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// TypeScript and TSX share node-type semantics but need different grammars:
// the plain TypeScript grammar treats JSX as a syntax error, so .tsx files must
// be parsed with the TSX grammar. The maps below are read-only and shared.
var (
	tsNameFields = map[string]string{
		"function_declaration":           "name",
		"method_definition":              "name",
		"class_declaration":              "name",
		"abstract_class_declaration":     "name",
		"generator_function_declaration": "name",
	}
	tsCallTypes = map[string]bool{"call_expression": true}
)

// Spec describes a supported language: detection by extension and the
// Tree-sitter node-type semantics needed for naming and call-depth analysis.
type Spec struct {
	ID      string
	GetLang func() *sitter.Language
	// Exts maps file extension (without leading dot) to this language.
	Exts map[string]bool
	// nameField maps a definition node type to the child field holding its
	// identifier (used as the stable alignment key and for dedup).
	nameField map[string]string
	// callTypes are node types representing a call (callee identifier
	// implies a call edge for intra-file call depth).
	callTypes map[string]bool
}

var specs = []*Spec{
	{
		ID:      "go",
		GetLang: golang.GetLanguage,
		Exts:    map[string]bool{"go": true},
		nameField: map[string]string{
			"function_declaration": "name",
			"method_declaration":   "name",
			"type_declaration":     "name",
		},
		callTypes: map[string]bool{
			"call_expression": true,
		},
	},
	{
		ID:      "python",
		GetLang: python.GetLanguage,
		Exts:    map[string]bool{"py": true},
		nameField: map[string]string{
			"function_definition":  "name",
			"class_definition":     "name",
			"decorated_definition": "definition",
		},
		callTypes: map[string]bool{
			"call": true,
		},
	},
	{
		ID:      "ruby",
		GetLang: ruby.GetLanguage,
		Exts:    map[string]bool{"rb": true},
		nameField: map[string]string{
			"method":           "name",
			"singleton_method": "name",
			"class":            "name",
			"module":           "name",
			"call":             "method",
		},
		callTypes: map[string]bool{
			"call": true,
		},
	},
	{
		ID:      "javascript",
		GetLang: javascript.GetLanguage,
		Exts:    map[string]bool{"js": true, "mjs": true, "cjs": true, "jsx": true},
		nameField: map[string]string{
			"function_declaration":           "name",
			"method_definition":              "name",
			"class_declaration":              "name",
			"generator_function_declaration": "name",
		},
		callTypes: map[string]bool{
			"call_expression": true,
		},
	},
	{
		ID:        "typescript",
		GetLang:   ts.GetLanguage,
		Exts:      map[string]bool{"ts": true, "mts": true, "cts": true},
		nameField: tsNameFields,
		callTypes: tsCallTypes,
	},
	{
		ID:        "tsx",
		GetLang:   tsx.GetLanguage,
		Exts:      map[string]bool{"tsx": true},
		nameField: tsNameFields,
		callTypes: tsCallTypes,
	},
	{
		ID:      "java",
		GetLang: java.GetLanguage,
		Exts:    map[string]bool{"java": true},
		nameField: map[string]string{
			"class_declaration":           "name",
			"interface_declaration":       "name",
			"enum_declaration":            "name",
			"annotation_type_declaration": "name",
			"method_declaration":          "name",
			"record_declaration":          "name",
		},
		callTypes: map[string]bool{
			"method_invocation": true,
		},
	},
}

// LanguageFor returns the spec for a file path by extension, or nil if the
// file is not a supported source file.
func LanguageFor(path string) *Spec {
	e := ext(path)
	for _, s := range specs {
		if s.Exts[e] {
			return s
		}
	}
	return nil
}

func ext(path string) string {
	dot := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			break
		}
		if path[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return ""
	}
	return path[dot+1:]
}

// IsCall reports whether a node type represents a call expression.
func (s *Spec) IsCall(typ string) bool { return s.callTypes[typ] }
