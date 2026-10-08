// Package eval holds the gate: the rule that turns a pull request's scores
// into "needs review" or not.
//
// Evaluate implements requiresReview in alloy/gate.als and is tested against
// evaluator_alloy_test.go, which is generated from the examples in
// alloy/gate_scenarios.als by cmd/genalloy (see `make generate`).
package eval

import (
	"errors"
	"fmt"
)

// Scores holds one score per measure for one pull request, each in that
// measure's own unit. A nil score is unavailable: the measure is not
// implemented for the change's languages. An unavailable score never forces
// review.
//
// The JSON names are the report's wire format (schemaVersion 2), which the
// action and users' own workflow steps read by these exact names.
type Scores struct {
	// EditDepth is how deep the deepest change to existing code reaches:
	// nesting depth plus intra-file call depth. Frozen at its v1 definition.
	EditDepth int `json:"editDepth"`
	// NewDepth is the same measure for inserted code. Reported, never gated.
	NewDepth int `json:"newDepth"`
	// DepthTotal is the edit depth across every change, new code included.
	DepthTotal int `json:"depthTotal"`
	// BreadthFiles is the number of files the change touches.
	BreadthFiles int `json:"breadthFiles"`
	// BreadthModules is the number of language modules the change touches.
	BreadthModules *int `json:"breadthModules"`
	// CogDelta is the largest increase in cognitive complexity among
	// functions that exist on both sides.
	CogDelta *int `json:"cogDelta"`
	// NewFunctionComplexity is the largest cognitive complexity among new
	// functions.
	NewFunctionComplexity *int `json:"newFunctionComplexity"`
	// Significance is the highest significance level among changes to
	// existing symbols.
	Significance *Level `json:"significance"`
	// CalledSignificance is the highest significance level among changed
	// existing symbols that something calls. The compound rule reads it, so
	// an exported signature change gates only when that symbol has callers.
	CalledSignificance *Level `json:"calledSignificance"`
	// BlastRadius is the number of symbols that call a changed existing
	// symbol, within three hops, excluding the changed symbols themselves.
	BlastRadius *int `json:"blastRadius"`
}

// Facts are the inputs to the fixed rules. They are not scores: no
// threshold applies to them.
type Facts struct {
	// Trusted exempts the change from every threshold. Which authors are
	// trusted is the caller's policy, outside this decision.
	Trusted bool `json:"trusted"`
	// Unparsed: some file on either side could not be fully parsed.
	Unparsed bool `json:"unparsed"`
	// AnalysisFailed: a measure supports a file's language but failed to
	// analyze it.
	AnalysisFailed bool `json:"analysisFailed"`
}

// Gate is one team's configuration: one threshold per measure. A nil
// threshold is switched off. Thresholds are never zero; Validate rejects it.
type Gate struct {
	ThetaDepth   *int `json:"thetaDepth"`
	ThetaBreadth *int `json:"thetaBreadth"`
	// EpsilonTrivial: the files threshold fires only when DepthTotal is above
	// this, so a wide but shallow change (a rename) passes.
	EpsilonTrivial    int    `json:"epsilonTrivial"`
	ThetaModules      *int   `json:"thetaModules"`
	ThetaCog          *int   `json:"thetaCog"`
	ThetaNewFunction  *int   `json:"thetaNewFunction"`
	ThetaSignificance *Level `json:"thetaSignificance"`
	ThetaBlast        *int   `json:"thetaBlast"`
}

// On returns a threshold that is switched on at v.
func On(v int) *int { return &v }

// LevelOn returns a significance threshold that is switched on at l.
func LevelOn(l Level) *Level { return &l }

// DefaultGate mirrors `defaults` in alloy/gate_scenarios.als.
var DefaultGate = Gate{
	ThetaDepth:        On(7),
	ThetaBreadth:      On(6),
	EpsilonTrivial:    1,
	ThetaModules:      nil,
	ThetaCog:          On(5),
	ThetaNewFunction:  On(25),
	ThetaSignificance: LevelOn(Crucial),
	ThetaBlast:        On(50),
}

