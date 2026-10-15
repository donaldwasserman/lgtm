package parse

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/donaldwasserman/lgtm/internal/model"
)

// Inline import types as call type arguments (tree-sitter-typescript#367),
// next to the variants the grammar always handled. Each sample is checked in
// both the TypeScript and TSX grammars.
var typeofImportSamples = map[string]struct {
	src    string
	callee string // a call that must survive parsing, or "" for none
}{
	"await_empty_args": {
		"async function f() {\n  const actual = await importOriginal<typeof import(\"react-router-dom\")>();\n}\n",
		"importOriginal"},
	// Parsed without error before the workaround, but as `a < b > (c)`.
	"member_with_args": {
		"async function f() {\n  const mod = await vi.importActual<typeof import('./x')>(\"./x\");\n}\n",
		"importActual"},
	"plain_call":     {"const m = g<typeof import(\"x\")>();\n", "g"},
	"member_access":  {"const m = g<typeof import(\"x\").default>();\n", "g"},
	"second_arg":     {"const m = g<string, typeof import(\"x\")>();\n", "g"},
	"type_alias":     {"type A = typeof import(\"x\");\n", ""},
	"annotation":     {"let b: typeof import(\"x\");\n", ""},
	"generic_type":   {"type C = Foo<typeof import(\"x\")>;\n", ""},
	"typeof_ident":   {"g<typeof x>();\n", "g"},
	"dynamic_import": {"const m = await import(\"x\");\n", ""},
}

func TestTypeofImportTypeArguments(t *testing.T) {
	dir := t.TempDir()
	for name, s := range typeofImportSamples {
		for _, ext := range []string{"ts", "tsx"} {
			if err := os.WriteFile(filepath.Join(dir, name+"."+ext), []byte(s.src), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	files, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, s := range typeofImportSamples {
		for _, ext := range []string{"ts", "tsx"} {
			path := name + "." + ext
			f := files[path]
			if f == nil {
				t.Errorf("%s: not scanned", path)
				continue
			}
			if f.Error != nil {
				t.Errorf("%s: valid source reported as %q", path, f.Error.Msg)
			}
			// Node text comes from the original source, not the masked copy.
			if f.Root.Text != s.src {
				t.Errorf("%s: root text = %q, want the original source", path, f.Root.Text)
			}
			if s.callee != "" && !hasCallee(f.Root, s.callee) {
				t.Errorf("%s: no call to %s in the tree", path, s.callee)
			}
		}
	}
}

func hasCallee(n *model.Node, callee string) bool {
	if n.Callee == callee {
		return true
	}
	for _, c := range n.Children {
		if hasCallee(c, callee) {
			return true
		}
	}
	return false
}

func TestMaskTypeofImports(t *testing.T) {
	cases := map[string]string{
		`f<typeof import("x")>()`:       `f<typeof ___________>()`,
		`f<typeof  import ( 'a/b' )>()`: `f<typeof  ________________>()`,
		`f<typeof import("x").T>()`:     `f<typeof ___________.T>()`,
		`await import("x")`:             `await import("x")`,
		`f<typeof imports("x")>()`:      `f<typeof imports("x")>()`,
		`f<notypeof import("x")>()`:     `f<notypeof import("x")>()`,
		"f<typeof import(\n\"x\")>()":   "f<typeof import(\n\"x\")>()",
	}
	for in, want := range cases {
		got := string(maskTypeofImports([]byte(in)))
		if got != want {
			t.Errorf("maskTypeofImports(%q) = %q, want %q", in, got, want)
		}
		if len(got) != len(in) {
			t.Errorf("maskTypeofImports(%q) changed length", in)
		}
	}
}
