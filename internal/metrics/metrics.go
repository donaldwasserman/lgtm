// Package metrics maps AST change output from the differ into the
// eval.Metrics struct that drives the Alloy-grounded RequiresReview decision.
package metrics

import (
	"lgtm/eval"
	"lgtm/internal/diff"
	"lgtm/internal/model"
)

// Compute converts a diff result and the trusted-submitter flag into
// eval.Metrics:
//
//	depthModified: max depth of modified + deleted (existing) AST nodes
//	depthNew:      max depth of newly added nodes
//	depthTotal:    max depth across all changed nodes
//	breadth:       number of distinct files actually changed
//	topTwenty:     caller-supplied trusted-submitter exemption
//	unparsed:      caller-supplied flag that some file failed to parse
func Compute(res *diff.Result, topTwenty, unparsed bool) eval.Metrics {
	m := eval.Metrics{TopTwenty: topTwenty, Unparsed: unparsed}
	seenFiles := map[string]bool{}
	for _, fc := range res.Files {
		if len(fc.Changes) == 0 {
			continue // unchanged file: not part of the change surface
		}
		seenFiles[fc.Path] = true
		for _, c := range fc.Changes {
			switch c.Kind {
			case model.KindInsert:
				m.DepthNew = max(m.DepthNew, c.Depth)
			case model.KindDelete, model.KindModify:
				m.DepthMod = max(m.DepthMod, c.Depth)
			}
			m.DepthTotal = max(m.DepthTotal, c.Depth)
		}
	}
	m.Breadth = len(seenFiles)
	return m
}
