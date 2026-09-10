// Package model defines the language-agnostic AST and change representation
// used by the lgtm parser, differ, and metrics layers.
package model

import "strconv"

// Node is a serializable, language-agnostic syntax-tree node captured from a
// Tree-sitter parse. Only named nodes are retained (leaf tokens like commas
// are ignored) to reduce noise when aligning base versus head trees.
type Node struct {
	Type     string  `json:"type"`               // tree-sitter node type (e.g. "function_definition")
	Name     string  `json:"name,omitempty"`     // extracted identifier of a definition, if any
	Callee   string  `json:"callee,omitempty"`   // callee name for a call node, if any
	Text     string  `json:"text,omitempty"`     // source text of the node (for leaf equivalence)
	Start    uint32  `json:"startByte"`          // byte offset of node start
	End      uint32  `json:"endByte"`            // byte offset of node end
	Index    int     `json:"index"`              // position among the parent's retained children
	Nest     int     `json:"nest"`               // raw AST nesting depth (root = 0)
	Call     int     `json:"call,omitempty"`     // max intra-file call depth through this node
	Depth    int     `json:"depth"`              // Nest + Call (computed)
	Children []*Node `json:"children,omitempty"` // named children in source order
}

// Key returns a stable identity for alignment: the extracted name when
// present, falling back to the node's position among its parent's children.
//
// Byte offsets are deliberately not used here. Editing a single token shifts
// the offset of everything after it, which made untouched siblings fail to
// align and be reported as a delete plus an insert.
func (n *Node) Key() string {
	if n.Name != "" {
		return n.Type + ":" + n.Name
	}
	return n.Type + ":#" + strconv.Itoa(n.Index)
}

// File is one parsed source file.
type File struct {
	Path  string   `json:"path"`            // relative path within the tree
	Lang  string   `json:"language"`        // canonical language id (go, python, ...)
	Root  *Node    `json:"root"`            // root of the named-node tree
	Error *FileErr `json:"error,omitempty"` // parse failure, if any
}

// FileErr records that a file could not be parsed.
type FileErr struct {
	Msg string `json:"message"`
}
