package main

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/donaldwasserman/lgtm/eval"
	"github.com/donaldwasserman/lgtm/internal/config"
	"github.com/donaldwasserman/lgtm/internal/diff"
	"github.com/donaldwasserman/lgtm/internal/metrics"
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

// styleChange is a changed stylesheet: a file lgtm can't read as code but
// whose size still counts.
type styleChange struct {
	path           string
	added, removed int
}

// coverage sorts the changed files lgtm did not parse into stylesheets, those
// the configuration ignores, and those it must count. A stylesheet is never
// uncovered; the lines it adds and the files it touches are counted instead.
func coverage(others []parse.Unscanned, cfg *config.Resolved, base, head string) (files []*fileSummary, uncovered []string, styles []styleChange) {
	for _, o := range others {
		state := coverageUncovered
		switch {
		case cfg.Styles.Match(o.Path) && !cfg.Tests.Match(o.Path):
			state = coverageStyle
			added, removed := metrics.TextChange(readText(base, o.Path), readText(head, o.Path))
			if added+removed > 0 {
				styles = append(styles, styleChange{o.Path, added, removed})
			}
		case cfg.UncoveredIgnore.Match(o.Path):
			state = coverageIgnored
		default:
			uncovered = append(uncovered, o.Path)
		}
		files = append(files, &fileSummary{Path: o.Path, Kind: string(o.Kind), Coverage: state})
	}
	return files, uncovered, styles
}

// readText returns a file's text, or "" when it is missing or unreadable (a
// file added or deleted by the change).
func readText(root, rel string) string {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	return string(b)
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
