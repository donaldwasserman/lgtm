package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/blast"
	"github.com/donaldwasserman/lgtm/internal/config"
	"github.com/donaldwasserman/lgtm/internal/metrics"
	"github.com/donaldwasserman/lgtm/internal/significance"
)

func withGate(g eval.Gate) *config.Resolved {
	c := config.Default()
	c.Gate = g
	return c
}

// TestAnalyzeRequiresReview drives the full pipeline (scan -> diff -> metrics
// -> decision) with distinct languages and asserts the Alloy-grounded outcome.
func TestAnalyzeRequiresReview(t *testing.T) {
	cases := []struct {
		name   string
		base   map[string]string
		head   map[string]string
		thr    eval.Gate
		expect bool
	}{
		{
			name: "deep_new_function_triggers",
			base: map[string]string{"a.go": "package main\nfunc f(){}\n"},
			head: map[string]string{"a.go": "package main\nfunc helper()int{return 1}\nfunc f(){_ = helper()}\n"},
			// The added helper/body produces structural+call depth in a single
			// file; assert decision true only when the change is non-trivial.
			thr:    eval.DefaultGate,
			expect: false, // single file, low depth/breadth
		},
		{
			name: "broad_change_triggers",
			base: broadFiles("func X(){}\n"),
			head: broadFiles("func X(){ y() }\n"),
			// 26 edited files in one package => breadth 26 >= thetaBreadth 25
			thr:    eval.DefaultGate,
			expect: true,
		},
		{
			// The same 16 files added as new code are not wide: new code is
			// judged by lines added and new-function complexity.
			name:   "broad_addition_passes",
			base:   map[string]string{},
			head:   broadFiles("func X(){}\n"),
			thr:    eval.DefaultGate,
			expect: false,
		},
		{
			// Regression: breadth once counted every file in the tree, so a
			// one-word edit in a repo with >= thetaBreadth files tripped the
			// broadAndNontrivial rule on its own.
			name:   "trivial_edit_in_wide_repo",
			base:   wideRepo("hi"),
			head:   wideRepo("hello"),
			thr:    eval.DefaultGate,
			expect: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := writeTree(t, "base", c.base)
			head := writeTree(t, "head", c.head)
			_, requiresReview, err := analyze(base, head, withGate(c.thr))
			if err != nil {
				t.Fatal(err)
			}
			if requiresReview != c.expect {
				t.Fatalf("requiresReview = %v, want %v", requiresReview, c.expect)
			}
		})
	}
}

