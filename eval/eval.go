// Package eval holds the PR-review complexity decision logic.
//
// RequiresReview implements the formal spec in alloy/pr_review.als and is
// tested against evaluator_alloy_test.go, which is generated from Alloy
// scenario instances by cmd/genalloy (see `make generate`).
package eval

// Metrics summarizes the AST-graph complexity of a pull request's changes.
type Metrics struct {
	// The JSON tags pin the report's wire format, which consumers (including
	// the check-run summary in action.yml) read by these exact names. They
	// restate what the field names already produced, so renaming a field can
	// no longer change the output by accident.
	DepthMod int `json:"DepthMod"` // impact/call depth of modified/deleted existing AST nodes
	// DepthNew is the nesting/call depth of newly added AST nodes. It is
	// deliberately not consulted by RequiresReview: deep new code on its own
	// does not force review (see NewCodeExemption in alloy/properties.als).
	DepthNew   int  `json:"DepthNew"`
	Breadth    int  `json:"Breadth"`    // distinct files touched by the change
	DepthTotal int  `json:"DepthTotal"` // overall semantic and dependency depth across all changes
	Trusted    bool `json:"Trusted"`    // submitter is exempt from review; how trust is decided is the caller's policy
	Unparsed   bool `json:"Unparsed"`   // a file on either side could not be parsed
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
// through. Otherwise a trusted contributor is exempt.
//
// Trusted is an opaque input: which contributors are trusted is a policy the
// caller resolves, deliberately outside this decision and outside the formal
// model.
//
//	RequiresReview = Unparsed || (!Trusted && (DepthMod >= ThetaDepth ||
//	                 (Breadth >= ThetaBreadth && DepthTotal > EpsilonTrivial)))
func RequiresReview(m Metrics, t Thresholds) bool {
	if m.Unparsed {
		return true
	}
	if m.Trusted {
		return false
	}
	highDepthExisting := m.DepthMod >= t.ThetaDepth
	broadAndNontrivial := m.Breadth >= t.ThetaBreadth && m.DepthTotal > t.EpsilonTrivial
	return highDepthExisting || broadAndNontrivial
}
