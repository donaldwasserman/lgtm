package parse

import "testing"

func TestExcludeMatch(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"*.pb.go", "api/x.pb.go", true},
		{"*.pb.go", "x.go", false},
		{"testdata", "a/b/testdata/c.go", true},
		{"testdata", "a/testdata2/c.go", false},
		{"docs/gen", "docs/gen/a.go", true},
		{"docs/gen", "src/docs/gen/a.go", false},
		{"/docs", "docs/a.go", true},
		{"/docs", "src/docs/a.go", false},
		{"src/**/*.min.js", "src/a/b/x.min.js", true},
		{"src/**/*.min.js", "src/x.min.js", true},
		{"src/**/*.min.js", "lib/x.min.js", false},
		{"generated/", "generated/a.go", true},
		{"**/gen/**", "a/gen/b/c.go", true},
	}
	for _, c := range cases {
		e, err := NewExclude([]string{c.pattern})
		if err != nil {
			t.Fatal(err)
		}
		if got := e.Match(c.path); got != c.want {
			t.Errorf("%q vs %q = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestExcludeBadPattern(t *testing.T) {
	if _, err := NewExclude([]string{"[a"}); err == nil {
		t.Error("want error for malformed glob")
	}
}

func TestNilExclude(t *testing.T) {
	var e *Exclude
	if e.Match("a.go") {
		t.Error("nil Exclude matched")
	}
}
