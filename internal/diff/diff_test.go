package diff

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"lgtm/internal/model"
	"lgtm/internal/parse"
)

// small model helpers (bypass the parser for pure diff tests)
func leaf(typ string, start, end uint32) *model.Node {
	return &model.Node{Type: typ, Start: start, End: end}
}

func TestDiffClassifiesInsertDeleteModify(t *testing.T) {
	base := &model.Node{Type: "root", Text: "base version"}
	head := &model.Node{Type: "root", Text: "head version"}

	// params: identifier "keep" unchanged, "removed" deleted, "added" inserted
	keep := &model.Node{Type: "identifier", Name: "keep", Text: "keep"}
	rem := &model.Node{Type: "identifier", Name: "removed", Text: "removed"}
	base.Children = []*model.Node{
		{Type: "declaration", Name: "d1", Text: "keep removed",
			Children: []*model.Node{keep, rem}},
	}
	add := &model.Node{Type: "identifier", Name: "added", Text: "added"}
	keepH := &model.Node{Type: "identifier", Name: "keep", Text: "keep"}
	head.Children = []*model.Node{
		{Type: "declaration", Name: "d1", Text: "keep added",
			Children: []*model.Node{keepH, add}},
	}

	baseFile := &model.File{Path: "f.go", Root: base}
	headFile := &model.File{Path: "f.go", Root: head}

	res := Diff(map[string]*model.File{"f.go": baseFile}, map[string]*model.File{"f.go": headFile})
	if len(res.Files) != 1 {
		t.Fatalf("expected 1 file change, got %d", len(res.Files))
	}
	fc := res.Files[0]
	if fc.Kind != model.KindModify {
		t.Fatalf("file kind = %v, want modify", fc.Kind)
	}
	// Expect modify on declaration/d1, delete on "removed", insert on "added".
	found := map[string]bool{}
	for _, c := range fc.Changes {
		switch {
		case c.Name == "declaration" || c.Type == "declaration":
			if c.Kind != model.KindModify {
				t.Errorf("declaration kind = %v, want modify", c.Kind)
			}
			found["declaration"] = true
		case c.Name == "removed":
			if c.Kind != model.KindDelete {
				t.Errorf("removed kind = %v, want delete", c.Kind)
			}
			found["removed"] = true
		case c.Name == "added":
			if c.Kind != model.KindInsert {
				t.Errorf("added kind = %v, want insert", c.Kind)
			}
			found["added"] = true
		case c.Name == "keep":
			t.Errorf("unchanged keep was reported in changes")
		}
	}
	for _, want := range []string{"declaration", "removed", "added"} {
		if !found[want] {
			t.Errorf("missing expected change %q; got %d changes", want, len(fc.Changes))
		}
	}
}

func TestDiffNewFileAllInsert(t *testing.T) {
	h := &model.File{Path: "new.go", Root: &model.Node{Type: "root", Text: "abc",
		Children: []*model.Node{leaf("identifier", 0, 3)}}}
	res := Diff(map[string]*model.File{}, map[string]*model.File{"new.go": h})
	if len(res.Files) != 1 || res.Files[0].Kind != model.KindInsert {
		t.Fatalf("expected single insert file, got %+v", res.Files)
	}
	if len(res.Files[0].Changes) == 0 {
		t.Fatal("expected node changes for inserted file")
	}
}

// TestDiffSkipsUnchangedFiles guards the breadth metric: a file present on both
// sides with an identical AST must not be reported as a change, otherwise every
// file in the repo counts toward breadth.
func TestDiffSkipsUnchangedFiles(t *testing.T) {
	tree := func() *model.Node {
		return &model.Node{Type: "root", Text: "package p\nfunc f(){}\n",
			Children: []*model.Node{{Type: "declaration", Name: "f", Text: "func f(){}"}}}
	}
	base := map[string]*model.File{"f.go": {Path: "f.go", Root: tree()}}
	head := map[string]*model.File{"f.go": {Path: "f.go", Root: tree()}}

	res := Diff(base, head)
	if len(res.Files) != 0 {
		t.Fatalf("identical file reported as changed: %+v", res.Files)
	}
}

// TestDiffReportsOnlyChangedFileAmongUnchanged mixes one real change into a set
// of identical files and asserts only the changed path survives.
func TestDiffReportsOnlyChangedFileAmongUnchanged(t *testing.T) {
	same := func() *model.Node {
		return &model.Node{Type: "root", Text: "same", Children: []*model.Node{
			{Type: "declaration", Name: "u", Text: "u"}}}
	}
	base := map[string]*model.File{
		"changed.go": {Path: "changed.go", Root: &model.Node{Type: "root", Text: "before"}},
		"u1.go":      {Path: "u1.go", Root: same()},
		"u2.go":      {Path: "u2.go", Root: same()},
	}
	head := map[string]*model.File{
		"changed.go": {Path: "changed.go", Root: &model.Node{Type: "root", Text: "after"}},
		"u1.go":      {Path: "u1.go", Root: same()},
		"u2.go":      {Path: "u2.go", Root: same()},
	}

	res := Diff(base, head)
	if len(res.Files) != 1 {
		t.Fatalf("expected 1 changed file, got %d: %+v", len(res.Files), res.Files)
	}
	if res.Files[0].Path != "changed.go" {
		t.Fatalf("reported %q, want changed.go", res.Files[0].Path)
	}
}

// TestDiffIgnoresByteOffsetShifts guards against aligning nodes by byte offset.
// Editing one literal shifts the offset of everything after it, which used to
// leave the untouched statements below unmatched and record each as a delete
// plus an insert.
func TestDiffIgnoresByteOffsetShifts(t *testing.T) {
	const (
		before = "package p\n\nfunc F() int {\n\tx := 1\n\ty := 2\n\tz := 3\n\treturn x + y + z\n}\n"
		after  = "package p\n\nfunc F() int {\n\tx := 11\n\ty := 2\n\tz := 3\n\treturn x + y + z\n}\n"
	)
	base, head := t.TempDir(), t.TempDir()
	for dir, src := range map[string]string{base: before, head: after} {
		if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}

	baseFiles, err := parse.Scan(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	headFiles, err := parse.Scan(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}

	// Changing one token may only ever modify the chain of ancestors above it.
	for _, fc := range Diff(baseFiles, headFiles).Files {
		for _, c := range fc.Changes {
			if c.Kind != model.KindModify {
				t.Errorf("got %s of %s; a one-token edit must not insert or delete siblings",
					c.Kind, c.Type)
			}
		}
	}
}
