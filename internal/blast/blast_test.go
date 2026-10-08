package blast

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/donaldwasserman/lgtm/internal/parse"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

func tree(t *testing.T, files map[string]string) (string, *symbols.Table) {
	t.Helper()
	dir := t.TempDir()
	for p, src := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	parsed, err := parse.Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, symbols.Extract(parsed)
}

func find(t *testing.T, tab *symbols.Table, name string) *symbols.Symbol {
	t.Helper()
	for _, s := range tab.Symbols {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no symbol %s", name)
	return nil
}

func radius(t *testing.T, files map[string]string, changed ...string) int {
	t.Helper()
	_, tab := tree(t, files)
	g := Build(tab)
	var cs []*symbols.Symbol
	for _, c := range changed {
		cs = append(cs, find(t, tab, c))
	}
	return Measure(g, cs).Radius
}

func TestRadius(t *testing.T) {
	chain := map[string]string{"a.go": `package p
func a() {}
func b() { a() }
func c() { b() }
func d() { c() }
func e() { d() }
`}
	if got := radius(t, chain, "a"); got != 3 {
		t.Errorf("chain: radius = %d, want 3 (b, c, d; e is 4 hops away)", got)
	}
	// Changing a caller too can shrink the radius (RadiusCanShrinkWhenMoreChanges).
	pair := map[string]string{"a.go": "package p\nfunc a() {}\nfunc b() { a() }\n"}
	if got := radius(t, pair, "a"); got != 1 {
		t.Errorf("pair: radius = %d, want 1", got)
	}
	if got := radius(t, pair, "a", "b"); got != 0 {
		t.Errorf("pair with the caller changed too: radius = %d, want 0", got)
	}

	fan := map[string]string{"lib/target.go": "package lib\nfunc Target() {}\n"}
	for i := 0; i < 60; i++ {
		fan[fmt.Sprintf("svc%d/f%d.go", i%7, i)] = fmt.Sprintf("package svc%d\nimport \"lib\"\nfunc Caller%d() { lib.Target() }\n", i%7, i)
	}
	if got := radius(t, fan, "Target"); got != 60 {
		t.Errorf("fan-in across packages: radius = %d, want 60", got)
	}

	if got := radius(t, map[string]string{"a.go": "package p\nfunc lonely() {}\nfunc other() {}\n"}, "lonely"); got != 0 {
		t.Errorf("uncalled: radius = %d, want 0", got)
	}

	cycle := map[string]string{"a.go": "package p\nfunc a() { b() }\nfunc b() { a() }\nfunc top() { a() }\n"}
	if got := radius(t, cycle, "a"); got != 2 {
		t.Errorf("cycle: radius = %d, want 2 (b, top)", got)
	}

	// A name defined in many places is not attributed globally.
	common := map[string]string{}
	for i := 0; i < 8; i++ {
		common[fmt.Sprintf("t%d/t.go", i)] = fmt.Sprintf("package t%d\ntype T struct{}\nfunc (T) Render() string { return \"\" }\n", i)
	}
	common["use/u.go"] = "package use\nfunc Use(x interface{ Render() string }) { x.Render() }\n"
	if got := radius(t, common, "Render"); got != 0 {
		t.Errorf("ambiguous name: radius = %d, want 0", got)
	}

	// Same module across files resolves before the global tier.
	ruby := map[string]string{
		"a.rb": "class Foo\n  def helper; end\nend\n",
		"b.rb": "class Foo\n  def run_it\n    helper\n  end\nend\n",
	}
	if got := radius(t, ruby, "helper"); got != 1 {
		t.Errorf("ruby open class: radius = %d, want 1", got)
	}
}

func TestAffectedMatchesDefinition(t *testing.T) {
	// callers[v]: 1 -> 0, 2 -> 1, 3 -> 2, 4 -> 3 (a chain), and 0 <-> 5.
	callers := [][]int{{1, 5}, {2}, {3}, {4}, nil, {0}}
	got := fmt.Sprint(Affected(callers, []int{0}, 3))
	if got != "[0 1 2 3 5]" {
		t.Errorf("Affected = %s", got)
	}
}

func TestExemption(t *testing.T) {
	dir, tab := tree(t, map[string]string{
		"a.go":       "package p\nfunc dead() {}\nfunc routed() {}\nfunc Exported() {}\nfunc called() {}\nfunc user() { called() }\n",
		"routes.txt": "GET /hook -> routed\n",
	})
	e := NewExemption(Build(tab), dir)
	for name, want := range map[string]bool{
		"dead":     true,  // unexported, uncalled, unmentioned
		"routed":   false, // named in a non-code file
		"Exported": false,
		"called":   false,
	} {
		if got := e.Exempt(find(t, tab, name)); got != want {
			t.Errorf("Exempt(%s) = %v, want %v", name, got, want)
		}
	}
	// dead being exempt also shows its own declaration is not a mention.
}
