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
	"strconv"
)

// Scores holds one score per measure for one pull request, each in that
// measure's own unit. A nil score is unavailable: the measure is not
// implemented for the change's languages. An unavailable score never forces
// review.
//
// The JSON names are the report's wire format (schemaVersion 3), which the
// action and users' own workflow steps read by these exact names.
type Scores struct {
	// EditDepth is how deep the deepest change to existing code reaches:
	// nesting depth plus intra-file call depth. It follows each grammar's
	// shape, so it is off in the default gate (docs/adr/0003).
	EditDepth int `json:"editDepth"`
	// NewDepth is the same measure for inserted code. Reported, never gated.
	NewDepth int `json:"newDepth"`
	// BreadthFiles is the number of non-test files with a substantive change.
	BreadthFiles int `json:"breadthFiles"`
	// BreadthModules is the number of packages those files belong to.
	BreadthModules *int `json:"breadthModules"`
	// LinesAdded is the number of non-blank lines the change adds to
	// non-test source files.
	LinesAdded int `json:"linesAdded"`
	// UncoveredFiles is the number of changed files no measure could look at
	// and the configuration does not ignore.
	UncoveredFiles int `json:"uncoveredFiles"`
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

// Facts are the inputs to the rules no threshold applies to.
type Facts struct {
	// Unparsed: some file on either side could not be fully parsed.
	Unparsed bool `json:"unparsed"`
	// AlwaysReview: a changed path matches the configured always-review list.
	AlwaysReview bool `json:"alwaysReview"`
}

// Gate is one team's configuration: one threshold per measure. A nil
// threshold is switched off. Thresholds are never zero; Validate rejects it.
type Gate struct {
	ThetaDepth        *int   `json:"thetaDepth"`
	ThetaBreadth      *int   `json:"thetaBreadth"`
	ThetaModules      *int   `json:"thetaModules"`
	ThetaLines        *int   `json:"thetaLines"`
	ThetaUncovered    *int   `json:"thetaUncovered"`
	ThetaCog          *int   `json:"thetaCog"`
	ThetaNewFunction  *int   `json:"thetaNewFunction"`
	ThetaSignificance *Level `json:"thetaSignificance"`
	ThetaBlast        *int   `json:"thetaBlast"`
}

// On returns a threshold that is switched on at v.
func On(v int) *int { return &v }

// LevelOn returns a significance threshold that is switched on at l.
func LevelOn(l Level) *Level { return &l }

// DefaultGate is the "balanced" preset. It mirrors `defaults` in
// alloy/gate_scenarios.als.
var DefaultGate = Gate{
	ThetaBreadth:      On(25),
	ThetaModules:      On(4),
	ThetaLines:        On(7000),
	ThetaUncovered:    On(1),
	ThetaCog:          On(5),
	ThetaNewFunction:  On(25),
	ThetaSignificance: LevelOn(Crucial),
	ThetaBlast:        On(30),
}

// StrictGate is the "strict" preset, for code where a wrong change is costly.
var StrictGate = Gate{
	ThetaBreadth:      On(8),
	ThetaModules:      On(3),
	ThetaLines:        On(400),
	ThetaUncovered:    On(1),
	ThetaCog:          On(3),
	ThetaNewFunction:  On(15),
	ThetaSignificance: LevelOn(Medium),
	ThetaBlast:        On(15),
}

// Presets are the named gates a configuration can start from.
var Presets = map[string]Gate{"balanced": DefaultGate, "strict": StrictGate}

// Validate rejects thresholds of zero or below. A zero threshold would fire
// on every change, pure additions included; "off" is the way to disable one.
func (g Gate) Validate() error {
	var errs []error
	check := func(name string, t *int) {
		if t != nil && *t < 1 {
			errs = append(errs, fmt.Errorf("theta-%s must be at least 1, or off (got %d)", name, *t))
		}
	}
	for _, name := range ThresholdNames {
		if t := g.count(name); t != nil {
			check(name, *t)
		}
	}
	if t := g.ThetaSignificance; t != nil && (*t < Low || *t > Crucial) {
		errs = append(errs, fmt.Errorf("theta-significance must be low..crucial, or off"))
	}
	return errors.Join(errs...)
}

// ThresholdNames lists every threshold, by the name configuration uses.
var ThresholdNames = []string{"depth", "breadth", "modules", "lines", "uncovered",
	"cog", "new-function", "significance", "blast"}

// count returns the field of a numeric threshold, or nil for any other name.
func (g *Gate) count(name string) **int {
	switch name {
	case "depth":
		return &g.ThetaDepth
	case "breadth":
		return &g.ThetaBreadth
	case "modules":
		return &g.ThetaModules
	case "lines":
		return &g.ThetaLines
	case "uncovered":
		return &g.ThetaUncovered
	case "cog":
		return &g.ThetaCog
	case "new-function":
		return &g.ThetaNewFunction
	case "blast":
		return &g.ThetaBlast
	}
	return nil
}

// Set assigns one threshold by name: a number, a significance level, or "off".
func (g *Gate) Set(name, value string) error {
	if name == "significance" {
		if value == "off" {
			g.ThetaSignificance = nil
			return nil
		}
		l, err := ParseLevel(value)
		if err != nil {
			return err
		}
		g.ThetaSignificance = &l
		return nil
	}
	p := g.count(name)
	if p == nil {
		return fmt.Errorf("unknown threshold %q", name)
	}
	if value == "off" {
		*p = nil
		return nil
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("want a number or \"off\", got %q", value)
	}
	*p = &v
	return nil
}

// Stricter combines two gates threshold by threshold, keeping whichever would
// require review sooner. A threshold that is off in one gate takes the other's
// value, so combining gates can never loosen either of them.
func (g Gate) Stricter(o Gate) Gate {
	out := g
	for _, name := range ThresholdNames {
		a, b := out.count(name), o.count(name)
		if a == nil { // significance
			continue
		}
		if *b != nil && (*a == nil || **b < **a) {
			*a = *b
		}
	}
	if s := o.ThetaSignificance; s != nil && (out.ThetaSignificance == nil || *s < *out.ThetaSignificance) {
		out.ThetaSignificance = s
	}
	return out
}

// Reason names one rule that made review required.
type Reason string

const (
	ReasonUnparsed       Reason = "unparsed"
	ReasonAlwaysReview   Reason = "always-review"
	ReasonEditDepth      Reason = "edit-depth"
	ReasonBreadthFiles   Reason = "breadth-files"
	ReasonBreadthModules Reason = "breadth-modules"
	ReasonLinesAdded     Reason = "lines-added"
	ReasonUncovered      Reason = "uncovered-files"
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
//	requiresReview = unparsed || alwaysReview || some threshold fires
//
// The first two hold under every setting of the thresholds.
func Evaluate(s Scores, f Facts, g Gate) Decision {
	var rs []Reason
	if f.Unparsed {
		rs = append(rs, ReasonUnparsed)
	}
	if f.AlwaysReview {
		rs = append(rs, ReasonAlwaysReview)
	}
	if reaches(&s.EditDepth, g.ThetaDepth) {
		rs = append(rs, ReasonEditDepth)
	}
	if reaches(&s.BreadthFiles, g.ThetaBreadth) {
		rs = append(rs, ReasonBreadthFiles)
	}
	if reaches(s.BreadthModules, g.ThetaModules) {
		rs = append(rs, ReasonBreadthModules)
	}
	if reaches(&s.LinesAdded, g.ThetaLines) {
		rs = append(rs, ReasonLinesAdded)
	}
	if reaches(&s.UncoveredFiles, g.ThetaUncovered) {
		rs = append(rs, ReasonUncovered)
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
	return Decision{RequiresReview: len(rs) > 0, Reasons: rs}
}

// RequiresReview is Evaluate's verdict alone.
func RequiresReview(s Scores, f Facts, g Gate) bool {
	return Evaluate(s, f, g).RequiresReview
}
