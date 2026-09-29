// Command lgtm compares a base and head source tree with Tree-sitter,
// computes PR-review complexity metrics, and emits a JSON decision matching
// the Alloy-grounded eval.RequiresReview logic.
//
// The trees come either from two directories (--base/--head) or from a git
// checkout (--repo/--base-ref), in which case lgtm extracts the merge base
// and the head commit itself.
//
// Exit codes are the interface for CI: 0 means review is not required, 1
// means review is required, and anything else means lgtm failed and produced
// no verdict. --exit-zero reports both verdicts as 0, leaving the decision to
// the report's "verdict" field.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/diff"
	"github.com/donaldwasserman/lgtm/internal/metrics"
	"github.com/donaldwasserman/lgtm/internal/model"
	"github.com/donaldwasserman/lgtm/internal/parse"
)

// schemaVersion is bumped whenever a field of report is renamed, removed or
// changes meaning. Adding a field does not bump it.
const schemaVersion = 1

const (
	verdictReviewRequired = "review-required"
	verdictNoReview       = "no-review"
)

type report struct {
	SchemaVersion  int             `json:"schemaVersion"`
	Verdict        string          `json:"verdict"`
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

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lgtm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "", "path to base (pre-PR) source tree")
	head := fs.String("head", "", "path to head (post-PR) source tree")
	repo := fs.String("repo", "", "path to a git checkout; use with --base-ref instead of --base/--head")
	baseRef := fs.String("base-ref", "", "git ref the change targets; the base tree is its merge base with --head-ref")
	headRef := fs.String("head-ref", "HEAD", "git ref of the change itself")
	thetaDepth := fs.Int("theta-depth", 7, "high depth threshold")
	thetaBreadth := fs.Int("theta-breadth", 6, "high breadth threshold")
	epsilonTrivial := fs.Int("epsilon-trivial", 1, "below-depth trivial epsilon")
	trusted := fs.Bool("trusted", false, "submitter is a trusted contributor (exempts review); the caller decides who is trusted")
	output := fs.String("output", "", "also write the JSON report to this file")
	exitZero := fs.Bool("exit-zero", false, "exit 0 whatever the verdict; failures still exit non-zero")
	showVersion := fs.Bool("version", false, "print the version and exit")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lgtm --base <dir> --head <dir> [flags]\n")
		fmt.Fprintf(stderr, "       lgtm --repo <dir> --base-ref <ref> [--head-ref <ref>] [flags]\n\n")
		fmt.Fprintf(stderr, "Compares two source trees and reports whether the change requires review.\n")
		fmt.Fprintf(stderr, "Exits 0 when review is not required, 1 when it is, and 2 on failure.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}

	dirMode := *base != "" || *head != ""
	gitMode := *repo != "" || *baseRef != ""
	switch {
	case dirMode && gitMode:
		fmt.Fprintln(stderr, "error: use either --base/--head or --repo/--base-ref, not both")
		return 2
	case dirMode && (*base == "" || *head == ""):
		fmt.Fprintln(stderr, "error: --base and --head must be given together")
		return 2
	case gitMode && (*repo == "" || *baseRef == ""):
		fmt.Fprintln(stderr, "error: --repo and --base-ref must be given together")
		return 2
	case !dirMode && !gitMode:
		fs.Usage()
		return 2
	}

	if gitMode {
		trees, err := extractTrees(*repo, *baseRef, *headRef)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
		defer trees.cleanup()
		*base, *head = trees.base, trees.head
	}

	thr := eval.Thresholds{
		ThetaDepth:     *thetaDepth,
		ThetaBreadth:   *thetaBreadth,
		EpsilonTrivial: *epsilonTrivial,
	}

	out, requiresReview, err := analyze(*base, *head, thr, *trusted)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	fmt.Fprintln(stdout, string(data))

	if *output != "" {
		// A report that was printed but not saved is still a failure: the
		// caller asked for the file and would otherwise read a stale or
		// missing one.
		if err := os.WriteFile(*output, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
	}

	if requiresReview && !*exitZero {
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

	verdict := verdictNoReview
	if decision {
		verdict = verdictReviewRequired
	}
	rep := report{
		SchemaVersion:  schemaVersion,
		Verdict:        verdict,
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