func writeTree(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for p, content := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// wideRepo builds a tree of eight files in which only greet.go depends on
// greeting; the other seven are identical across calls. It is deliberately
// wide enough to show that unchanged files do not count.
func wideRepo(greeting string) map[string]string {
	m := map[string]string{
		"greet.go": "package p\n\nfunc Greet() string { return \"" + greeting + "\" }\n",
	}
	for i := 0; i < 7; i++ {
		n := "u" + string(rune('a'+i))
		m[filepath.Join("pkg", n+".go")] = "package p\n\nfunc " + n + "() int { return 1 }\n"
	}
	return m
}

// TestUnparseableFileForcesReview covers the gate failing closed. Tree-sitter
// reports broken syntax as ERROR nodes rather than a parse error, so a
// malformed file used to produce a shallow diff and sail through.
func TestUnparseableFileForcesReview(t *testing.T) {
	base := writeTree(t, "base", map[string]string{
		"x.go": "package p\n\nfunc A() int { return 1 }\n",
	})
	head := writeTree(t, "head", map[string]string{
		"x.go": "package p\n\nfunc A() int { return \n((( \n",
	})

	rep, requiresReview, err := analyze(base, head, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if !requiresReview {
		t.Fatal("expected review to be required for an unparseable file")
	}
	if !rep.Facts.Unparsed {
		t.Error("Facts.Unparsed = false, want true")
	}
	if len(rep.ParseErrors) != 1 ||
		rep.ParseErrors[0].Path != "x.go" || rep.ParseErrors[0].Side != "head" {
		t.Fatalf("ParseErrors = %+v, want a single head-side entry for x.go", rep.ParseErrors)
	}
}

// TestCleanTreeReportsNoParseErrors pins the negative case, so the gate above
// cannot start firing on well-formed input.
func TestCleanTreeReportsNoParseErrors(t *testing.T) {
	base := writeTree(t, "base", map[string]string{"x.go": "package p\n\nfunc A() int { return 1 }\n"})
	head := writeTree(t, "head", map[string]string{"x.go": "package p\n\nfunc A() int { return 2 }\n"})

	rep, requiresReview, err := analyze(base, head, config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.ParseErrors) != 0 {
		t.Fatalf("ParseErrors = %+v, want none", rep.ParseErrors)
	}
	if rep.Facts.Unparsed || requiresReview {
		t.Fatalf("clean one-line edit: Unparsed=%v requiresReview=%v, want false/false",
			rep.Facts.Unparsed, requiresReview)
	}
}

// TestNewMeasuresEndToEnd drives the new measures through the real pipeline
// under the default gate.
func TestNewMeasuresEndToEnd(t *testing.T) {
	const lib = "package lib\n\nfunc Parse(s string) int { return len(s) }\n"
	const libWider = "package lib\n\nfunc Parse(s string, strict bool) int { return len(s) }\n"
	const caller = "package app\n\nimport \"lib\"\n\nfunc Run() int { return lib.Parse(\"x\") }\n"
	cases := []struct {
		name       string
		base, head map[string]string
		want       bool
		reason     eval.Reason
	}{
		{
			name:   "called_exported_signature",
			base:   map[string]string{"lib/lib.go": lib, "app/app.go": caller},
			head:   map[string]string{"lib/lib.go": libWider, "app/app.go": caller},
			want:   true,
			reason: eval.ReasonCalledSignature,
		},
		{
			name: "uncalled_exported_signature",
			base: map[string]string{"lib/lib.go": lib},
			head: map[string]string{"lib/lib.go": libWider},
			want: false,
		},
		{
			name: "dead_private_function_deleted",
			base: map[string]string{"lib/lib.go": lib + "\nfunc unused() int { return 7 }\n"},
			head: map[string]string{"lib/lib.go": lib},
			want: false,
		},
		{
			name: "export_removed",
			base: map[string]string{"lib/lib.go": lib},
			head: map[string]string{"lib/lib.go": "package lib\n\nfunc parse(s string) int { return len(s) }\n"},
			want: true, reason: eval.ReasonSignificance,
		},
		{
			name: "existing_function_much_harder_to_read",
			base: map[string]string{"a.go": "package a\n\nfunc F(xs []int) int { return len(xs) }\n"},
			head: map[string]string{"a.go": `package a

func F(xs []int) int {
	n := 0
	for _, x := range xs {
		if x > 0 && x < 10 || x == 99 {
			for x > 0 {
				x--
			}
		}
	}
	return n
}
`},
			want: true, reason: eval.ReasonCogDelta,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep, got, err := analyze(writeTree(t, "base", c.base), writeTree(t, "head", c.head), config.Default())
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("requiresReview = %v, want %v; reasons %v, scores %+v", got, c.want, rep.Reasons, rep.Scores)
			}
			if c.reason != "" {
				found := false
				for _, r := range rep.Reasons {
					found = found || r == c.reason
				}
				if !found {
					t.Errorf("reasons = %v, want %s among them", rep.Reasons, c.reason)
				}
			}
		})
	}
}

func hasReason(rep report, r eval.Reason) bool {
	for _, x := range rep.Reasons {
		if x == r {
			return true
		}
	}
	return false
}

