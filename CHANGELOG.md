# Changelog

Each version's section becomes its GitHub release notes. Versioning rules are
in [docs/releasing.md](docs/releasing.md#version-numbers).

## 0.0.6

Changes how the gate decides, and the report format (`schemaVersion` 4). See
[docs/adr/0005-new-code-is-not-wide-and-blast-counts-logic.md](https://github.com/donaldwasserman/lgtm/blob/v0.0.6/docs/adr/0005-new-code-is-not-wide-and-blast-counts-logic.md).

- Files and modules touched count only files whose existing code changed. New
  files and new symbols no longer count, so a feature built from new code is
  judged by lines added and new-function complexity.
- Blast radius counts only callers of changes of at least medium significance
  (a condition, error handling, a signature or more). A statement or constant
  edit in a widely called function no longer has a blast radius.
  `calledSignificance` is unchanged.
- New score, test lines removed (`scores.testLinesRemoved`), and threshold
  `test-deletions` (`--theta-test-deletions`, action input
  `theta-test-deletions`): 100 in `balanced`, 50 in `strict`. Lines removed from
  test files and fixtures count unless they appear again in a test file, so
  moving tests is free and deleting them en masse requires review.

## 0.0.5

Changes how the gate decides, and the report format (`schemaVersion` 3).

- Removed the trusted-author exemption: `--trusted` and the action's
  `trust-mode`, `trust-count`, `trust-percent` and `author` inputs. Deciding who
  may skip review is a separate problem.
- Edit depth is off in the default gate; it is still reported. New default gate:
  files 15, modules 4, lines added 1000, uncovered files 1, complexity increase
  5, new-function complexity 25, significance `crucial`, blast radius 30.
- `.lgtm.yml`, read from the base commit so a pull request cannot loosen its own
  gate. It sets a preset (`balanced` or `strict`), thresholds, path lists and
  path overrides that tighten the gate.
- The directories lgtm skips (`node_modules`, `build`, ...) are now a default
  list. Add to it, or remove an entry with `!build`.
- New score, lines added. New score, uncovered files: a changed file lgtm cannot
  read (another language, SQL, YAML, ...) now requires review unless ignored.
- New always-review paths; by default `.github/workflows` and `.lgtm.yml`.
- Files and modules touched count only substantive changes outside tests.
  `--epsilon-trivial` is gone, and `scores.depthTotal` with it.
- Module-level constants and variables are now symbols, so a changed value
  counts in significance (low) and blast radius.
- An approval counts only on the latest commit and from someone with write
  access. Reviews from anyone else no longer block either.
- Every score names what is behind it, with file and line, and the check
  annotates those places for each rule that fired.
- Removed `facts.analysisFailed`, which nothing ever set. A crash already fails
  the check.

## 0.0.4

- `--exclude` (action input `exclude`) leaves paths matching a glob out of
  the scan. Repeat the flag, or put one pattern per line in the input.

## 0.0.3

Faster runs; results are unchanged.

- Files that are identical on both sides of the diff are parsed once
  instead of twice.

## 0.0.2

Changes how the gate decides, and the report format (`schemaVersion` 2).

- One score per measure: edit depth, files and modules touched, the
  largest cognitive-complexity increase of an existing function, the
  complexity of the most complex new function, significance of changes to
  existing symbols (signature, visibility, supertypes, conditions, error
  handling), called significance, and blast radius over the code that
  calls what changed.
- Every threshold is a flag (`--theta-*`) and an action input, and each
  can be switched `off`. `0` is rejected. Unparsed or unanalyzable code
  always requires review, even for trusted authors.
- The report gains `reasons`, `facts`, `gate` and `contributors` (the
  symbols behind each score); `metrics` became `scores` and `thresholds`
  became `gate`. The check summary lists every score.
- Fixed: `lgtm` crashed on mutually recursive functions, and call depth
  could undercount at random where two call paths share a callee.
- Fixed: TypeScript files that pass an inline import type to a call, such
  as `importOriginal<typeof import("x")>()`, were reported as unparseable
  or misread as comparisons.

## 0.0.1

First release.

- Command-line tool: compares a branch with its target (`--repo`,
  `--base-ref`) or two folders (`--base`, `--head`) and reports whether the
  change needs review, as an exit code and a JSON report (`schemaVersion` 1).
  Supports Go, Python, Ruby, JavaScript, TypeScript, Java, and Rust.
- GitHub Action: posts an `LGTM / review-gate` check that stays red on big
  changes until someone approves. Runs the analysis in a Docker image.
  Optional labels, reviewer requests, and trusted-author modes (`codeowners`,
  `top-count`, `top-percent`).
- Docker image at `ghcr.io/donaldwasserman/lgtm` (amd64, arm64), and
  prebuilt binaries for Linux and macOS.
