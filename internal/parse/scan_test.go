package parse

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/donaldwasserman/lgtm/internal/model"
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

// Mutual recursion once overflowed the stack: the cycle guard only tripped on
// names whose search had finished. A cycle now contributes nothing, and every
// member of one shares the longest chain leading out of it.
func TestCallDepthCycles(t *testing.T) {
	cases := []struct {
		name  string
		graph map[string]map[string]bool
		want  map[string]int
	}{
		{"two_cycle", map[string]map[string]bool{"a": {"b": true}, "b": {"a": true}},
			map[string]int{"a": 0, "b": 0}},
		{"three_cycle", map[string]map[string]bool{"a": {"b": true}, "b": {"c": true}, "c": {"a": true}},
			map[string]int{"a": 0, "b": 0, "c": 0}},
		{"cycle_with_exit", map[string]map[string]bool{
			"a": {"b": true}, "b": {"a": true, "c": true}, "c": {"d": true}},
			map[string]int{"a": 2, "b": 2, "c": 1, "d": 0}},
		{"chain_into_cycle", map[string]map[string]bool{
			"top": {"a": true}, "a": {"b": true}, "b": {"a": true}},
			map[string]int{"top": 1, "a": 0, "b": 0}},
		// The old search skipped any callee it had already finished, so
		// whether a counted b->d->e depended on map order: 2 or 3 at random.
		{"shared_callee", map[string]map[string]bool{
			"a": {"b": true, "d": true}, "b": {"d": true}, "d": {"e": true}},
			map[string]int{"a": 3, "b": 2, "d": 1, "e": 0}},
		{"diamond", map[string]map[string]bool{
			"a": {"b": true, "c": true}, "b": {"d": true}, "c": {"d": true}, "d": {"e": true}},
			map[string]int{"a": 3, "b": 2, "c": 2, "d": 1, "e": 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i := 0; i < 50; i++ { // map order must not matter
				for name, want := range c.want {
					if got := longestPath(name, c.graph); got != want {
						t.Fatalf("longestPath(%s) = %d, want %d", name, got, want)
					}
				}
			}
		})
	}
}

func TestScanSurvivesMutualRecursion(t *testing.T) {
	dir := t.TempDir()
	src := "package p\n\nfunc a(n int) int { if n == 0 { return 0 }; return b(n - 1) }\n\nfunc b(n int) int { return a(n) + leaf() }\n\nfunc leaf() int { return 1 }\n"
	if err := os.WriteFile(filepath.Join(dir, "r.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if files["r.go"] == nil || files["r.go"].Error != nil {
		t.Fatalf("r.go not scanned cleanly: %+v", files["r.go"])
	}
	// a and b form a cycle whose only way out is leaf: chain length 1.
	if got := maxCall(files["r.go"].Root); got != 1 {
		t.Errorf("max call depth = %d, want 1", got)
	}
}
