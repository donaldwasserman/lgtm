// Package metrics turns the differ's output into eval.Scores, the per-measure
// scores the gate reads.
package metrics

import (
	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/diff"
	"github.com/donaldwasserman/lgtm/internal/model"
)

// Compute converts a diff result into the frozen v1 scores:
//
//	EditDepth:    max depth of modified + deleted (existing) AST nodes
//	NewDepth:     max depth of newly added nodes
//	DepthTotal:   max depth across all changed nodes
//	BreadthFiles: number of distinct files actually changed
//
// The newer measures are left nil (unavailable) here; their own packages
// fill them in.
func Compute(res *diff.Result) eval.Scores {
	var m eval.Scores
	seenFiles := map[string]bool{}
	for _, fc := range res.Files {
		if len(fc.Changes) == 0 {
			continue // unchanged file: not part of the change surface
		}
		seenFiles[fc.Path] = true
		for _, c := range fc.Changes {
			switch c.Kind {
			case model.KindInsert:
				m.NewDepth = max(m.NewDepth, c.Depth)
			case model.KindDelete, model.KindModify:
				m.EditDepth = max(m.EditDepth, c.Depth)
			}
			m.DepthTotal = max(m.DepthTotal, c.Depth)
		}
	}
	m.BreadthFiles = len(seenFiles)
	return m
}
