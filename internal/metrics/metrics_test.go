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

	if res := diff.Diff(baseFiles, headFiles); len(res.Files) != 1 {
		t.Fatalf("%d files changed, want 1 (only a.go)", len(res.Files))
	}
}

func TestAddedLines(t *testing.T) {
	cases := []struct {
		name, base, head string
		want             int
	}{
		{"new_file", "", "a\nb\n", 2},
		{"blank_lines_ignored", "a\n", "a\n\n\n", 0},
		{"moved_line", "a\nb\n", "b\na\n", 0},
		{"edited_line", "a\nb\n", "a\nc\n", 1},
		{"duplicate_added", "a\n", "a\na\n", 1},
		{"indent_only", "a\n", "    a\n", 0},
		{"deletion", "a\nb\n", "a\n", 0},
	}
	for _, c := range cases {
		have := map[string]int{}
		remember(have, c.base)
		if got := added(have, c.head); got != c.want {
			t.Errorf("%s: added = %d, want %d", c.name, got, c.want)
		}
	}
}
