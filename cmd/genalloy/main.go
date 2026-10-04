// Command genalloy generates a table-driven Go test (eval/evaluator_alloy_test.go)
// from Alloy scenario instances.
//
// It reads:
//   - the Alloy scenario source (alloy/gate_scenarios.als) to determine each
//     command's expected review polarity,
//   - the instance XML files dumped by the Alloy CLI (--type xml) to extract
//     the concrete scores and thresholds the solver produced for each
//     scenario. A field with no tuple is meaningful: an unset threshold is
//     switched off and an unset score is unavailable, and both become nil.
//
// Usage:
//
//	go run ./cmd/genalloy -als alloy/gate_scenarios.als -xml alloy/runtime -out eval/evaluator_alloy_test.go
package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---- Alloy XML instance format ----

type AlloyDoc struct {
	Instance *Instance `xml:"instance"`
}

type Instance struct {
	Command string  `xml:"command,attr"`
	Fields  []Field `xml:"field"`
}

type Field struct {
	Label string  `xml:"label,attr"`
	Tuple []Tuple `xml:"tuple"`
}

type Tuple struct {
	Atom []Atom `xml:"atom"`
}

type Atom struct {
	Label string `xml:"label,attr"`
}

// ---- Parsed scenario ----

// Scenario is one instance: field label -> value, absent when the field had
// no tuple. Booleans are stored as 1/0.
type Scenario struct {
	Name   string
	Fields map[string]int
	Want   bool
}

func main() {
	var (
		alsPath = flag.String("als", "alloy/gate_scenarios.als", "path to the Alloy scenario source")
		xmlDir  = flag.String("xml", "alloy/runtime", "directory containing instance XML files")
		outPath = flag.String("out", "eval/evaluator_alloy_test.go", "output Go test file")
	)
	flag.Parse()

	want, err := parseExpectations(*alsPath)
	if err != nil {
		fatal("parse expectations: %v", err)
	}

	scenarios, err := parseInstances(*xmlDir)
	if err != nil {
		fatal("parse instances: %v", err)
	}

	if len(scenarios) == 0 {
		fatal("no scenarios found in %s", *xmlDir)
	}

	// attach expected polarity
	for i := range scenarios {
		w, ok := want[scenarios[i].Name]
		if !ok {
			fatal("no expectation found for scenario %q in %s", scenarios[i].Name, *alsPath)
		}
		scenarios[i].Want = w
	}

	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].Name < scenarios[j].Name })

	if err := writeTest(*outPath, *alsPath, scenarios); err != nil {
		fatal("write test: %v", err)
	}
	fmt.Printf("wrote %s (%d scenarios)\n", *outPath, len(scenarios))
}

// parseExpectations reads the .als source and, for each `run <name> { ... }`
// block, records whether the body asserts `requiresReview` (true) or
// `not requiresReview` (false).
func parseExpectations(path string) (map[string]bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	blockRe := regexp.MustCompile(`(?s)run\s+([A-Za-z0-9_]+)\s*\{`)
	names := blockRe.FindAllStringSubmatch(string(src), -1)
	out := make(map[string]bool, len(names))
	for _, m := range names {
		name := m[1]
		body := extractBlock(string(src), "run "+name)
		want := !strings.Contains(strings.ReplaceAll(body, " ", ""), "notrequiresReview[s,g]")
		out[name] = want
	}
	return out, nil
}

// extractBlock returns the text inside the first `{ ... }` following `prefix`.
func extractBlock(src, prefix string) string {
	i := strings.Index(src, prefix+" {")
	if i < 0 {
		i = strings.Index(src, prefix+"{")
		if i < 0 {
			return ""
		}
		open := i + strings.Index(src[i:], "{")
		return bracket(src, open)
	}
	open := i + strings.Index(src[i:], "{")
	return bracket(src, open)
}

func bracket(src string, open int) string {
	depth := 0
	for j := open; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open+1 : j]
			}
		}
	}
	return ""
}

// parseInstances reads every *.xml file in dir and extracts one Scenario per
// instance command.
func parseInstances(dir string) ([]Scenario, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, err
	}
	var out []Scenario
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var doc AlloyDoc
		if err := xml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if doc.Instance == nil {
			return nil, fmt.Errorf("%s: no <instance> element", f)
		}
		s, err := instanceToScenario(*doc.Instance)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, s)
	}
	return out, nil
}

