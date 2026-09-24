package parse

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// sampleByExt holds one syntactically valid snippet per supported extension.
// The test below fails if a spec declares an extension with no sample here, so
// adding a language forces adding coverage for it.
var sampleByExt = map[string]string{
	"go":   "package p\n\nfunc f() {}\n",
	"py":   "def f():\n    pass\n",
	"rb":   "def f\nend\n",
	"java": "class A { void f() {} }\n",
	"rs":   "fn f() -> i32 { 1 }\n",
	"js":   "function f() { return 1; }\n",
	"mjs":  "export function f() { return 1; }\n",
	"cjs":  "module.exports = function f() { return 1; };\n",
	"jsx":  "export default function C() {\n  return <div className=\"c\">hi</div>;\n}\n",
	"ts":   "function f(): number { return 1; }\n",
	"mts":  "export function f(): number { return 1; }\n",
	"cts":  "export function f(): number { return 1; }\n",
	"tsx":  "type P = { t: string };\n\nexport default function C({ t }: P) {\n  return <div className=\"c\">{t}</div>;\n}\n",
}

func TestAllSupportedLanguagesParse(t *testing.T) {
	dir := t.TempDir()

	var names []string
	for _, spec := range specs {
		for ext := range spec.Exts {
			src, ok := sampleByExt[ext]
			if !ok {
				t.Errorf("language %q declares .%s but sampleByExt has no sample for it", spec.ID, ext)
				continue
			}
			name := "sample_" + ext + "." + ext
			if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
	}

	files, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		f := files[name]
		if f == nil {
			t.Errorf("%s: file not scanned", name)
			continue
		}
		if f.Root == nil {
			t.Errorf("%s: no root node", name)
		}
		// The assertion that matters: valid source must not be flagged as a
		// syntax error. Every .tsx file failed here, because .tsx was mapped to
		// the TypeScript grammar, which rejects JSX.
		if f.Error != nil {
			t.Errorf("%s: valid source reported as %q - wrong grammar for this extension?",
				name, f.Error.Msg)
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
