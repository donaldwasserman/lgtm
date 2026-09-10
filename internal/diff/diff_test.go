package diff

import (
	"testing"

	"lgtm/internal/model"
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
