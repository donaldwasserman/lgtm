# Changelog

Each version's section becomes its GitHub release notes. Versioning rules are
in [docs/releasing.md](docs/releasing.md#version-numbers).

## Unreleased

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
