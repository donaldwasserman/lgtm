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

func TestScanRust(t *testing.T) {
	dir := t.TempDir()
	src := `struct Counter { n: i32 }

trait Tick { fn tick(&mut self); }

impl Counter {
    fn a(&self) -> i32 { self.b() }
    fn b(&self) -> i32 { Counter::c() }
    fn c() -> i32 { 1 }
}

fn d() -> i32 { "1".parse::<i32>().unwrap() + parse::<i32>("2") }
`
	if err := os.WriteFile(filepath.Join(dir, "lib.rs"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	f := files["lib.rs"]
	if f == nil || f.Root == nil {
		t.Fatal("expected lib.rs to be scanned")
	}
	if f.Error != nil {
		t.Fatalf("valid Rust reported as %q", f.Error.Msg)
	}

	names := map[string]*model.Node{}
	var callees []string
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n.Name != "" {
			names[n.Name] = n
		}
		if n.Callee != "" {
			callees = append(callees, n.Callee)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(f.Root)

	for _, want := range []string{"Counter", "Tick", "tick", "a", "b", "c"} {
		if names[want] == nil {
			t.Errorf("definition %q not named", want)
		}
	}
	// a -> b -> c is a call chain of length 2.
	if a := names["a"]; a != nil && a.Call != 2 {
		t.Errorf("a.Call = %d, want 2", a.Call)
	}
	for _, c := range callees {
		if c == "i32" {
			t.Errorf("turbofish call resolved to type argument: callees = %v", callees)
		}
	}
	parses := 0
	for _, c := range callees {
		if c == "parse" {
			parses++
		}
	}
	if parses != 2 {
		t.Errorf("want 2 calls to parse, got callees %v", callees)
	}
}