func analyzeFiles(t *testing.T, base, head map[string]string, cfg *config.Resolved) report {
	t.Helper()
	rep, _, err := analyze(writeTree(t, "base", base), writeTree(t, "head", head), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// TestUncoveredFiles: a changed file no measure can look at needs review
// unless the configuration says it does not matter.
func TestUncoveredFiles(t *testing.T) {
	cases := []struct {
		name       string
		base, head map[string]string
		want       int
	}{
		{"unsupported_language", map[string]string{"A.cs": "a"}, map[string]string{"A.cs": "b"}, 1},
		{"sql_added", nil, map[string]string{"m.sql": "drop table t;"}, 1},
		{"manifest_deleted", map[string]string{"go.mod": "module a"}, nil, 1},
		{"docs_ignored", map[string]string{"README.md": "a"}, map[string]string{"README.md": "b"}, 0},
		{"docs_dir_ignored", map[string]string{"docs/run.sh": "a"}, map[string]string{"docs/run.sh": "b"}, 0},
		{"unchanged", map[string]string{"m.sql": "x"}, map[string]string{"m.sql": "x"}, 0},
		{"same_size_differs", map[string]string{"m.sql": "aa"}, map[string]string{"m.sql": "ab"}, 1},
		{"excluded_dir", map[string]string{"node_modules/x.yml": "a"}, map[string]string{"node_modules/x.yml": "b"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := analyzeFiles(t, c.base, c.head, config.Default())
			if rep.Scores.UncoveredFiles != c.want {
				t.Fatalf("uncoveredFiles = %d, want %d; files %+v", rep.Scores.UncoveredFiles, c.want, rep.Files)
			}
			if hasReason(rep, eval.ReasonUncovered) != (c.want > 0) {
				t.Errorf("reasons = %v", rep.Reasons)
			}
		})
	}
}

// TestStylesheets: a stylesheet is never uncovered, but the lines it adds and
// the existing file it touches still count; like new code, a new stylesheet
// counts only its lines. Whitespace and reordering change nothing.
func TestStylesheets(t *testing.T) {
	const a = ".a { color: red; }\n.b { margin: 0; }\n"
	cases := []struct {
		name        string
		base, head  map[string]string
		files, line int
	}{
		{"edited", map[string]string{"app.css": a}, map[string]string{"app.css": ".a { color: blue; }\n.b { margin: 0; }\n"}, 1, 1},
		{"added_scss", nil, map[string]string{"s/x.scss": "a {\n  b: c;\n}\n"}, 0, 3},
		{"reordered", map[string]string{"app.css": a}, map[string]string{"app.css": ".b { margin: 0; }\n.a { color: red; }\n"}, 0, 0},
		{"whitespace", map[string]string{"app.css": a}, map[string]string{"app.css": "\n" + a + "\n"}, 0, 0},
		{"deleted", map[string]string{"app.css": a}, nil, 1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := analyzeFiles(t, c.base, c.head, config.Default())
			if rep.Scores.UncoveredFiles != 0 {
				t.Errorf("uncoveredFiles = %d, want 0", rep.Scores.UncoveredFiles)
			}
			if rep.Scores.BreadthFiles != c.files || rep.Scores.LinesAdded != c.line {
				t.Errorf("files=%d lines=%d, want %d and %d", rep.Scores.BreadthFiles, rep.Scores.LinesAdded, c.files, c.line)
			}
		})
	}
	t.Run("opt_out", func(t *testing.T) {
		cfg, err := (&config.File{StylePaths: []string{"!*.css"}, UncoveredIgnore: []string{"!*.css"}}).Resolve(config.Options{})
		if err != nil {
			t.Fatal(err)
		}
		rep := analyzeFiles(t, map[string]string{"app.css": "a"}, map[string]string{"app.css": "b"}, cfg)
		if rep.Scores.UncoveredFiles != 1 {
			t.Errorf("uncoveredFiles = %d, want 1", rep.Scores.UncoveredFiles)
		}
	})
}

func TestAlwaysReviewPaths(t *testing.T) {
	const on, off = "on: push\n", "on: pull_request\n"
	rep := analyzeFiles(t, map[string]string{".github/workflows/x.yml": on}, map[string]string{".github/workflows/x.yml": off}, config.Default())
	if !hasReason(rep, eval.ReasonAlwaysReview) || !rep.Facts.AlwaysReview {
		t.Errorf("a workflow change must need review: %v", rep.Reasons)
	}
	rep = analyzeFiles(t, nil, map[string]string{config.FileName: "preset: strict\n"}, config.Default())
	if !hasReason(rep, eval.ReasonAlwaysReview) {
		t.Errorf("adding the configuration must need review: %v", rep.Reasons)
	}
}

// TestConfigComesFromBase: a change cannot loosen the rules it is judged by.
func TestConfigComesFromBase(t *testing.T) {
	const a, b = "package a\n\nfunc F() {}\n", "package a\n\nfunc F() {}\n\nfunc G() {}\n"
	base := writeTree(t, "base", map[string]string{"a.go": a, config.FileName: "always-review:\n  - a.go\n"})
	head := writeTree(t, "head", map[string]string{"a.go": b, config.FileName: "always-review:\n  - '!/.lgtm.yml'\n"})
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--base", base, "--head", head}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1 from the base-side rule; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"a.go"`) {
		t.Errorf("report should name a.go: %s", stdout.String())
	}
}

