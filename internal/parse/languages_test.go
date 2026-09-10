package parse

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAllSupportedLanguagesParse(t *testing.T) {
	dir := t.TempDir()
	samples := map[string]string{
		"s.go":   "package p\nfunc f(){}\n",
		"s.py":   "def f():\n    pass\n",
		"s.rb":   "def f\nend\n",
		"s.java": "class A { void f() {} }",
		"s.js":   "function f() { return 1; }",
		"s.ts":   "function f(): number { return 1; }",
	}
	for name, content := range samples {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for name := range samples {
		f := files[name]
		if f == nil {
			t.Errorf("%s: file not scanned", name)
			continue
		}
		if f.Root == nil {
			t.Errorf("%s: no root node (error=%v)", name, f.Error)
		}
	}
}

func TestUnsupportedFileSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# hi"), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files for unsupported ext, got %v", files)
	}
}
