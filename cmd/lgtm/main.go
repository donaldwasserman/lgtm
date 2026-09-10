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

	"lgtm/eval"
	"lgtm/internal/diff"
	"lgtm/internal/metrics"
	"lgtm/internal/parse"
)

type report struct {
	RequiresReview bool            `json:"requiresReview"`
	Metrics        eval.Metrics    `json:"metrics"`
	Thresholds     eval.Thresholds `json:"thresholds"`
	Files          []*fileSummary  `json:"files"`
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
	topTwenty := fs.Bool("top-twenty", false, "submitter is a trusted top-20% contributor (exempts review)")
	help := fs.Bool("help", false, "show usage")
	_ = fs.String("languages", "", "comma-separated language ids (v1: supported automatically by extension)")

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

	if env := os.Getenv("LGTM_TOP_TWENTY"); env != "" && !*topTwenty {
		*topTwenty = env == "1" || env == "true"
	}

	out, requiresReview, err := analyze(*base, *head, thr, *topTwenty)
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
func analyze(base, head string, thr eval.Thresholds, topTwenty bool) (report, bool, error) {
	ctx := context.Background()
	baseFiles, err := parse.Scan(ctx, base)
	if err != nil {
		return report{}, false, err
	}
	headFiles, err := parse.Scan(ctx, head)
	if err != nil {
		return report{}, false, err
	}

	res := diff.Diff(baseFiles, headFiles)
	m := metrics.Compute(res, topTwenty)
	decision := eval.RequiresReview(m, thr)

	rep := report{
		RequiresReview: decision,
		Metrics:        m,
		Thresholds:     thr,
	}
	for _, fc := range res.Files {
		rep.Files = append(rep.Files, &fileSummary{Path: fc.Path, Kind: string(fc.Kind)})
	}
	return rep, decision, nil
}
