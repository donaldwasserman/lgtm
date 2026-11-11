// Package config reads a repository's .lgtm.yml and combines it with the
// built-in defaults and command-line settings.
//
// The file comes from the base side of the pull request, so a change cannot
// loosen the rules it is judged by.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/parse"
)

// FileName is the configuration file, at the root of the base tree.
const FileName = ".lgtm.yml"

// The built-in lists. A configuration adds to any of them with a pattern and
// removes an entry with "!entry".
var (
	// DefaultExclude is left out of the scan entirely.
	DefaultExclude = []string{".git", "node_modules", "vendor", ".venv", "venv", "__pycache__",
		"dist", "build", "target", ".tox", ".idea", ".vscode"}
	// DefaultTestPaths are not counted in breadth or lines added.
	DefaultTestPaths = []string{"*_test.go", "test_*.py", "*_test.py", "conftest.py", "*_test.rb",
		"*_spec.rb", "*.test.*", "*.spec.*", "*Test.java", "*Tests.java", "__tests__",
		"test", "tests", "spec", "testdata"}
	// DefaultUncoveredIgnore can change without counting as uncovered.
	DefaultUncoveredIgnore = []string{"*.md", "*.mdx", "*.rst", "*.txt", "docs", "LICENSE*", "NOTICE*",
		"AUTHORS*", ".gitignore", ".gitattributes", ".editorconfig",
		"*.png", "*.jpg", "*.jpeg", "*.gif", "*.svg", "*.ico", "*.webp",
		"*.css", "*.scss", "*.sass", "*.less"}
	// DefaultStylePaths are stylesheets lgtm can't read as code. They are never
	// uncovered, but the lines they add and the files they touch still count.
	DefaultStylePaths = []string{"*.css", "*.scss", "*.sass", "*.less"}
	// DefaultAlwaysReview always needs review when changed: the workflows
	// that run the gate and the file that configures it.
	DefaultAlwaysReview = []string{".github/workflows", "/" + FileName}
)

// File is the contents of .lgtm.yml.
type File struct {
	Preset          string            `yaml:"preset"`
	Thresholds      map[string]string `yaml:"thresholds"`
	Exclude         []string          `yaml:"exclude"`
	TestPaths       []string          `yaml:"test-paths"`
	UncoveredIgnore []string          `yaml:"uncovered-ignore"`
	StylePaths      []string          `yaml:"style-paths"`
	AlwaysReview    []string          `yaml:"always-review"`
	Overrides       []Override        `yaml:"overrides"`
}

// Override applies stricter thresholds to the whole pull request when it
// changes any of Paths.
type Override struct {
	Name       string            `yaml:"name"`
	Paths      []string          `yaml:"paths"`
	Preset     string            `yaml:"preset"`
	Thresholds map[string]string `yaml:"thresholds"`
}

// Load reads a configuration file. A missing file is an error only when
// required; otherwise it is the empty configuration.
func Load(path string, required bool) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return &File{}, nil
	}
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // a misspelled key must not silently do nothing
	var f File
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// Options are the settings from the command line, which beat the file.
type Options struct {
	Preset string
	// Thresholds are given by name, as in eval.Gate.Set.
	Thresholds map[string]string
	Exclude    []string
}

// Resolved is a configuration ready to use.
type Resolved struct {
	Gate            eval.Gate
	Exclude         *parse.Globs
	TextSkip        *parse.Globs // built-in excludes still in force, for text searches
	Tests           *parse.Globs
	UncoveredIgnore *parse.Globs
	Styles          *parse.Globs
	AlwaysReview    *parse.Globs
	overrides       []resolvedOverride
	pinned          map[string]string
}

type resolvedOverride struct {
	name  string
	paths *parse.Globs
	gate  eval.Gate
}

// Default is the configuration of a repository with no .lgtm.yml.
func Default() *Resolved {
	r, err := (&File{}).Resolve(Options{})
	if err != nil {
		panic(err) // the built-in lists are constants
	}
	return r
}

