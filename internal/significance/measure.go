package significance

import (
	"sort"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// Rated is one changed existing symbol with its level.
type Rated struct {
	Pair  symbols.Pair
	Level eval.Level
	Parts []Part
}

// Symbol is the existing (base) side of the change.
func (r Rated) Symbol() *symbols.Symbol { return r.Pair.Base }

// Contributor is one symbol behind the score, for the report.
type Contributor struct {
	Symbol string     `json:"symbol"`
	File   string     `json:"file"`
	Level  eval.Level `json:"level"`
	Parts  []Part     `json:"parts"`
}

// Result holds the significance score and the symbols behind it.
type Result struct {
	// Score is the highest level among changes to existing symbols.
	Score eval.Level
	// Rated lists every changed existing symbol, highest level first.
	Rated []Rated
}

// Measure rates every change to an existing symbol. New symbols are not
// rated. A deleted symbol is an export removal when it was exported
// (crucial); otherwise its callers inside the module must change with it, a
// signature change of an unexported symbol (medium) - unless exempt reports
// that nothing could have used it, in which case it is not counted at all.
func Measure(pairs []symbols.Pair, exempt func(*symbols.Symbol) bool) Result {
	var r Result
	for _, p := range pairs {
		if p.Base == nil || !p.Changed() {
			continue
		}
		var parts []Part
		after := false
		switch {
		case p.Head != nil:
			parts, after = Classify(p.Base, p.Head), p.Head.Exported
		case p.Base.Exported:
			parts = []Part{Visibility}
		case exempt != nil && exempt(p.Base):
			continue
		default:
			parts = []Part{Signature}
		}
		lvl := Level(parts, p.Base.Exported, after)
		r.Rated = append(r.Rated, Rated{Pair: p, Level: lvl, Parts: parts})
		r.Score = max(r.Score, lvl)
	}
	sort.SliceStable(r.Rated, func(i, j int) bool {
		if r.Rated[i].Level != r.Rated[j].Level {
			return r.Rated[i].Level > r.Rated[j].Level
		}
		return r.Rated[i].Symbol().ID() < r.Rated[j].Symbol().ID()
	})
	return r
}

// TopN bounds the contributor list in the report.
const TopN = 5

// Contributors lists the highest-rated symbols.
func (r Result) Contributors() []Contributor {
	out := []Contributor{}
	for _, x := range r.Rated {
		if len(out) == TopN || x.Level == eval.None {
			break
		}
		out = append(out, Contributor{Symbol: x.Symbol().Display(), File: x.Symbol().File,
			Level: x.Level, Parts: x.Parts})
	}
	return out
}
