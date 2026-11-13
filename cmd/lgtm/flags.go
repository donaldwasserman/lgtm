package main

import (
	"flag"
	"strings"

	"github.com/donaldwasserman/lgtm/eval"
)

// thresholdFlag records a threshold given on the command line: a number, a
// significance level, or "off". Set rejects a value of the wrong shape; the
// gate as a whole is validated once the configuration is resolved.
type thresholdFlag struct {
	name   string
	pinned map[string]string
}

func (f thresholdFlag) String() string { return f.pinned[f.name] }

func (f thresholdFlag) Set(s string) error {
	var g eval.Gate
	if err := g.Set(f.name, s); err != nil {
		return err
	}
	f.pinned[f.name] = s
	return nil
}

// stringsFlag collects a repeatable string flag.
type stringsFlag []string

func (f *stringsFlag) String() string { return strings.Join(*f, ",") }

func (f *stringsFlag) Set(s string) error {
	*f = append(*f, s)
	return nil
}

// thresholdHelp describes each threshold, keyed by its eval.Gate name.
var thresholdHelp = map[string]string{
	"depth":        "edit depth",
	"breadth":      "non-test files with a substantive change",
	"modules":      "packages those files belong to",
	"lines":        "non-blank lines added to non-test source files",
	"uncovered":    "changed files no measure can look at",
	"cog":          "increase in one existing function's cognitive complexity",
	"new-function": "cognitive complexity of one new function",
	"significance": "significance level (low, medium, high, crucial)",
	"blast":        "blast radius (callers within 3 hops)",
}

// gateFlags registers one --theta-* flag per threshold.
func gateFlags(fs *flag.FlagSet, pinned map[string]string) {
	for _, name := range eval.ThresholdNames {
		fs.Var(thresholdFlag{name, pinned}, "theta-"+name,
			thresholdHelp[name]+` at which review is required, or "off" (default: from the preset)`)
	}
}
