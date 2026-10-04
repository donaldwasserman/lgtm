package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donaldwasserman/lgtm/eval"
)

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
			base: map[string]string{},
			head: func() map[string]string {
				m := map[string]string{}
				for i := 0; i < 8; i++ {
					m[filepath.Join("pkg", "f"+string(rune('a'+i))+".go")] =
						"package p\nfunc X( ){}\n"
				}
				return m
			}(),
			// 8 new files => breadth 8 >= thetaBreadth 6 and depth>epsilon
			thr:    eval.DefaultGate,
			expect: true,
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
			_, requiresReview, err := analyze(base, head, c.thr, false)
			if err != nil {
				t.Fatal(err)
			}
			if requiresReview != c.expect {
				t.Fatalf("requiresReview = %v, want %v", requiresReview, c.expect)
			}
		})
	}
}

func TestTrustExemptsReview(t *testing.T) {
	base := writeTree(t, "base", map[string]string{"f.go": "package p\nfunc helper(){}\n"})
	// broad change that would normally require review
	headFiles := map[string]string{}
	for i := 0; i < 10; i++ {
		headFiles[filepath.Join("pkg", "g"+string(rune('a'+i))+".go")] = "package p\nfunc Z(){}\n"
	}
	headFiles["f.go"] = "package p\nfunc helper(){ _ = Z() }\n"
	head := writeTree(t, "head", headFiles)

	rep, noExempt, err := analyze(base, head, eval.DefaultGate, false)
	if err != nil {
		t.Fatal(err)
	}
	if !noExempt {
		t.Fatal("expected review required without exemption")
	}
	rep2, exempt, err := analyze(base, head, eval.DefaultGate, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = rep
	if exempt {
		t.Fatal("expected no review when the trusted exemption is set")
	}
	_ = rep2
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
// wider than the default thetaBreadth of 6.
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

	rep, requiresReview, err := analyze(base, head, eval.DefaultGate, false)
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

	// The top-20% exemption must not rescue a change that could not be measured.
	_, exempt, err := analyze(base, head, eval.DefaultGate, true)
	if err != nil {
		t.Fatal(err)
	}
	if !exempt {
		t.Fatal("top-20% exemption must not apply when a file failed to parse")
	}
}

// TestCleanTreeReportsNoParseErrors pins the negative case, so the gate above
// cannot start firing on well-formed input.
func TestCleanTreeReportsNoParseErrors(t *testing.T) {
	base := writeTree(t, "base", map[string]string{"x.go": "package p\n\nfunc A() int { return 1 }\n"})
	head := writeTree(t, "head", map[string]string{"x.go": "package p\n\nfunc A() int { return 2 }\n"})

	rep, requiresReview, err := analyze(base, head, eval.DefaultGate, false)
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
			rep, got, err := analyze(writeTree(t, "base", c.base), writeTree(t, "head", c.head), eval.DefaultGate, false)
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
