package eval_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/donaldwasserman/lgtm/eval"
)

// DefaultGate must be the configuration the model calls `defaults`, which the
// generated v1 examples (scenario_coreRefactor among them) are pinned to.
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
	s := eval.Scores{EditDepth: 9, DepthTotal: 9, BreadthFiles: 8,
		CogDelta: eval.On(6), BlastRadius: eval.On(60), Significance: eval.LevelOn(eval.High)}
	d := eval.Evaluate(s, eval.Facts{Unparsed: true}, eval.DefaultGate)
	want := []eval.Reason{eval.ReasonUnparsed, eval.ReasonEditDepth, eval.ReasonBreadthFiles,
		eval.ReasonCogDelta, eval.ReasonCalledSignature, eval.ReasonBlastRadius}
	if !reflect.DeepEqual(d.Reasons, want) {
		t.Errorf("reasons = %v, want %v", d.Reasons, want)
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
