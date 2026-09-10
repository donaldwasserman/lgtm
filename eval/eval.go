// Package eval holds the PR-review complexity decision logic.
//
// RequiresReview implements the formal spec in alloy/pr_review.als and is
// tested against evaluator_alloy_test.go, which is generated from Alloy
// scenario instances by cmd/genalloy (see `make generate`).
package eval

// Metrics summarizes the AST-graph complexity of a pull request's changes.
type Metrics struct {
	DepthMod int // impact/call depth of modified/deleted existing AST nodes
	// DepthNew is the nesting/call depth of newly added AST nodes. It is
	// deliberately not consulted by RequiresReview: deep new code on its own
	// does not force review (see NewCodeExemption in alloy/properties.als).
	DepthNew   int
	Breadth    int  // distinct files touched by the change
	DepthTotal int  // overall semantic and dependency depth across all changes
	TopTwenty  bool // submitter is in the top 20% of contributors (exemption)
	Unparsed   bool // a file on either side could not be parsed
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
// An unparseable tree overrides everything, the trusted-contributor exemption
// included: with no reliable metrics the gate must not wave the change
// through. Otherwise a trusted (top-20%) contributor is exempt:
//
//	RequiresReview = Unparsed || (!TopTwenty && (DepthMod >= ThetaDepth ||
//	                 (Breadth >= ThetaBreadth && DepthTotal > EpsilonTrivial)))
func RequiresReview(m Metrics, t Thresholds) bool {
	if m.Unparsed {
		return true
	}
	if m.TopTwenty {
		return false
	}
	highDepthExisting := m.DepthMod >= t.ThetaDepth
	broadAndNontrivial := m.Breadth >= t.ThetaBreadth && m.DepthTotal > t.EpsilonTrivial
	return highDepthExisting || broadAndNontrivial
}
