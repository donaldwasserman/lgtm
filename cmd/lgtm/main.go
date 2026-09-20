// Command lgtm compares a base and head source tree with Tree-sitter,
// computes PR-review complexity metrics, and emits a JSON decision matching
// the Alloy-grounded eval.RequiresReview logic. Exit code 0 means review is
// not required; non-zero (1) means review is required (suitable for gating a
// GitHub Action).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"lgtm/eval"
	"lgtm/internal/diff"
	"lgtm/internal/metrics"
	"lgtm/internal/model"
	"lgtm/internal/parse"
)

type report struct {
	RequiresReview bool            `json:"requiresReview"`
	Metrics        eval.Metrics    `json:"metrics"`
	Thresholds     eval.Thresholds `json:"thresholds"`
	ParseErrors    []parseError    `json:"parseErrors,omitempty"`
	Files          []*fileSummary  `json:"files"`
}

// parseError names a file that could not be parsed. Any entry here forces
// review: the metrics no longer describe the whole change.
type parseError struct {
	Path   string `json:"path"`
	Side   string `json:"side"` // "base" or "head"
	Reason string `json:"reason"`
}

type fileSummary struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("lgtm", flag.ExitOnError)
	base := fs.String("base", "", "path to base (pre-PR) source tree")
	head := fs.String("head", "", "path to head (post-PR) source tree")
	thetaDepth := fs.Int("theta-depth", 7, "high depth threshold")
	thetaBreadth := fs.Int("theta-breadth", 6, "high breadth threshold")
	epsilonTrivial := fs.Int("epsilon-trivial", 1, "below-depth trivial epsilon")
	trusted := fs.Bool("trusted", false, "submitter is a trusted contributor (exempts review); the caller decides who is trusted")
	help := fs.Bool("help", false, "show usage")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lgtm --base <dir> --head <dir> [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Compares two source trees and reports whether the change requires review.\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if *help || *base == "" || *head == "" {
		fs.Usage()
		if *help {
			return 0
		}
		return 2
	}

	thr := eval.Thresholds{
		ThetaDepth:     *thetaDepth,
		ThetaBreadth:   *thetaBreadth,
		EpsilonTrivial: *epsilonTrivial,
	}

	out, requiresReview, err := analyze(*base, *head, thr, *trusted)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	fmt.Println(string(data))

	if requiresReview {
		return 1
	}
	return 0
}

// analyze scans both trees, diffs them, computes metrics, and returns the
// JSON report plus the review decision.
func analyze(base, head string, thr eval.Thresholds, trusted bool) (report, bool, error) {
	ctx := context.Background()
	baseFiles, err := parse.Scan(ctx, base)
	if err != nil {
		return report{}, false, err
	}
	headFiles, err := parse.Scan(ctx, head)
	if err != nil {
		return report{}, false, err
	}

	parseErrs := append(
		collectParseErrors("base", baseFiles),
		collectParseErrors("head", headFiles)...)

	res := diff.Diff(baseFiles, headFiles)
	m := metrics.Compute(res, trusted, len(parseErrs) > 0)
	decision := eval.RequiresReview(m, thr)

	rep := report{
		RequiresReview: decision,
		Metrics:        m,
		Thresholds:     thr,
		ParseErrors:    parseErrs,
	}
	for _, fc := range res.Files {
		rep.Files = append(rep.Files, &fileSummary{Path: fc.Path, Kind: string(fc.Kind)})
	}
	return rep, decision, nil
}

// collectParseErrors lists the files on one side that could not be parsed,
// sorted by path so the report is deterministic across runs.
func collectParseErrors(side string, files map[string]*model.File) []parseError {
	var out []parseError
	for _, f := range files {
		if f.Error == nil {
			continue
		}
		out = append(out, parseError{Path: f.Path, Side: side, Reason: f.Error.Msg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
