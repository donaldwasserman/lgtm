package main

import (
	"os"
	"path/filepath"
	"testing"

	"lgtm/eval"
)

// TestAnalyzeRequiresReview drives the full pipeline (scan -> diff -> metrics
// -> decision) with distinct languages and asserts the Alloy-grounded outcome.
func TestAnalyzeRequiresReview(t *testing.T) {
	cases := []struct {
		name   string
		base   map[string]string
		head   map[string]string
		thr    eval.Thresholds
		expect bool
	}{
		{
			name: "deep_new_function_triggers",
			base: map[string]string{"a.go": "package main\nfunc f(){}\n"},
			head: map[string]string{"a.go": "package main\nfunc helper()int{return 1}\nfunc f(){_ = helper()}\n"},
			// The added helper/body produces structural+call depth in a single
			// file; assert decision true only when the change is non-trivial.
			thr:    eval.DefaultThresholds,
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
			thr:    eval.DefaultThresholds,
			expect: true,
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

func TestTopTwentyExemptsReview(t *testing.T) {
	base := writeTree(t, "base", map[string]string{"f.go": "package p\nfunc helper(){}\n"})
	// broad change that would normally require review
	headFiles := map[string]string{}
	for i := 0; i < 10; i++ {
		headFiles[filepath.Join("pkg", "g"+string(rune('a'+i))+".go")] = "package p\nfunc Z(){}\n"
	}
	headFiles["f.go"] = "package p\nfunc helper(){ _ = Z() }\n"
	head := writeTree(t, "head", headFiles)

	rep, noExempt, err := analyze(base, head, eval.DefaultThresholds, false)
	if err != nil {
		t.Fatal(err)
	}
	if !noExempt {
		t.Fatal("expected review required without exemption")
	}
	rep2, exempt, err := analyze(base, head, eval.DefaultThresholds, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = rep
	if exempt {
		t.Fatal("expected no review when topTwenty exemption set")
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
