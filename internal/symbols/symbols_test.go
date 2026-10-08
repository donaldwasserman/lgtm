package symbols

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/donaldwasserman/lgtm/internal/parse"
)

func scan(t *testing.T, files map[string]string) *Table {
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
	return Extract(parsed)
}

// describe renders each symbol as "id kind" plus " exported" when exported.
func describe(tab *Table) []string {
	var out []string
	for _, s := range tab.Symbols {
		d := s.ID() + " " + string(s.Kind)
		if s.Exported {
			d += " exported"
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func TestExtract(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"go", map[string]string{"pkg/a.go": `package p

type T struct{ Base }

type i interface {
	M(a int) error
	unexported()
}

func (t *T) Get(a int) int { return a }
func (t T) set() {}
func helper[K comparable](k K) int { return 0 }
func Public() {}
`}, []string{
			"pkg|T|Get method exported",
			"pkg|T|set method",
			"pkg||Public func exported",
			"pkg||T type exported",
			"pkg||helper func",
			"pkg||i type",
			"pkg|i|M method",
			"pkg|i|unexported method",
		}},
		{"python", map[string]string{"app/models.py": `__all__ = ["Pub", "helper"]

class Pub(Base):
    def method(self): pass
    def _hidden(self): pass
    def __init__(self): pass

class Other:
    def method(self): pass

@decorator
def helper(): pass

def _private(): pass
`}, []string{
			"app.models|Pub|__init__ method exported",
			"app.models|Pub|_hidden method",
			"app.models|Pub|method method exported",
			"app.models|Other|method method",
			"app.models||Other type",
			"app.models||Pub type exported",
			"app.models||_private func",
			"app.models||helper func exported",
		}},
		{"python_init_without_all", map[string]string{"pkg/__init__.py": "def f(): pass\ndef _g(): pass\n"},
			[]string{"pkg||_g func", "pkg||f func exported"}},
		{"ruby", map[string]string{"lib/foo.rb": `module Outer
  class Foo < Bar::Base
    def pub; end
    def later_private; end
    private :later_private

    private

    def hidden; end
    def self.klass; end

    public
    def again; end
    private def inline; end
    protected
    def prot; end
  end
end

def top_level; end
`}, []string{
			"Object||Outer module exported",
			"Object||top_level method exported",
			"Outer::Foo||again method exported",
			"Outer::Foo||hidden method",
			"Outer::Foo||inline method",
			"Outer::Foo||later_private method",
			"Outer::Foo||prot method",
			"Outer::Foo||pub method exported",
			"Outer::Foo||self.klass method exported",
			"Outer||Foo type exported",
		}},
		{"javascript", map[string]string{"src/a.js": `export function exported(a) {}
function local() {}
function viaClause() {}
class K extends Base { #priv() {} method(x) {} }
const arrow = () => 1;
module.exports = { local };
exports.other = function () {};
export { viaClause };
export default class D {}
`}, []string{
			"src/a|K|#priv method",
			"src/a|K|method method",
			"src/a||D type exported",
			"src/a||K type",
			"src/a||arrow func",
			"src/a||exported func exported",
			"src/a||local func exported",
			"src/a||other func exported",
			"src/a||viaClause func exported",
		}},
		{"typescript", map[string]string{"src/c.ts": `export class C<T> extends B implements I {
  private p(x: number): string { return ""; }
  protected r(): void {}
  public q(): void {}
}
export interface I { m(a: string): void }
function f(a: number): number { return a; }
export const g = (x: number) => x;
`}, []string{
			"src/c|C|p method",
			"src/c|C|q method exported",
			"src/c|C|r method",
			"src/c|I|m method exported",
			"src/c||C type exported",
			"src/c||I type exported",
			"src/c||f func",
			"src/c||g func exported",
		}},
		{"java", map[string]string{"src/main/java/com/ex/A.java": `package com.ex;

public class A extends B implements I {
    public int m(int a) { return a; }
    public int m(String a) { return 0; }
    void pkg() {}
    protected static class Inner { private void x() {} }
}
interface I { void n(); }
`}, []string{
			"com.ex|A.Inner|x method",
			"com.ex|A|Inner type exported",
			"com.ex|A|m method exported",
			"com.ex|A|m method exported",
			"com.ex|A|pkg method",
			"com.ex|I|n method",
			"com.ex||A type exported",
			"com.ex||I type",
		}},
		{"rust", map[string]string{"src/net/mod.rs": `pub mod inner { pub fn f() {} fn g() {} }
mod private_mod { pub fn h() {} }
pub struct S;
pub trait Tr { fn m(&self); }
impl Tr for S { fn m(&self) {} }
impl S { pub fn new() -> S { S } fn helper(&self) {} }
pub(crate) fn crate_fn() {}
fn private_fn() {}
`}, []string{
			"net::inner||f func exported",
			"net::inner||g func",
			"net::private_mod||h func",
			"net|S|helper method",
			"net|S|m method exported",
			"net|S|new method exported",
			"net|Tr|m method exported",
			"net||S type exported",
			"net||Tr type exported",
			"net||crate_fn func exported",
			"net||inner module exported",
			"net||private_fn func",
			"net||private_mod module",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := describe(scan(t, c.files))
			want := append([]string(nil), c.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("symbols:\n got  %q\n want %q", got, want)
			}
		})
	}
}

// changed lists the IDs of pairs that differ, marking adds and deletes.
func changed(base, head *Table) []string {
	var out []string
	for _, p := range Compare(base, head) {
		switch {
		case p.Base == nil:
			out = append(out, "+"+p.Head.ID())
		case p.Head == nil:
			out = append(out, "-"+p.Base.ID())
		case p.Changed():
			out = append(out, "~"+p.Head.ID())
		}
	}
	sort.Strings(out)
	return out
}

func TestCompare(t *testing.T) {
	cases := []struct {
		name       string
		base, head map[string]string
		want       []string
	}{
		{
			// ADR 0001: a move inside one Go package is not a change.
			name: "go_move_within_package",
			base: map[string]string{"p/a.go": "package p\nfunc Parse() int { return 1 }\n", "p/b.go": "package p\n"},
			head: map[string]string{"p/a.go": "package p\n", "p/b.go": "package p\nfunc Parse() int { return 1 }\n"},
			want: nil,
		},
		{
			name: "go_move_across_packages",
			base: map[string]string{"p/a.go": "package p\nfunc Parse() int { return 1 }\n"},
			head: map[string]string{"q/a.go": "package q\nfunc Parse() int { return 1 }\n"},
			want: []string{"+q||Parse", "-p||Parse"},
		},
		{
			name: "comments_and_whitespace_only",
			base: map[string]string{"a.go": "package p\nfunc F() int { return 1 }\n"},
			head: map[string]string{"a.go": "package p\n\n// F returns one.\nfunc F() int {\n\treturn 1 // always\n}\n"},
			want: nil,
		},
		{
			name: "body_edit",
			base: map[string]string{"a.go": "package p\nfunc F() int { return 1 }\n"},
			head: map[string]string{"a.go": "package p\nfunc F() int { return 2 }\n"},
			want: []string{"~.||F"},
		},
		{
			// Editing a method changes the method, not its class.
			name: "member_edit_leaves_class_unchanged",
			base: map[string]string{"m.py": "class A:\n    def f(self):\n        return 1\n"},
			head: map[string]string{"m.py": "class A:\n    def f(self):\n        return 2\n"},
			want: []string{"~m|A|f"},
		},
		{
			name: "export_change_alone",
			base: map[string]string{"m.py": "def f(): pass\n"},
			head: map[string]string{"m.py": "__all__ = []\ndef f(): pass\n"},
			want: []string{"~m||f"},
		},
		{
			// A Ruby class reopened in another file is the same module, so a
			// method moved between the two files is unchanged.
			name: "ruby_open_class_move",
			base: map[string]string{
				"a.rb": "class Foo\n  def bar\n    1\n  end\nend\n",
				"b.rb": "class Foo\n  def baz; end\nend\n",
			},
			head: map[string]string{
				"a.rb": "class Foo\nend\n",
				"b.rb": "class Foo\n  def baz; end\n  def bar\n    1\n  end\nend\n",
			},
			want: nil,
		},
		{
			// Overloads pair by parameter list, so editing one overload does
			// not read as changing the other.
			name: "java_overloads",
			base: map[string]string{"A.java": "class A { int m(int a) { return a; } int m(String s) { return 0; } }\n"},
			head: map[string]string{"A.java": "class A { int m(int a) { return a + 1; } int m(String s) { return 0; } }\n"},
			want: []string{"~.|A|m"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := changed(scan(t, c.base), scan(t, c.head))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("changes = %q, want %q", got, c.want)
			}
		})
	}
}

func TestModulesTouched(t *testing.T) {
	tab := scan(t, map[string]string{
		"a/x.go": "package a\n", "a/y.go": "package a\n", "b/z.go": "package b\n",
		"src/A.java": "package com.ex;\nclass A {}\n", "other/B.java": "package com.ex;\nclass B {}\n",
	})
	if got := ModulesTouched([]string{"a/x.go", "a/y.go"}, tab, tab); got != 1 {
		t.Errorf("one Go package: got %d", got)
	}
	if got := ModulesTouched([]string{"a/x.go", "b/z.go"}, tab, tab); got != 2 {
		t.Errorf("two Go packages: got %d", got)
	}
	// Java modules are packages, wherever the files sit.
	if got := ModulesTouched([]string{"src/A.java", "other/B.java"}, tab, tab); got != 1 {
		t.Errorf("one Java package in two directories: got %d", got)
	}
}