// Validate rejects thresholds of zero or below. A zero threshold would fire
// on every change, pure additions included; "off" is the way to disable one.
func (g Gate) Validate() error {
	var errs []error
	check := func(name string, t *int) {
		if t != nil && *t < 1 {
			errs = append(errs, fmt.Errorf("%s must be at least 1, or off (got %d)", name, *t))
		}
	}
	check("theta-depth", g.ThetaDepth)
	check("theta-breadth", g.ThetaBreadth)
	check("theta-modules", g.ThetaModules)
	check("theta-cog", g.ThetaCog)
	check("theta-new-function", g.ThetaNewFunction)
	check("theta-blast", g.ThetaBlast)
	if t := g.ThetaSignificance; t != nil && (*t < Low || *t > Crucial) {
		errs = append(errs, fmt.Errorf("theta-significance must be low..crucial, or off"))
	}
	if g.EpsilonTrivial < 0 {
		errs = append(errs, fmt.Errorf("epsilon-trivial must not be negative (got %d)", g.EpsilonTrivial))
	}
	return errors.Join(errs...)
}

// Reason names one rule that made review required.
type Reason string

const (
	ReasonUnparsed       Reason = "unparsed"
	ReasonAnalysisFailed Reason = "analysis-failed"
	ReasonEditDepth      Reason = "edit-depth"
	ReasonBreadthFiles   Reason = "breadth-files"
	ReasonBreadthModules Reason = "breadth-modules"
	ReasonCogDelta       Reason = "cog-delta"
	ReasonNewFunction    Reason = "new-function-complexity"
	ReasonSignificance   Reason = "significance"
	// ReasonCalledSignature is the compound significance rule: a change of
	// at least high significance (an exported signature) to a symbol
	// something calls.
	ReasonCalledSignature Reason = "called-exported-signature"
	ReasonBlastRadius     Reason = "blast-radius"
)

// Decision is the gate's answer and every rule that contributed to it.
type Decision struct {
	RequiresReview bool     `json:"requiresReview"`
	Reasons        []Reason `json:"reasons"`
}

// reaches reports whether an available score meets a threshold that is on.
func reaches(score, theta *int) bool {
	return score != nil && theta != nil && *score >= *theta
}

// Evaluate applies the gate (alloy/gate.als):
//
//	requiresReview = unparsed || analysisFailed ||
//	                 (!trusted && some threshold fires)
//
// The fixed rules - unparsed, analysis failed - hold under every
// configuration and override trust. Otherwise a trusted author is exempt from
// every threshold.
func Evaluate(s Scores, f Facts, g Gate) Decision {
	var rs []Reason
	if f.Unparsed {
		rs = append(rs, ReasonUnparsed)
	}
	if f.AnalysisFailed {
		rs = append(rs, ReasonAnalysisFailed)
	}
	if !f.Trusted {
		if reaches(&s.EditDepth, g.ThetaDepth) {
			rs = append(rs, ReasonEditDepth)
		}
		if reaches(&s.BreadthFiles, g.ThetaBreadth) && s.DepthTotal > g.EpsilonTrivial {
			rs = append(rs, ReasonBreadthFiles)
		}
		if reaches(s.BreadthModules, g.ThetaModules) {
			rs = append(rs, ReasonBreadthModules)
		}
		if reaches(s.CogDelta, g.ThetaCog) {
			rs = append(rs, ReasonCogDelta)
		}
		if reaches(s.NewFunctionComplexity, g.ThetaNewFunction) {
			rs = append(rs, ReasonNewFunction)
		}
		if g.ThetaSignificance != nil && s.Significance != nil {
			if *s.Significance >= *g.ThetaSignificance {
				rs = append(rs, ReasonSignificance)
			} else if s.CalledSignificance != nil && *s.CalledSignificance >= High {
				rs = append(rs, ReasonCalledSignature)
			}
		}
		if reaches(s.BlastRadius, g.ThetaBlast) {
			rs = append(rs, ReasonBlastRadius)
		}
	}
	return Decision{RequiresReview: len(rs) > 0, Reasons: rs}
}

// RequiresReview is Evaluate's verdict alone.
func RequiresReview(s Scores, f Facts, g Gate) bool {
	return Evaluate(s, f, g).RequiresReview
}
