// Package diff aligns base and head ASTs and classifies changes as inserts,
// deletes, or modifications.
package diff

import (
	"sort"

	"lgtm/internal/model"
)

// Result holds the file-level changes between a base and head scan.
type Result struct {
	Files []*model.FileChange
}

// Diff classifies node changes by aligning each base/head file pair by
// relative path, then aligning nodes by a stable key (type + extracted name,
// falling back to source byte position).
func Diff(base, head map[string]*model.File) *Result {
	res := &Result{}
	paths := unionKeys(base, head)
	for _, p := range paths {
		b := base[p]
		h := head[p]
		switch {
		case b == nil && h == nil:
			continue
		case b == nil: // only in head -> new file
			res.Files = append(res.Files, insertedFile(h))
		case h == nil: // only in base -> deleted file
			res.Files = append(res.Files, deletedFile(b))
		default:
			res.Files = append(res.Files, modifiedFile(b, h))
		}
	}
	return res
}

func unionKeys(a, b map[string]*model.File) []string {
	seen := map[string]bool{}
	var keys []string
	for k := range a {
		seen[k] = true
		keys = append(keys, k)
	}
	for k := range b {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func insertedFile(h *model.File) *model.FileChange {
	fc := &model.FileChange{Path: h.Path, Kind: model.KindInsert}
	if h.Root != nil {
		collectChanges(fc, h.Root, model.KindInsert)
	}
	return fc
}

func deletedFile(b *model.File) *model.FileChange {
	fc := &model.FileChange{Path: b.Path, Kind: model.KindDelete}
	if b.Root != nil {
		collectChanges(fc, b.Root, model.KindDelete)
	}
	return fc
}

func modifiedFile(b, h *model.File) *model.FileChange {
	fc := &model.FileChange{Path: h.Path, Kind: model.KindModify}
	alignNodes(fc, b.Root, h.Root)
	return fc
}

// aligned reports whether two aligned nodes are structurally identical. Text
// captures the full source of a node's subtree, so identical text implies an
// unchanged subtree.
func aligned(a, b *model.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Type == b.Type && a.Text == b.Text
}

// alignNodes compares the content of two already-aligned nodes a (base) and b
// (head). If they are structurally identical the whole subtree is unchanged.
// Otherwise the node is recorded as modified and its children are aligned by
// key to classify inserts/deletes/modifications one level down.
func alignNodes(fc *model.FileChange, a, b *model.Node) {
	if a == nil || b == nil {
		if a != nil {
			collectChanges(fc, a, model.KindDelete)
		}
		if b != nil {
			collectChanges(fc, b, model.KindInsert)
		}
		return
	}
	if aligned(a, b) {
		return // structurally identical; whole subtree unchanged
	}
	record(fc, a, model.KindModify)

	aByKey := indexByKey(a.Children)
	bByKey := indexByKey(b.Children)

	for _, ak := range a.Children {
		if match := findMatch(ak, bByKey); match != nil {
			alignNodes(fc, ak, match)
		} else {
			record(fc, ak, model.KindDelete)
		}
	}
	for _, bk := range b.Children {
		if findMatch(bk, aByKey) == nil {
			record(fc, bk, model.KindInsert)
		}
	}
}

// findMatch looks up a node by key in an index.
func findMatch(n *model.Node, byKey map[string][]*model.Node) *model.Node {
	if list := byKey[n.Key()]; len(list) > 0 {
		return list[0]
	}
	return nil
}

func indexByKey(nodes []*model.Node) map[string][]*model.Node {
	m := map[string][]*model.Node{}
	for _, n := range nodes {
		k := n.Key()
		m[k] = append(m[k], n)
	}
	return m
}

// record appends a node change to the file change, using its classification.
func record(fc *model.FileChange, n *model.Node, kind model.ChangeKind) {
	fc.Changes = append(fc.Changes, &model.NodeChange{
		Kind:  kind,
		Type:  n.Type,
		Name:  n.Name,
		Depth: n.Depth,
	})
}

// collectChanges records every node in a subtree as a single kind (used for
// fully-inserted or fully-deleted files).
func collectChanges(fc *model.FileChange, n *model.Node, kind model.ChangeKind) {
	record(fc, n, kind)
	for _, c := range n.Children {
		collectChanges(fc, c, kind)
	}
}