// Resolve applies the file and then opts over the built-in defaults.
func (f *File) Resolve(opts Options) (*Resolved, error) {
	preset := f.Preset
	if opts.Preset != "" {
		preset = opts.Preset
	}
	gate, err := start(preset)
	if err != nil {
		return nil, err
	}
	if err := setAll(&gate, f.Thresholds); err != nil {
		return nil, err
	}
	final := gate
	if err := setAll(&final, opts.Thresholds); err != nil {
		return nil, err
	}
	if err := final.Validate(); err != nil {
		return nil, err
	}

	r := &Resolved{Gate: gate, pinned: opts.Thresholds}
	exclude, err := merge("exclude", DefaultExclude, append(slices.Clone(f.Exclude), opts.Exclude...))
	if err != nil {
		return nil, err
	}
	var kept []string
	for _, p := range exclude {
		if slices.Contains(DefaultExclude, p) {
			kept = append(kept, p)
		}
	}
	lists := []struct {
		name     string
		defaults []string
		entries  []string
		out      **parse.Globs
	}{
		{"exclude", nil, exclude, &r.Exclude},
		{"exclude", nil, kept, &r.TextSkip},
		{"test-paths", DefaultTestPaths, f.TestPaths, &r.Tests},
		{"uncovered-ignore", DefaultUncoveredIgnore, f.UncoveredIgnore, &r.UncoveredIgnore},
		{"style-paths", DefaultStylePaths, f.StylePaths, &r.Styles},
		{"always-review", DefaultAlwaysReview, f.AlwaysReview, &r.AlwaysReview},
	}
	for _, l := range lists {
		merged, err := merge(l.name, l.defaults, l.entries)
		if err != nil {
			return nil, err
		}
		if *l.out, err = parse.NewGlobs(merged); err != nil {
			return nil, fmt.Errorf("%s: %w", l.name, err)
		}
	}
	for i, o := range f.Overrides {
		ro, err := o.resolve(i)
		if err != nil {
			return nil, err
		}
		r.overrides = append(r.overrides, ro)
	}
	return r, nil
}

func start(preset string) (eval.Gate, error) {
	if preset == "" {
		return eval.DefaultGate, nil
	}
	g, ok := eval.Presets[preset]
	if !ok {
		return eval.Gate{}, fmt.Errorf("unknown preset %q (want balanced or strict)", preset)
	}
	return g, nil
}

func setAll(g *eval.Gate, thresholds map[string]string) error {
	for name, v := range thresholds {
		if err := g.Set(name, v); err != nil {
			return fmt.Errorf("threshold %s: %w", name, err)
		}
	}
	return nil
}

func (o Override) resolve(i int) (resolvedOverride, error) {
	name := o.Name
	if name == "" {
		name = fmt.Sprintf("overrides[%d]", i)
	}
	if len(o.Paths) == 0 {
		return resolvedOverride{}, fmt.Errorf("override %s: paths is required", name)
	}
	if o.Preset == "" && len(o.Thresholds) == 0 {
		return resolvedOverride{}, fmt.Errorf("override %s: set a preset or thresholds", name)
	}
	var gate eval.Gate
	if o.Preset != "" {
		var err error
		if gate, err = start(o.Preset); err != nil {
			return resolvedOverride{}, fmt.Errorf("override %s: %w", name, err)
		}
	}
	for k, v := range o.Thresholds {
		if v == "off" {
			return resolvedOverride{}, fmt.Errorf("override %s: %s cannot be off; overrides only tighten the gate", name, k)
		}
		if err := gate.Set(k, v); err != nil {
			return resolvedOverride{}, fmt.Errorf("override %s: %s: %w", name, k, err)
		}
	}
	if err := gate.Validate(); err != nil {
		return resolvedOverride{}, fmt.Errorf("override %s: %w", name, err)
	}
	paths, err := parse.NewGlobs(o.Paths)
	if err != nil {
		return resolvedOverride{}, fmt.Errorf("override %s: %w", name, err)
	}
	return resolvedOverride{name, paths, gate}, nil
}

// GateFor returns the gate for a pull request changing these paths, and the
// names of the overrides that applied. Command-line thresholds are final.
func (r *Resolved) GateFor(changed []string) (eval.Gate, []string) {
	gate := r.Gate
	applied := []string{}
	for _, o := range r.overrides {
		if slices.ContainsFunc(changed, o.paths.Match) {
			gate = gate.Stricter(o.gate)
			applied = append(applied, o.name)
		}
	}
	// Validated in Resolve, so this cannot fail.
	_ = setAll(&gate, r.pinned)
	return gate, applied
}

// merge applies entries to a built-in list: a pattern is added, and "!entry"
// removes that entry. Removing something that is not listed is an error, so
// a typo cannot silently leave a directory excluded.
func merge(name string, defaults, entries []string) ([]string, error) {
	out := slices.Clone(defaults)
	for _, raw := range entries {
		e := strings.TrimSpace(raw)
		rest, negated := strings.CutPrefix(e, "!")
		if !negated {
			out = append(out, e)
			continue
		}
		i := slices.Index(out, rest)
		if i < 0 {
			return nil, fmt.Errorf("%s: %q removes nothing; %q is not in the list", name, e, rest)
		}
		out = slices.Delete(out, i, i+1)
	}
	return out, nil
}
