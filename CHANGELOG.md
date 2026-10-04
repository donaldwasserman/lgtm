# Changelog

Each version's section becomes its GitHub release notes. Versioning rules are
in [docs/releasing.md](docs/releasing.md#version-numbers).

## 0.0.1

First release.

- Command-line tool: compares a branch with its target (`--repo`,
  `--base-ref`) or two folders (`--base`, `--head`) and reports one score per
  measure, plus whether the change needs review under the configured gate, as
  an exit code and a JSON report (`schemaVersion` 2). Supports Go, Python,
  Ruby, JavaScript, TypeScript, Java, and Rust.
- Measures: edit depth, files and modules touched, cognitive complexity
  increase of existing functions and complexity of new ones, significance of
  changes to existing symbols (signature, visibility, supertypes, conditions,
  error handling), and blast radius over the code that calls what changed.
  Every threshold can be set or switched `off`; unparsed or unanalyzable code
  always requires review. The report names the symbols behind each score.
- GitHub Action: posts an `LGTM / review-gate` check that stays red on big
  changes until someone approves. Runs the analysis in a Docker image.
  Optional labels, reviewer requests, and trusted-author modes (`codeowners`,
  `top-count`, `top-percent`).
- Docker image at `ghcr.io/donaldwasserman/lgtm` (amd64, arm64), and
  prebuilt binaries for Linux and macOS.