func TestExcludeDefaultsCanBeRemoved(t *testing.T) {
	base := map[string]string{"internal/build/a.go": "package b\n\nfunc F() {}\n"}
	head := map[string]string{"internal/build/a.go": "package b\n\nfunc F() {}\n\nfunc G() {}\n"}
	if rep := analyzeFiles(t, base, head, config.Default()); len(rep.Files) != 0 {
		t.Fatalf("build/ is excluded by default: %+v", rep.Files)
	}
	f := &config.File{Exclude: []string{"!build"}}
	cfg, err := f.Resolve(config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep := analyzeFiles(t, base, head, cfg); len(rep.Files) != 1 {
		t.Fatalf("!build should bring the directory back: %+v", rep.Files)
	}
}

// TestBreadthCountsSubstantiveNonTestFiles: only changes that could alter
// behaviour, outside tests, make a change wide.
func TestBreadthCountsSubstantiveNonTestFiles(t *testing.T) {
	fn := func(body string) string { return "package a\n\nfunc F() {\n" + body + "\n}\n" }
	cases := []struct {
		name       string
		base, head string
		path       string
		want       int
	}{
		{"statement", fn("\tg(1)"), fn("\tg(2)"), "a.go", 1},
		{"comment", fn("\t// a\n\tg(1)"), fn("\t// b\n\tg(1)"), "a.go", 0},
		{"local_rename", fn("\tx := 1\n\tg(x)"), fn("\ty := 1\n\tg(y)"), "a.go", 0},
		{"import_only", "package a\n\nimport \"x\"\n", "package a\n\nimport \"y\"\n", "a.go", 0},
		{"test_file", fn("\tg(1)"), fn("\tg(2)"), "a_test.go", 0},
		{"test_dir", "def f():\n    return 1\n", "def f():\n    return 2\n", "tests/a.py", 0},
		{"top_level_script", "print(1)\n", "print(2)\n", "a.py", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := analyzeFiles(t, map[string]string{c.path: c.base}, map[string]string{c.path: c.head}, config.Default())
			if rep.Scores.BreadthFiles != c.want {
				t.Fatalf("breadthFiles = %d, want %d", rep.Scores.BreadthFiles, c.want)
			}
		})
	}
}

// TestModuleSweepRequiresReview: the default gate counts packages.
func TestModuleSweepRequiresReview(t *testing.T) {
	base, head := map[string]string{}, map[string]string{}
	for i := 0; i < 4; i++ {
		p := string(rune('a'+i)) + "/f.go"
		base[p] = "package p\n\nfunc F() {\n\tg(1)\n}\n"
		head[p] = "package p\n\nfunc F() {\n\tg(2)\n}\n"
	}
	rep := analyzeFiles(t, base, head, config.Default())
	if !hasReason(rep, eval.ReasonBreadthModules) {
		t.Errorf("reasons = %v, scores %+v", rep.Reasons, rep.Scores)
	}
}

