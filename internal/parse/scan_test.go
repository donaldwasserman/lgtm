package parse

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"lgtm/internal/model"
)

func TestScanAndCallDepth(t *testing.T) {
	dir := t.TempDir()
	src := `package main

func helper() int { return 1 }

func alpha() int { return helper() }

func beta() int { return alpha() }
`
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	f := files["sample.go"]
	if f == nil {
		t.Fatal("expected sample.go to be scanned")
	}
	if f.Root == nil {
		t.Fatal("expected root node")
	}
	// beta -> alpha -> helper is a call chain of length 2.
	max := maxCall(f.Root)
	if max != 2 {
		t.Fatalf("max call depth = %d, want 2 (call chain beta->alpha->helper)", max)
	}
}

func maxCall(n *model.Node) int {
	if n == nil {
		return 0
	}
	m := n.Call
	for _, c := range n.Children {
		if d := maxCall(c); d > m {
			m = d
		}
	}
	return m
}
