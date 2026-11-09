package eval_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/donaldwasserman/lgtm/eval"
)

// DefaultGate must be the configuration the model calls `defaults`, which the
// generated examples (scenario_coreRefactor among them) are pinned to.
func TestDefaultGateMatchesModel(t *testing.T) {
	for _, tc := range alloyCases {
		if tc.name != "scenario_coreRefactor" {
			continue
		}
		if !reflect.DeepEqual(tc.gate, eval.DefaultGate) {
			t.Fatalf("DefaultGate = %+v, model defaults = %+v", eval.DefaultGate, tc.gate)
		}
		return
	}
	t.Fatal("scenario_coreRefactor not found in generated cases")
}

func TestValidateRejectsZeroThresholds(t *testing.T) {
	if err := eval.DefaultGate.Validate(); err != nil {
		t.Fatalf("default gate invalid: %v", err)
	}
	g := eval.DefaultGate
	g.ThetaDepth = eval.On(0)
	if g.Validate() == nil {
		t.Error("theta-depth 0 accepted; it must be off or at least 1")
	}
	g = eval.DefaultGate
	g.ThetaSignificance = eval.LevelOn(eval.None)
	if g.Validate() == nil {
		t.Error("theta-significance none accepted; it would fire on every change")
	}
	if (eval.Gate{}).Validate() != nil {
		t.Error("an all-off gate is valid")
	}
}

func TestReasonsNameEveryRuleThatFired(t *testing.T) {
	s := eval.Scores{EditDepth: 9, BreadthFiles: 16, LinesAdded: 1000, UncoveredFiles: 1,
		CogDelta: eval.On(6), BlastRadius: eval.On(60), Significance: eval.LevelOn(eval.High),
		CalledSignificance: eval.LevelOn(eval.High)}
	d := eval.Evaluate(s, eval.Facts{Unparsed: true, AlwaysReview: true}, eval.DefaultGate)
	want := []eval.Reason{eval.ReasonUnparsed, eval.ReasonAlwaysReview, eval.ReasonBreadthFiles,
		eval.ReasonLinesAdded, eval.ReasonUncovered,
		eval.ReasonCogDelta, eval.ReasonCalledSignature, eval.ReasonBlastRadius}
	if !reflect.DeepEqual(d.Reasons, want) {
		t.Errorf("reasons = %v, want %v", d.Reasons, want)
	}
}

func TestStricterNeverLoosens(t *testing.T) {
	a := eval.Gate{ThetaCog: eval.On(5), ThetaBlast: eval.On(10), ThetaSignificance: eval.LevelOn(eval.Crucial)}
	b := eval.Gate{ThetaCog: eval.On(9), ThetaLines: eval.On(100), ThetaSignificance: eval.LevelOn(eval.Medium)}
	want := eval.Gate{ThetaCog: eval.On(5), ThetaBlast: eval.On(10), ThetaLines: eval.On(100),
		ThetaSignificance: eval.LevelOn(eval.Medium)}
	if got := a.Stricter(b); !reflect.DeepEqual(got, want) {
		t.Errorf("Stricter = %+v, want %+v", got, want)
	}
	if got := b.Stricter(a); !reflect.DeepEqual(got, want) {
		t.Errorf("Stricter is not symmetric: %+v", got)
	}
}

func TestGateSet(t *testing.T) {
	var g eval.Gate
	for _, name := range eval.ThresholdNames {
		v := "3"
		if name == "significance" {
			v = "medium"
		}
		if err := g.Set(name, v); err != nil {
			t.Errorf("Set(%s): %v", name, err)
		}
		if err := g.Set(name, "off"); err != nil {
			t.Errorf("Set(%s, off): %v", name, err)
		}
	}
	if !reflect.DeepEqual(g, eval.Gate{}) {
		t.Errorf("all off = %+v", g)
	}
	if g.Set("nope", "1") == nil || g.Set("cog", "lots") == nil || g.Set("significance", "severe") == nil {
		t.Error("bad names and values must be errors")
	}
}

func TestLevelJSON(t *testing.T) {
	b, err := json.Marshal(eval.Scores{Significance: eval.LevelOn(eval.High)})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["significance"] != "high" {
		t.Errorf("significance = %v, want \"high\"", m["significance"])
	}
	if m["cogDelta"] != nil {
		t.Errorf("unavailable cogDelta = %v, want null", m["cogDelta"])
	}
	var l eval.Level
	if err := l.UnmarshalText([]byte("crucial")); err != nil || l != eval.Crucial {
		t.Errorf("UnmarshalText(crucial) = %v, %v", l, err)
	}
	if _, err := eval.ParseLevel("severe"); err == nil {
		t.Error("ParseLevel accepted an unknown level")
	}
}
