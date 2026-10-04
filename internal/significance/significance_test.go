package significance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/parse"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

func table(t *testing.T, files map[string]string) *symbols.Table {
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
	for p, f := range parsed {
		if f.Error != nil {
			t.Fatalf("%s: %s", p, f.Error.Msg)
		}
	}
	return symbols.Extract(parsed)
}

type sigCase struct {
	file       string
	base, head string
	want       eval.Level
	parts      string // comma-separated, sorted; empty to skip the check
}

func run(t *testing.T, cases map[string]sigCase) {
	t.Helper()
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := Measure(symbols.Compare(
				table(t, map[string]string{c.file: c.base}),
				table(t, map[string]string{c.file: c.head})), nil)
			if r.Score != c.want {
				t.Errorf("level = %v, want %v (rated %+v)", r.Score, c.want, describe(r))
			}
			if c.parts != "" && len(r.Rated) > 0 {
				var ps []string
				for _, p := range r.Rated[0].Parts {
					ps = append(ps, string(p))
				}
				if got := strings.Join(ps, ","); got != c.parts {
					t.Errorf("parts = %s, want %s", got, c.parts)
				}
			}
		})
	}
}

func describe(r Result) []string {
	var out []string
	for _, x := range r.Rated {
		var ps []string
		for _, p := range x.Parts {
			ps = append(ps, string(p))
		}
		out = append(out, x.Symbol().ID()+"="+x.Level.String()+"["+strings.Join(ps, ",")+"]")
	}
	return out
}

func TestGo(t *testing.T) {
	run(t, map[string]sigCase{
		"private_statement": {"a.go",
			"package a\nfunc f(x int) int { y := x; return y }\n",
			"package a\nfunc f(x int) int { y := x + 1; return y }\n", eval.Low, "statement"},
		"rename_locals_and_params": {"a.go",
			"package a\nfunc F(x int) int { y := x * 2; return y }\n",
			"package a\nfunc F(n int) int { doubled := n * 2; return doubled }\n", eval.None, "rename"},
		// A new local declared early must not make the untouched condition
		// below it look changed.
		"added_local_before_condition": {"a.go",
			"package a\nfunc f(x int) int { y := x; if y > 0 { return 1 }; return 0 }\n",
			"package a\nfunc f(x int) int { z := 2; y := x; if y > 0 { return z }; return 0 }\n", eval.Low, "statement"},
		"rename_plus_edit_is_an_edit": {"a.go",
			"package a\nfunc f(x int) int { y := x; return y }\n",
			"package a\nfunc f(x int) int { w := x; return w + 1 }\n", eval.Low, "statement"},
		"condition": {"a.go",
			"package a\nfunc f(x int) int { if x > 0 { return 1 }; return 0 }\n",
			"package a\nfunc f(x int) int { if x >= 0 { return 1 }; return 0 }\n", eval.Medium, "condition"},
		"error_flow": {"a.go",
			"package a\nfunc f(x int) { if x < 0 { panic(\"neg\") } }\n",
			"package a\nfunc f(x int) { if x < 0 { panic(\"negative\") } }\n", eval.Medium, "error-flow"},
		"private_signature": {"a.go",
			"package a\nfunc f(x int) int { return x }\n",
			"package a\nfunc f(x int64) int { return int(x) }\n", eval.Medium, ""},
		"exported_signature": {"a.go",
			"package a\nfunc F(x int) int { return x }\n",
			"package a\nfunc F(x int, y int) int { return x }\n", eval.High, "signature"},
		"exported_result": {"a.go",
			"package a\nfunc F() int { return 0 }\n",
			"package a\nfunc F() (int, error) { return 0, nil }\n", eval.High, ""},
		// Renaming an exported function removes the export.
		"exported_rename": {"a.go",
			"package a\nfunc Old() {}\n", "package a\nfunc New() {}\n", eval.Crucial, "visibility"},
		"exported_deleted": {"a.go",
			"package a\nfunc Gone() {}\nfunc keep() {}\n", "package a\nfunc keep() {}\n", eval.Crucial, ""},
		"private_deleted": {"a.go",
			"package a\nfunc gone() {}\nfunc keep() {}\n", "package a\nfunc keep() {}\n", eval.Medium, "signature"},
		"embedding_changed": {"a.go",
			"package a\ntype T struct { Base\n x int }\n",
			"package a\ntype T struct { Other\n x int }\n", eval.Crucial, "supertypes"},
		"struct_field_added": {"a.go",
			"package a\ntype T struct { x int }\n",
			"package a\ntype T struct { x int\n y int }\n", eval.Low, "statement"},
		"new_function_not_rated": {"a.go",
			"package a\n", "package a\nfunc New(x int) int { if x > 0 { return 1 }; return 0 }\n", eval.None, ""},
	})
}

