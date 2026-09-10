package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"lgtm/eval"
	"lgtm/internal/diff"
	"lgtm/internal/parse"
)

func TestComputeFromRealDiff(t *testing.T) {
	base := t.TempDir()
	head := t.TempDir()

	// base: single shallow function
	if err := os.WriteFile(filepath.Join(base, "app.py"),
		[]byte("def helper():\n    return 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// head: adds a deep recursive-ish chain of functions (call depth)
	headSrc := `def f4():
    return 1

def f3():
    return f4()

def f2():
    return f3()

def f1():
    return f2()

def app():
    return f1()
`
	if err := os.WriteFile(filepath.Join(head, "app.py"),
		[]byte(headSrc), 0644); err != nil {
		t.Fatal(err)
	}

	baseFiles, err := parse.Scan(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	headFiles, err := parse.Scan(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	res := diff.Diff(baseFiles, headFiles)
	m := Compute(res, false)

	if m.DepthNew <= 0 {
		t.Fatalf("DepthNew = %d, want >0 (added call chain)", m.DepthNew)
	}
	if m.Breadth != 1 {
		t.Fatalf("Breadth = %d, want 1 file touched", m.Breadth)
	}
	if m.DepthTotal < m.DepthNew {
		t.Fatalf("DepthTotal %d < DepthNew %d", m.DepthTotal, m.DepthNew)
	}
	// a 5-deep call chain should exceed default theta depth 7 only via nesting,
	// so just sanity-check against a lenient threshold.
	if !eval.RequiresReview(m, eval.Thresholds{ThetaDepth: 0, ThetaBreadth: 0, EpsilonTrivial: 0}) {
		t.Fatal("expected RequiresReview true with zeroed (permissive) thresholds")
	}
}

func TestTopTwentyPassesThrough(t *testing.T) {
	res := &diff.Result{}
	m := Compute(res, true)
	if !m.TopTwenty {
		t.Fatal("TopTwenty should pass through true")
	}
}
