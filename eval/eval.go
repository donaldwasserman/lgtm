// Package eval holds the PR-review complexity decision logic.
//
// RequiresReview implements the formal spec in alloy/pr_review.als and is
// tested against evaluator_alloy_test.go, which is generated from Alloy
// scenario instances by cmd/genalloy (see `make generate`).
package eval

// Metrics summarizes the AST-graph complexity of a pull request's changes.
type Metrics struct {
	DepthMod   int  // impact/call depth of modified/deleted existing AST nodes
	DepthNew   int  // internal nesting/call depth of newly added AST nodes
	Breadth    int  // surface spread / distinct modules and files touched
	DepthTotal int  // overall semantic and dependency depth across all changes
	TopTwenty  bool // submitter is in the top 20% of contributors (exemption)
}

// Thresholds holds the repository baseline thresholds.
type Thresholds struct {
	ThetaDepth     int // high depth threshold
	ThetaBreadth   int // high breadth threshold
	EpsilonTrivial int // upper bound below which depth is considered trivial
}

// DefaultThresholds mirrors alloy/scenarios.als (7, 6, 1).
var DefaultThresholds = Thresholds{ThetaDepth: 7, ThetaBreadth: 6, EpsilonTrivial: 1}

// RequiresReview evaluates whether a PR requires review from its AST-graph
// complexity metrics (see alloy/pr_review.als).
//
// A trusted (top-20%) contributor is always exempt, overriding both rules:
//
//	RequiresReview = !TopTwenty && (DepthMod >= ThetaDepth ||
//	                 (Breadth >= ThetaBreadth && DepthTotal > EpsilonTrivial))
func RequiresReview(m Metrics, t Thresholds) bool {
	if m.TopTwenty {
		return false
	}
	highDepthExisting := m.DepthMod >= t.ThetaDepth
	broadAndNontrivial := m.Breadth >= t.ThetaBreadth && m.DepthTotal > t.EpsilonTrivial
	return highDepthExisting || broadAndNontrivial
}