func TestOtherLanguages(t *testing.T) {
	run(t, map[string]sigCase{
		"python_superclass": {"m.py",
			"class A(Base):\n    pass\n", "class A(Other):\n    pass\n", eval.Crucial, "supertypes"},
		"python_made_private_by_all": {"m.py",
			"def f(): pass\n", "__all__ = []\ndef f(): pass\n", eval.Crucial, "visibility"},
		"python_decorator": {"m.py",
			"@cache\ndef f(x):\n    return x\n", "@lru_cache(10)\ndef f(x):\n    return x\n", eval.Medium, "unclassified"},
		"python_except": {"m.py",
			"def f():\n    try:\n        g()\n    except ValueError:\n        pass\n",
			"def f():\n    try:\n        g()\n    except KeyError:\n        pass\n", eval.Medium, "error-flow"},
		"python_typed_param_rename": {"m.py",
			"def f(x: int) -> int:\n    return x\n", "def f(y: int) -> int:\n    return y\n", eval.None, "rename"},
		"ruby_include": {"a.rb",
			"class A\n  include M\n  def f; end\nend\n", "class A\n  include N\n  def f; end\nend\n", eval.Crucial, "supertypes"},
		"ruby_made_private": {"a.rb",
			"class A\n  def f; end\nend\n", "class A\n  private\n  def f; end\nend\n", eval.Crucial, "visibility"},
		"ruby_method_moves_between_reopened_files": {"a.rb",
			"class A\n  def f\n    1\n  end\nend\n", "class A\n  def f\n    1\n  end\nend\n", eval.None, ""},
		"js_export_removed": {"a.js",
			"export function f() {}\n", "function f() {}\n", eval.Crucial, "visibility"},
		"js_cjs_signature": {"a.js",
			"function f(a) { return a; }\nmodule.exports = { f };\n",
			"function f(a, b) { return a; }\nmodule.exports = { f };\n", eval.High, "signature"},
		"ts_private_method_signature": {"a.ts",
			"export class C { private p(x: number) {} }\n", "export class C { private p(x: string) {} }\n", eval.Medium, ""},
		"ts_extends": {"a.ts",
			"export class C extends A {}\n", "export class C extends B {}\n", eval.Crucial, "supertypes"},
		"java_public_to_private": {"A.java",
			"public class A { public void m() {} }\n", "public class A { private void m() {} }\n", eval.Crucial, "visibility"},
		"java_static_added": {"A.java",
			"public class A { public void m() {} }\n", "public class A { public static void m() {} }\n", eval.High, "signature"},
		"java_throws": {"A.java",
			"public class A { public void m() {} }\n", "public class A { public void m() throws E {} }\n", eval.High, ""},
		"java_catch": {"A.java",
			"class A { void m() { try { x(); } catch (A e) {} } }\n",
			"class A { void m() { try { x(); } catch (B e) {} } }\n", eval.Medium, "error-flow"},
		"java_param_rename": {"A.java",
			"public class A { public int m(int a) { return a; } }\n", "public class A { public int m(int b) { return b; } }\n", eval.None, "rename"},
		"rust_pub_removed": {"src/lib.rs",
			"pub fn f() {}\n", "fn f() {}\n", eval.Crucial, "visibility"},
		"rust_trait_bound": {"src/lib.rs",
			"pub trait T: A {}\n", "pub trait T: B {}\n", eval.Crucial, "supertypes"},
		"rust_question_mark": {"src/lib.rs",
			"fn f() -> R { let x = g(); Ok(x) }\n", "fn f() -> R { let x = g()?; Ok(x) }\n", eval.Medium, "error-flow"},
		"rust_let_rename": {"src/lib.rs",
			"pub fn f(a: i32) -> i32 { let b = a + 1; b }\n", "pub fn f(x: i32) -> i32 { let y = x + 1; y }\n", eval.None, "rename"},
	})
}

func TestExemptDeletionNotRated(t *testing.T) {
	base := table(t, map[string]string{"a.go": "package a\nfunc dead() {}\n"})
	head := table(t, map[string]string{"a.go": "package a\n"})
	r := Measure(symbols.Compare(base, head), func(*symbols.Symbol) bool { return true })
	if r.Score != eval.None || len(r.Rated) != 0 {
		t.Errorf("exempt deletion rated: %v", describe(r))
	}
}
