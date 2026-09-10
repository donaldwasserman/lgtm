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
//	breadth:       number of distinct files touched
//	topTwenty:     caller-supplied trusted-submitter exemption
func Compute(res *diff.Result, topTwenty bool) eval.Metrics {
	m := eval.Metrics{TopTwenty: topTwenty}
	seenFiles := map[string]bool{}
	for _, fc := range res.Files {
		seenFiles[fc.Path] = true
		for _, c := range fc.Changes {
			switch c.Kind {
			case model.KindInsert:
				m.DepthNew = maxInt(m.DepthNew, c.Depth)
			case model.KindDelete, model.KindModify:
				m.DepthMod = maxInt(m.DepthMod, c.Depth)
			}
			m.DepthTotal = maxInt(m.DepthTotal, c.Depth)
		}
	}
	m.Breadth = len(seenFiles)
	return m
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
