package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/diff"
	"github.com/donaldwasserman/lgtm/internal/parse"
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
	m := Compute(res)

	if m.NewDepth <= 0 {
		t.Fatalf("NewDepth = %d, want >0 (added call chain)", m.NewDepth)
	}
	if m.BreadthFiles != 1 {
		t.Fatalf("Breadth = %d, want 1 file touched", m.BreadthFiles)
	}
	if m.DepthTotal < m.NewDepth {
		t.Fatalf("DepthTotal %d < NewDepth %d", m.DepthTotal, m.NewDepth)
	}
	// a 5-deep call chain should exceed default theta depth 7 only via nesting,
	// so just sanity-check against a lenient threshold.
	g := eval.Gate{ThetaDepth: eval.On(1), ThetaBreadth: eval.On(1)}
	if !eval.RequiresReview(m, eval.Facts{}, g) {
		t.Fatal("expected RequiresReview true with the lowest thresholds")
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

	m := Compute(diff.Diff(baseFiles, headFiles))
	if m.BreadthFiles != 1 {
		t.Fatalf("Breadth = %d, want 1 (only a.go changed)", m.BreadthFiles)
	}
}
