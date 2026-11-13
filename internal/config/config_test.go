package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donaldwasserman/lgtm/eval"
)

func load(t *testing.T, yaml string) *File {
	t.Helper()
	p := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p, true)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestMissingFileIsEmptyUnlessRequired(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	if _, err := Load(p, false); err != nil {
		t.Errorf("optional file: %v", err)
	}
	if _, err := Load(p, true); err == nil {
		t.Error("required file: want an error")
	}
}

func TestUnknownKeyIsRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(p, []byte("threshold:\n  cog: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p, true); err == nil {
		t.Error("a misspelled key must be an error")
	}
}

func TestEmptyFileIsDefaults(t *testing.T) {
	r, err := load(t, "").Resolve(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Gate, eval.DefaultGate) {
		t.Errorf("gate = %+v, want the default", r.Gate)
	}
}

func TestPrecedence(t *testing.T) {
	f := load(t, "preset: strict\nthresholds:\n  cog: 9\n  blast: off\n")
	r, err := f.Resolve(Options{Thresholds: map[string]string{"cog": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	g, _ := r.GateFor(nil)
	if *g.ThetaCog != 2 {
		t.Errorf("cog = %d, the flag should win", *g.ThetaCog)
	}
	if g.ThetaBlast != nil {
		t.Error("blast should be off from the file")
	}
	if *g.ThetaModules != *eval.StrictGate.ThetaModules {
		t.Error("modules should come from the preset")
	}
}

func TestExcludeDefaultsAreOverridable(t *testing.T) {
	r := Default()
	if !r.Exclude.Match("internal/build/a.go") || !r.Exclude.Match("node_modules/x/a.js") {
		t.Fatal("built-in excludes missing")
	}
	f := load(t, "exclude:\n  - '!build'\n  - /build\n  - '*.pb.go'\n")
	r, err := f.Resolve(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"internal/build/a.go": false, // the any-depth entry is gone
		"build/a.go":          true,  // re-added at the root only
		"node_modules/x/a.js": true,
		"api/x.pb.go":         true,
	} {
		if got := r.Exclude.Match(path); got != want {
			t.Errorf("Match(%q) = %v, want %v", path, got, want)
		}
	}
	// User patterns never hide a use from the text search; removed built-ins
	// stop being skipped there too.
	if r.TextSkip.Match("api/x.pb.go") || r.TextSkip.Match("internal/build/a.go") {
		t.Error("TextSkip should hold only the built-ins still in force")
	}
	if !r.TextSkip.Match("node_modules/x/a.js") {
		t.Error("TextSkip lost a built-in")
	}
}

func TestRemovingAnAbsentEntryIsAnError(t *testing.T) {
	_, err := load(t, "exclude:\n  - '!bulid'\n").Resolve(Options{})
	if err == nil || !strings.Contains(err.Error(), "bulid") {
		t.Fatalf("err = %v, want one naming the typo", err)
	}
}

func TestOverridesOnlyTighten(t *testing.T) {
	f := load(t, `
overrides:
  - name: payments
    paths: ['services/payments/**']
    preset: strict
  - name: lax
    paths: ['scripts']
    thresholds:
      cog: 50
`)
	r, err := f.Resolve(Options{})
	if err != nil {
		t.Fatal(err)
	}
	g, applied := r.GateFor([]string{"services/payments/a.go", "scripts/x.py"})
	if !reflect.DeepEqual(applied, []string{"payments", "lax"}) {
		t.Errorf("applied = %v", applied)
	}
	if !reflect.DeepEqual(g, eval.StrictGate) {
		t.Errorf("gate = %+v, want strict", g)
	}
	g, applied = r.GateFor([]string{"scripts/x.py"})
	if len(applied) != 1 || *g.ThetaCog != *eval.DefaultGate.ThetaCog {
		t.Errorf("a looser override must not raise cog: %d", *g.ThetaCog)
	}
	g, applied = r.GateFor([]string{"src/a.go"})
	if len(applied) != 0 || !reflect.DeepEqual(g, eval.DefaultGate) {
		t.Errorf("no match should leave the gate alone: %v", applied)
	}
}

func TestOverrideRejectsOff(t *testing.T) {
	_, err := load(t, "overrides:\n  - paths: [a]\n    thresholds:\n      cog: off\n").Resolve(Options{})
	if err == nil {
		t.Error("an override must not switch a threshold off")
	}
}

func TestBadValues(t *testing.T) {
	for _, y := range []string{
		"preset: nope\n",
		"thresholds:\n  cog: lots\n",
		"thresholds:\n  cog: 0\n",
		"thresholds:\n  nope: 1\n",
		"overrides:\n  - preset: strict\n",
		"overrides:\n  - paths: [a]\n",
		"always-review:\n  - '[a'\n",
	} {
		if _, err := load(t, y).Resolve(Options{}); err == nil {
			t.Errorf("%q: want an error", y)
		}
	}
}
