# Changelog

Each version's section becomes its GitHub release notes. Versioning rules are
in [docs/releasing.md](docs/releasing.md#version-numbers).

## 1.0.0

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