// TestDepthIsOffByDefault: the one-token edit edit depth used to flag.
func TestDepthIsOffByDefault(t *testing.T) {
	const tmpl = "package a\n\nfunc log(s string) {}\n\nfunc F() {\n\tlog(%s)\n}\n"
	base := map[string]string{"a.go": strings.Replace(tmpl, "%s", `"old"`, 1)}
	head := map[string]string{"a.go": strings.Replace(tmpl, "%s", `"new"`, 1)}
	rep := analyzeFiles(t, base, head, config.Default())
	if rep.RequiresReview || rep.Scores.EditDepth < 7 {
		t.Fatalf("requiresReview %v at edit depth %d", rep.RequiresReview, rep.Scores.EditDepth)
	}
	f := &config.File{Thresholds: map[string]string{"depth": "7"}}
	cfg, err := f.Resolve(config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep = analyzeFiles(t, base, head, cfg); !hasReason(rep, eval.ReasonEditDepth) {
		t.Errorf("depth can still be switched on: %v", rep.Reasons)
	}
	cs, ok := rep.Contributors["editDepth"].([]metrics.Contributor)
	if !ok || len(cs) != 1 || cs[0].File != "a.go" || cs[0].Line != 6 {
		t.Errorf("editDepth contributors = %+v, want a.go line 6", rep.Contributors["editDepth"])
	}
}

func TestLinesAdded(t *testing.T) {
	var big strings.Builder
	big.WriteString("package a\n\n")
	for i := 0; i < 2400; i++ {
		big.WriteString("func F" + strconv.Itoa(i) + "() {\n\tg()\n}\n\n")
	}
	rep := analyzeFiles(t, nil, map[string]string{"big.go": big.String()}, config.Default())
	if !hasReason(rep, eval.ReasonLinesAdded) || rep.Scores.LinesAdded < 7000 {
		t.Errorf("reasons %v, linesAdded %d", rep.Reasons, rep.Scores.LinesAdded)
	}
	rep = analyzeFiles(t, nil, map[string]string{"big_test.go": big.String()}, config.Default())
	if rep.Scores.LinesAdded != 0 {
		t.Errorf("test lines counted: %d", rep.Scores.LinesAdded)
	}
}

// TestConstantsAreSymbols: a constant's value is visible in every language.
func TestMovedCodeAddsNoLines(t *testing.T) {
	const src = "package a\n\nfunc F() {\n\tg(1)\n}\n"
	rep := analyzeFiles(t, map[string]string{"a.go": src}, map[string]string{"z.go": src}, config.Default())
	if rep.Scores.LinesAdded != 0 || rep.Scores.BreadthFiles != 0 || rep.RequiresReview {
		t.Errorf("renaming a file: lines %d, files %d, reasons %v", rep.Scores.LinesAdded, rep.Scores.BreadthFiles, rep.Reasons)
	}
}

func TestConstantsAreSymbols(t *testing.T) {
	cases := []struct{ path, tmpl string }{
		{"a.go", "package a\n\nconst Limit = %s\n\nfunc G() int { return Limit }\n"},
		{"b.go", "package a\n\nvar (\n\tLimit = %s\n)\n\nfunc G() int { return Limit }\n"},
		{"a.py", "LIMIT = %s\n\ndef g():\n    return LIMIT\n"},
		{"a.js", "export const LIMIT = %s;\nexport function g() { return LIMIT; }\n"},
		{"a.ts", "export const LIMIT: number = %s;\nexport function g() { return LIMIT; }\n"},
		{"A.java", "public class A {\n  public static final int LIMIT = %s;\n  int g() { return LIMIT; }\n}\n"},
		{"a.rs", "pub const LIMIT: i32 = %s;\npub fn g() -> i32 { LIMIT }\n"},
		{"a.rb", "LIMIT = %s\ndef g\n  LIMIT\nend\n"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			rep := analyzeFiles(t,
				map[string]string{c.path: strings.Replace(c.tmpl, "%s", "1", 1)},
				map[string]string{c.path: strings.Replace(c.tmpl, "%s", "2", 1)}, config.Default())
			if *rep.Scores.Significance != eval.Low {
				t.Errorf("significance = %v, want low", *rep.Scores.Significance)
			}
			// A new constant value is a low change: its reader is reported
			// through calledSignificance, but it has no blast radius.
			if *rep.Scores.CalledSignificance != eval.Low {
				t.Errorf("calledSignificance = %v, want low: G reads it", *rep.Scores.CalledSignificance)
			}
			if *rep.Scores.BlastRadius != 0 {
				t.Errorf("blastRadius = %d, want 0 for a low change", *rep.Scores.BlastRadius)
			}
		})
	}
}