func instanceToScenario(inst Instance) (Scenario, error) {
	cmd := inst.Command // e.g. "Run scenario_coreRefactor for 8 int, ..."
	nameRe := regexp.MustCompile(`Run\s+([A-Za-z0-9_]+)`)
	m := nameRe.FindStringSubmatch(cmd)
	if m == nil {
		return Scenario{}, fmt.Errorf("cannot parse command %q", cmd)
	}
	s := Scenario{Name: m[1], Fields: map[string]int{}}
	for _, f := range inst.Fields {
		if len(f.Tuple) == 0 {
			continue // no value: threshold off, or score unavailable
		}
		if len(f.Tuple) > 1 || len(f.Tuple[0].Atom) != 2 {
			return Scenario{}, fmt.Errorf("field %s: want one binary tuple, got %d", f.Label, len(f.Tuple))
		}
		v := f.Tuple[0].Atom[1].Label
		switch {
		case strings.Contains(v, "TRUE"):
			s.Fields[f.Label] = 1
		case strings.Contains(v, "FALSE"):
			s.Fields[f.Label] = 0
		default:
			s.Fields[f.Label] = mustInt(v)
		}
	}
	for _, req := range []string{"edit_depth", "depth_total", "breadth_files",
		"epsilon_trivial", "unparsed", "analysis_failed", "trusted"} {
		if _, ok := s.Fields[req]; !ok {
			return Scenario{}, fmt.Errorf("instance has no value for %s", req)
		}
	}
	return s, nil
}

func mustInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		fatal("invalid int %q", s)
	}
	return n
}

// goFields maps Alloy field labels to the eval struct field they set, and how
// to render the value: "int", "*int", "*level" or "bool".
var goFields = []struct{ label, owner, field, kind string }{
	{"edit_depth", "scores", "EditDepth", "int"},
	{"depth_total", "scores", "DepthTotal", "int"},
	{"breadth_files", "scores", "BreadthFiles", "int"},
	{"breadth_modules", "scores", "BreadthModules", "*int"},
	{"cog_delta", "scores", "CogDelta", "*int"},
	{"new_function_complexity", "scores", "NewFunctionComplexity", "*int"},
	{"significance", "scores", "Significance", "*level"},
	{"blast_radius", "scores", "BlastRadius", "*int"},
	{"trusted", "facts", "Trusted", "bool"},
	{"unparsed", "facts", "Unparsed", "bool"},
	{"analysis_failed", "facts", "AnalysisFailed", "bool"},
	{"theta_depth", "gate", "ThetaDepth", "*int"},
	{"theta_breadth", "gate", "ThetaBreadth", "*int"},
	{"epsilon_trivial", "gate", "EpsilonTrivial", "int"},
	{"theta_modules", "gate", "ThetaModules", "*int"},
	{"theta_cog", "gate", "ThetaCog", "*int"},
	{"theta_new_function", "gate", "ThetaNewFunction", "*int"},
	{"theta_significance", "gate", "ThetaSignificance", "*level"},
	{"theta_blast", "gate", "ThetaBlast", "*int"},
}

// literal renders one owner's struct literal. Absent pointer fields are left
// out, which is nil: off, or unavailable.
func literal(s Scenario, owner, typ string) string {
	var parts []string
	for _, f := range goFields {
		if f.owner != owner {
			continue
		}
		v, ok := s.Fields[f.label]
		if !ok {
			continue
		}
		switch f.kind {
		case "int":
			parts = append(parts, fmt.Sprintf("%s: %d", f.field, v))
		case "*int":
			parts = append(parts, fmt.Sprintf("%s: eval.On(%d)", f.field, v))
		case "*level":
			parts = append(parts, fmt.Sprintf("%s: eval.LevelOn(%d)", f.field, v))
		case "bool":
			if v == 1 {
				parts = append(parts, f.field+": true")
			}
		}
	}
	return "eval." + typ + "{" + strings.Join(parts, ", ") + "}"
}

func writeTest(path, alsPath string, scenarios []Scenario) error {
	var b bytes.Buffer
	b.WriteString("// Code generated by genalloy; DO NOT EDIT.\n")
	b.WriteString("// source: " + alsPath + "\n\n")
	b.WriteString("package eval_test\n\n")
	b.WriteString("import (\n\t\"testing\"\n\n\t\"github.com/donaldwasserman/lgtm/eval\"\n)\n\n")

	b.WriteString("var alloyCases = []struct {\n")
	b.WriteString("name string\n")
	b.WriteString("scores eval.Scores\n")
	b.WriteString("facts eval.Facts\n")
	b.WriteString("gate eval.Gate\n")
	b.WriteString("want bool\n")
	b.WriteString("}{\n")
	for _, s := range scenarios {
		fmt.Fprintf(&b, "{\n%q,\n%s,\n%s,\n%s,\n%t,\n},\n", s.Name,
			literal(s, "scores", "Scores"), literal(s, "facts", "Facts"),
			literal(s, "gate", "Gate"), s.Want)
	}
	b.WriteString("}\n\n")

	b.WriteString(`func TestAlloyInstances(t *testing.T) {
	for _, tc := range alloyCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.gate.Validate(); err != nil {
				t.Fatalf("model produced an invalid gate: %v", err)
			}
			d := eval.Evaluate(tc.scores, tc.facts, tc.gate)
			if d.RequiresReview != tc.want {
				t.Errorf("Evaluate = %+v, want requiresReview %v", d, tc.want)
			}
			if d.RequiresReview != (len(d.Reasons) > 0) {
				t.Errorf("requiresReview %v disagrees with reasons %v", d.RequiresReview, d.Reasons)
			}
		})
	}
}
`)

	src, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("format generated source: %w", err)
	}
	return os.WriteFile(path, src, 0o644)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "genalloy: "+format+"\n", args...)
	os.Exit(1)
}
