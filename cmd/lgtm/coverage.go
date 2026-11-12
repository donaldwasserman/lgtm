package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// substantiveFiles lists the changed non-test files whose existing code
// changed in a way that could alter behaviour: an existing symbol changed in
// more than name, or top-level code outside any symbol changed in a file that
// already existed. A sweep of comments, formatting, local renames or moves
// counts for nothing, and so does new code: new files and new symbols are
// judged by lines added and new-function complexity, so a feature built
// mostly from new code is not "wide" however many files it adds.
func substantiveFiles(res *diff.Result, rated []significance.Rated, base, head tree, tests *parse.Globs) []string {
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
	for _, fc := range res.Files {
		if fc.Kind == model.KindInsert {
			continue
		}
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

// blastFloor is the lowest significance whose change counts toward blast
// radius: conditions, error handling and anything stronger.
const blastFloor = eval.Medium

// testFiles reads the changed files on test paths from both trees, parsed or
// not, keyed by path. A file missing on one side is absent from that map.
func testFiles(base, head string, res *diff.Result, others []parse.Unscanned, tests *parse.Globs) (map[string][]byte, map[string][]byte, error) {
	var paths []string
	for _, fc := range res.Files {
		paths = append(paths, fc.Path)
	}
	for _, o := range others {
		paths = append(paths, o.Path)
	}
	b, h := map[string][]byte{}, map[string][]byte{}
	for _, p := range paths {
		if !tests.Match(p) {
			continue
		}
		for _, side := range []struct {
			root string
			out  map[string][]byte
		}{{base, b}, {head, h}} {
			data, err := os.ReadFile(filepath.Join(side.root, filepath.FromSlash(p)))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			side.out[p] = data
		}
	}
	return b, h, nil
}