func TestContributorsCarryHeadLines(t *testing.T) {
	const base = "package a\n\nfunc Parse(s string) int { return 1 }\n\nfunc Run() { Parse(\"x\") }\n"
	const head = "package a\n\n// pushed down\n\nfunc Parse(s, t string) int { return 1 }\n\nfunc Run() { Parse(\"x\") }\n"
	rep := analyzeFiles(t, map[string]string{"a.go": base}, map[string]string{"a.go": head}, config.Default())
	sig, ok := rep.Contributors["significance"].([]significance.Contributor)
	if !ok || len(sig) == 0 || sig[0].Line != 5 {
		t.Errorf("significance = %+v, want line 5", rep.Contributors["significance"])
	}
	br, ok := rep.Contributors["blastRadius"].([]blast.Contributor)
	if !ok || len(br) == 0 || br[0].Line != 5 || br[0].Deleted {
		t.Errorf("blastRadius = %+v, want line 5", rep.Contributors["blastRadius"])
	}
}

func TestOverrideTightensTheGate(t *testing.T) {
	base := map[string]string{"pay/a.go": "package a\n\nfunc F(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"}
	head := map[string]string{"pay/a.go": "package a\n\nfunc F(x int) int {\n\tif x >= 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"}
	f := &config.File{Overrides: []config.Override{{Name: "pay", Paths: []string{"pay/**"}, Preset: "strict"}}}
	cfg, err := f.Resolve(config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rep := analyzeFiles(t, base, head, cfg)
	if !hasReason(rep, eval.ReasonSignificance) || len(rep.Overrides) != 1 {
		t.Errorf("reasons %v, overrides %v", rep.Reasons, rep.Overrides)
	}
	if rep = analyzeFiles(t, base, head, config.Default()); rep.RequiresReview {
		t.Errorf("without the override this passes: %v", rep.Reasons)
	}
}

// TestBlastCountsLogicChanges: blast radius asks how much depends on a
// change to logic, so a statement edit in a widely called function has none
// and a condition edit in the same function has its callers.
func TestBlastCountsLogicChanges(t *testing.T) {
	var callers strings.Builder
	for i := 0; i < 40; i++ {
		callers.WriteString("func C" + strconv.Itoa(i) + "() { Check(1) }\n")
	}
	const base = "package a\n\nfunc Check(n int) bool {\n\tlog(n)\n\tif n > 0 {\n\t\treturn true\n\t}\n\treturn false\n}\n"
	statement := strings.Replace(base, "log(n)", "log(n, 2)", 1)
	condition := strings.Replace(base, "n > 0", "n >= 0", 1)
	files := func(src string) map[string]string {
		return map[string]string{"a.go": src, "callers.go": "package a\n\n" + callers.String()}
	}

	rep := analyzeFiles(t, files(base), files(statement), config.Default())
	if *rep.Scores.BlastRadius != 0 || rep.RequiresReview {
		t.Errorf("statement edit: blast %d, reasons %v", *rep.Scores.BlastRadius, rep.Reasons)
	}
	rep = analyzeFiles(t, files(base), files(condition), config.Default())
	if *rep.Scores.BlastRadius != 40 || !hasReason(rep, eval.ReasonBlastRadius) {
		t.Errorf("condition edit: blast %d, reasons %v", *rep.Scores.BlastRadius, rep.Reasons)
	}
}

// TestFeatureAdditionPasses: a new form, endpoint and business logic, spread
// over several new packages and wired in with one small edit, is not wide.
func TestFeatureAdditionPasses(t *testing.T) {
	base := map[string]string{
		"server/routes.go": "package server\n\nfunc Routes(r R) {\n\tr.Get(\"/a\", a)\n}\n",
	}
	head := map[string]string{
		"server/routes.go":  "package server\n\nfunc Routes(r R) {\n\tr.Get(\"/a\", a)\n\tr.Post(\"/orders\", orders.Create)\n}\n",
		"orders/handler.go": "package orders\n\nfunc Create(w W, r Q) {\n\to, err := Decode(r)\n\tif err != nil {\n\t\tw.Fail(err)\n\t\treturn\n\t}\n\tw.OK(Save(o))\n}\n",
		"orders/logic.go":   "package orders\n\nfunc Save(o Order) Order {\n\tif o.Total < 0 {\n\t\to.Total = 0\n\t}\n\treturn store.Put(o)\n}\n",
		"orders/decode.go":  "package orders\n\nfunc Decode(r Q) (Order, error) { return parse(r.Body) }\n",
		"store/orders.go":   "package store\n\nfunc Put(o any) any { return o }\n",
		"web/form.ts":       "export function OrderForm(props: { onSubmit: () => void }) {\n  return props.onSubmit;\n}\n",
		"web/api.ts":        "export async function createOrder(body: unknown) {\n  return fetch('/orders', { method: 'POST', body: JSON.stringify(body) });\n}\n",
	}
	rep := analyzeFiles(t, base, head, config.Default())
	if rep.Scores.BreadthFiles != 1 || *rep.Scores.BreadthModules != 1 || rep.RequiresReview {
		t.Errorf("files %d, modules %d, reasons %v", rep.Scores.BreadthFiles, *rep.Scores.BreadthModules, rep.Reasons)
	}
}

// TestTestDeletions: deleting tests en masse requires review; moving them or
// rewriting a few lines does not.
func TestTestDeletions(t *testing.T) {
	var suite strings.Builder
	suite.WriteString("package a\n\n")
	for i := 0; i < 30; i++ {
		n := strconv.Itoa(i)
		suite.WriteString("func TestCase" + n + "(t *testing.T) {\n\tgot := F(" + n + ")\n\tif got != " + n + " {\n\t\tt.Fatal(got)\n\t}\n}\n\n")
	}
	src := map[string]string{"a.go": "package a\n\nfunc F(n int) int { return n }\n"}
	with := func(tests map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range src {
			m[k] = v
		}
		for k, v := range tests {
			m[k] = v
		}
		return m
	}
	full := with(map[string]string{"a_test.go": suite.String()})

	rep := analyzeFiles(t, full, src, config.Default())
	if rep.Scores.TestLinesRemoved < 100 || !hasReason(rep, eval.ReasonTestDeletions) {
		t.Errorf("deleted suite: removed %d, reasons %v", rep.Scores.TestLinesRemoved, rep.Reasons)
	}
	rep = analyzeFiles(t, full, with(map[string]string{"moved_test.go": suite.String()}), config.Default())
	if rep.Scores.TestLinesRemoved != 0 || rep.RequiresReview {
		t.Errorf("moved suite: removed %d, reasons %v", rep.Scores.TestLinesRemoved, rep.Reasons)
	}
	edited := strings.Replace(suite.String(), "t.Fatal(got)", "t.Errorf(\"got %d\", got)", 3)
	rep = analyzeFiles(t, full, with(map[string]string{"a_test.go": edited}), config.Default())
	if rep.Scores.TestLinesRemoved != 3 || rep.RequiresReview {
		t.Errorf("edited suite: removed %d, reasons %v", rep.Scores.TestLinesRemoved, rep.Reasons)
	}
	// Fixtures are on a test path too, parsed or not.
	var fixture strings.Builder
	for i := 0; i < 120; i++ {
		fixture.WriteString("{\"case\": " + strconv.Itoa(i) + "}\n")
	}
	rep = analyzeFiles(t, with(map[string]string{"testdata/cases.jsonl": fixture.String()}), src, config.Default())
	if rep.Scores.TestLinesRemoved != 120 || !hasReason(rep, eval.ReasonTestDeletions) {
		t.Errorf("deleted fixture: removed %d, reasons %v", rep.Scores.TestLinesRemoved, rep.Reasons)
	}
}
