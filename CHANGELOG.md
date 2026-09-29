# Changelog

Each version's section becomes its GitHub release notes. Versioning rules are
in [docs/releasing.md](docs/releasing.md#version-numbers).

## 1.0.0

First release.

- Command-line tool: compares two source trees and reports whether the change
  needs review. Supports Go, Python, Ruby, JavaScript, TypeScript, Java, and Rust.
- GitHub Action: posts an `LGTM / review-gate` check that stays red on big
  changes until someone approves. Optional labels, reviewer requests, and
  trusted-author modes (`codeowners`, `top-count`, `top-percent`).
- Prebuilt binaries for Linux and macOS, and a Docker image at
  `ghcr.io/donaldwasserman/lgtm`.
