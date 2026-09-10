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
	m := Compute(res, false, false)

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
	m := Compute(res, true, false)
	if !m.TopTwenty {
		t.Fatal("TopTwenty should pass through true")
	}
}

// TestBreadthCountsOnlyChangedFiles is the regression test for breadth having
// counted every source file in the tree rather than the touched ones. Before
// the fix this reported Breadth == 5.
func TestBreadthCountsOnlyChangedFiles(t *testing.T) {
	base := t.TempDir()
	head := t.TempDir()

	write := func(dir, name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// exactly one file differs between the trees
	write(base, "a.go", "package p\n\nfunc A() string { return \"hi\" }\n")
	write(head, "a.go", "package p\n\nfunc A() string { return \"hello\" }\n")

	// four byte-identical files that must not count toward breadth
	for _, n := range []string{"u1", "u2", "u3", "u4"} {
		src := "package p\n\nfunc " + n + "() int { return 1 }\n"
		write(base, n+".go", src)
		write(head, n+".go", src)
	}

	baseFiles, err := parse.Scan(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	headFiles, err := parse.Scan(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}

	m := Compute(diff.Diff(baseFiles, headFiles), false, false)
	if m.Breadth != 1 {
		t.Fatalf("Breadth = %d, want 1 (only a.go changed)", m.Breadth)
	}
}
