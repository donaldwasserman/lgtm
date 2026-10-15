package parse

import "regexp"

// Workaround for tree-sitter-typescript#367: the grammar misparses an inline
// import type used as a call's type argument, e.g.
//
//	await importOriginal<typeof import("react-router-dom")>()
//	await vi.importActual<typeof import("./x")>("./x")
//
// The first is a syntax error; the second silently parses as two comparisons.
// typeof <identifier> in the same position parses fine, so before parsing we
// overwrite the import("...") operand with an identifier of the same length.
// Byte offsets are unchanged, and build reads node text from the original
// source, so only the operand's node type differs (identifier, not
// call_expression).
//
// Remove once https://github.com/tree-sitter/tree-sitter-typescript/pull/375
// is released and the vendored TypeScript/TSX grammars include it.
var tsTypeofImport = regexp.MustCompile(
	`\btypeof\s+(import[ \t]*\([ \t]*(?:"[^"\n]*"|'[^'\n]*')[ \t]*\))`)

// maskTypeofImports returns src with each typeof import("...") operand
// replaced by underscores, or src itself when there is none.
func maskTypeofImports(src []byte) []byte {
	locs := tsTypeofImport.FindAllSubmatchIndex(src, -1)
	if locs == nil {
		return src
	}
	out := append([]byte(nil), src...)
	for _, loc := range locs {
		for i := loc[2]; i < loc[3]; i++ {
			out[i] = '_'
		}
	}
	return out
}
