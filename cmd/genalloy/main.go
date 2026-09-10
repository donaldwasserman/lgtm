// Command genalloy generates a table-driven Go test (eval/evaluator_alloy_test.go)
// from Alloy scenario instances.
//
// It reads:
//   - the Alloy scenario source (alloy/scenarios.als) to determine each
//     command's expected review polarity,
//   - the instance XML files dumped by the Alloy CLI (--type xml) to extract
//     the concrete metric values the solver produced for each scenario.
//
// Usage:
//
//	go run ./cmd/genalloy -als alloy/scenarios.als -xml alloy/runtime -out eval/evaluator_alloy_test.go
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

type Scenario struct {
	Name       string
	DepthMod   int
	DepthNew   int
	Breadth    int
	DepthTotal int
	TopTwenty  bool
	Unparsed   bool
	// Thresholds are read from the instance rather than assumed, so a scenario
	// that varies them is not silently tested against the defaults.
	ThetaDepth     int
	ThetaBreadth   int
	EpsilonTrivial int
	Want           bool
}

func main() {
	var (
		alsPath = flag.String("als", "alloy/scenarios.als", "path to the Alloy scenario source")
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
		want := !strings.Contains(strings.ReplaceAll(body, " ", ""), "notrequiresReview[Metrics]")
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
	cmd := inst.Command // e.g. "Run scenario_coreRefactor for 3"
	nameRe := regexp.MustCompile(`Run\s+([A-Za-z0-9_]+)`)
	m := nameRe.FindStringSubmatch(cmd)
	if m == nil {
		return Scenario{}, fmt.Errorf("cannot parse command %q", cmd)
	}
	s := Scenario{Name: m[1]}

	for _, f := range inst.Fields {
		if len(f.Tuple) == 0 || len(f.Tuple[0].Atom) < 2 {
			continue
		}
		value := f.Tuple[0].Atom[1].Label
		switch f.Label {
		case "depth_mod":
			s.DepthMod = mustInt(value)
		case "depth_new":
			s.DepthNew = mustInt(value)
		case "breadth":
			s.Breadth = mustInt(value)
		case "depth_total":
			s.DepthTotal = mustInt(value)
		case "topTwenty":
			s.TopTwenty = strings.Contains(value, "TRUE")
		case "unparsed":
			s.Unparsed = strings.Contains(value, "TRUE")
		case "theta_depth":
			s.ThetaDepth = mustInt(value)
		case "theta_breadth":
			s.ThetaBreadth = mustInt(value)
		case "epsilon_trivial":
			s.EpsilonTrivial = mustInt(value)
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

func writeTest(path, alsPath string, scenarios []Scenario) error {
	var b bytes.Buffer
	b.WriteString("// Code generated by genalloy; DO NOT EDIT.\n")
	b.WriteString("// source: " + alsPath + "\n\n")
	b.WriteString("package eval_test\n\n")
	b.WriteString("import (\n\t\"testing\"\n\n\t\"lgtm/eval\"\n)\n\n")

	b.WriteString("var alloyCases = []struct {\n")
	b.WriteString("name string\n")
	b.WriteString("depthMod, depthNew, breadth, depthTotal int\n")
	b.WriteString("topTwenty, unparsed bool\n")
	b.WriteString("thetaDepth, thetaBreadth, epsilonTrivial int\n")
	b.WriteString("want bool\n")
	b.WriteString("}{\n")
	for _, s := range scenarios {
		fmt.Fprintf(&b, "{%q, %d, %d, %d, %d, %t, %t, %d, %d, %d, %t},\n",
			s.Name, s.DepthMod, s.DepthNew, s.Breadth, s.DepthTotal,
			s.TopTwenty, s.Unparsed,
			s.ThetaDepth, s.ThetaBreadth, s.EpsilonTrivial, s.Want)
	}
	b.WriteString("}\n\n")

	b.WriteString(`func TestAlloyInstances(t *testing.T) {
	for _, tc := range alloyCases {
		t.Run(tc.name, func(t *testing.T) {
			got := eval.RequiresReview(
				eval.Metrics{
					DepthMod:   tc.depthMod,
					DepthNew:   tc.depthNew,
					Breadth:    tc.breadth,
					DepthTotal: tc.depthTotal,
					TopTwenty:  tc.topTwenty,
					Unparsed:   tc.unparsed,
				},
				eval.Thresholds{
					ThetaDepth:     tc.thetaDepth,
					ThetaBreadth:   tc.thetaBreadth,
					EpsilonTrivial: tc.epsilonTrivial,
				},
			)
			if got != tc.want {
				t.Errorf("RequiresReview(%+v) = %v, want %v", tc, got, tc.want)
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
