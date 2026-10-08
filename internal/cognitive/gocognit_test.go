package cognitive

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/uudashr/gocognit"

	"github.com/donaldwasserman/lgtm/internal/parse"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// TestAgreesWithGocognit compares every Go function in this repository and
// in a few standard-library packages against gocognit, an independent
// implementation of the same paper for Go. Any difference must be one of
// the deliberate choices documented in the package comment.
func TestAgreesWithGocognit(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dirs := []string{root}
	for _, pkg := range []string{"strings", "sort", "strconv", "bufio", "encoding/json", "text/template/parse"} {
		dirs = append(dirs, filepath.Join(runtime.GOROOT(), "src", pkg))
	}
	compared := 0
	for _, dir := range dirs {
		compared += compareDir(t, dir)
	}
	if compared < 500 {
		t.Fatalf("compared only %d functions; the corpus went missing", compared)
	}
	t.Logf("compared %d functions", compared)
}

func compareDir(t *testing.T, dir string) int {
	t.Helper()
	parsed, err := parse.Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ours := map[string]int{}       // "file:Container.Name" -> score
	selfCalls := map[string]bool{} // functions that call themselves
	for _, s := range symbols.Extract(parsed).Symbols {
		if s.Lang != "go" {
			continue
		}
		if v, ok := Of(s); ok {
			key := s.File + ":" + qualified(s.Container, s.Name)
			ours[key] = v
			selfCalls[key] = callsItself(s)
		}
	}
	n := 0
	for rel, f := range parsed {
		if f.Lang != "go" || f.Error != nil || strings.Contains(rel, "testdata") {
			continue
		}
		fset := token.NewFileSet()
		src, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, rel, src, 0)
		if err != nil {
			continue
		}
		for _, d := range af.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == "_" {
				continue
			}
			key := rel + ":" + qualified(receiver(fn), fn.Name.Name)
			got, ok := ours[key]
			if !ok {
				t.Errorf("%s: no score from lgtm", key)
				continue
			}
			// Recursion is scored differently on purpose (see the package
			// comment), so functions that call themselves are skipped. So
			// are methods making a bare call to their own name: that is a
			// builtin or conversion (append, recover, Kind(x)), which
			// gocognit miscounts as recursion.
			if selfCalls[key] || (fn.Recv != nil && bareCall(fn)) {
				continue
			}
			if want := gocognit.Complexity(fn); got != want {
				t.Errorf("%s/%s: lgtm %d, gocognit %d", dir, key, got, want)
			}
			n++
		}
	}
	return n
}

// callsItself reports whether the walker scored recursion for s.
func callsItself(s *symbols.Symbol) bool {
	w := newWalker(s)
	w.visit(symbols.Body(s.Node), 0)
	return w.recursive
}

// bareCall reports whether fn's body calls an identifier named like fn.
func bareCall(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if id, ok := c.Fun.(*ast.Ident); ok && id.Name == fn.Name.Name {
				found = true
			}
		}
		return !found
	})
	return found
}

func qualified(container, name string) string {
	if container == "" {
		return name
	}
	return container + "." + name
}

func receiver(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}
