package main

import (
	"sort"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/config"
	"github.com/donaldwasserman/lgtm/internal/diff"
	"github.com/donaldwasserman/lgtm/internal/model"
	"github.com/donaldwasserman/lgtm/internal/parse"
	"github.com/donaldwasserman/lgtm/internal/significance"
	"github.com/donaldwasserman/lgtm/internal/symbols"
)

// maxFilesListed bounds the per-file contributor lists in the report.
const maxFilesListed = 50

// fileRef names one file behind a score.
type fileRef struct {
	File string `json:"file"`
}

// moduleRef names one package behind a score.
type moduleRef struct {
	Symbol string `json:"symbol"`
}

// tree is one side of the pull request.
type tree struct {
	files map[string]*model.File
	syms  *symbols.Table
}

// substantiveFiles lists the changed non-test files whose change could alter
// behaviour: a symbol changed in more than name, a symbol appeared, or
// top-level code outside any symbol changed. A sweep of comments, formatting,
// local renames or moves counts for nothing.
func substantiveFiles(res *diff.Result, rated []significance.Rated, pairs []symbols.Pair, base, head tree, tests *parse.Globs) []string {
	set := map[string]bool{}
	for _, r := range rated {
		if r.Level == eval.None {
			continue
		}
		if r.Pair.Head != nil {
			set[r.Pair.Head.File] = true
		} else {
			set[r.Symbol().File] = true
		}
	}
	for _, p := range pairs {
		if p.Base == nil {
			set[p.Head.File] = true
		}
	}
	for _, fc := range res.Files {
		if base.syms.Residual(base.files[fc.Path]) != head.syms.Residual(head.files[fc.Path]) {
			set[fc.Path] = true
		}
	}
	out := make([]string, 0, len(set))
	for f := range set {
		if !tests.Match(f) {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// coverage sorts the changed files lgtm did not parse into those the
// configuration ignores and those it must count.
func coverage(others []parse.Unscanned, cfg *config.Resolved) (files []*fileSummary, uncovered []string) {
	for _, o := range others {
		state := coverageUncovered
		if cfg.UncoveredIgnore.Match(o.Path) {
			state = coverageIgnored
		} else {
			uncovered = append(uncovered, o.Path)
		}
		files = append(files, &fileSummary{Path: o.Path, Kind: string(o.Kind), Coverage: state})
	}
	return files, uncovered
}

func fileRefs(paths []string) []fileRef {
	out := []fileRef{}
	for _, p := range paths {
		if len(out) == maxFilesListed {
			break
		}
		out = append(out, fileRef{p})
	}
	return out
}

func moduleRefs(modules []string) []moduleRef {
	out := []moduleRef{}
	for _, m := range modules {
		if len(out) == maxFilesListed {
			break
		}
		out = append(out, moduleRef{m})
	}
	return out
}
