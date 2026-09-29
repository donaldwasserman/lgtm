# lgtm

[![Release](https://img.shields.io/github/v/release/donaldwasserman/lgtm)](https://github.com/donaldwasserman/lgtm/releases)
[![Marketplace](https://img.shields.io/badge/marketplace-LGTM%20Review%20Gate-blue?logo=github)](https://github.com/marketplace/actions/lgtm-review-gate)
[![CI](https://github.com/donaldwasserman/lgtm/actions/workflows/ci.yml/badge.svg)](https://github.com/donaldwasserman/lgtm/actions/workflows/ci.yml)

`lgtm` looks at a pull request and decides whether it needs a human review.

Small, shallow changes pass on their own. Changes that dig deep into existing
code, or spread across many files, are flagged and stay blocked until someone
approves them.

It runs as a GitHub Action (the usual way), a Docker image, or a command-line
tool.

## How it decides

`lgtm` reads the code before and after the change, compares the two, and
measures:

| Measure | Plain meaning | Flags review when |
| --- | --- | --- |
| **Depth of edits** | How deeply the change reaches into code that already existed | at or above `theta-depth` (default 7) |
| **Breadth** | How many files changed | at or above `theta-breadth` (default 6), *and* total depth is above `epsilon-trivial` (default 1) |
| **Depth of new code** | How deeply new code nests | never — new code alone does not force review |

Two overrides:

- **A file it can't read always forces review.** No measurement means no free pass.
- **Trusted authors skip review** (off by default). You choose who counts as
  trusted — see [Trust modes](docs/action.md#trust-modes).

The full rule, for reference:

```
needs review = unreadable file
               OR (not trusted AND (edit depth >= theta-depth
                                    OR (breadth >= theta-breadth AND total depth > epsilon-trivial)))
```

This rule is written down in a formal model and the code is tested against
it. See [docs/development.md](docs/development.md) if you want the details.

## Quick start: GitHub Action

Save this as `.github/workflows/lgtm.yml` in your repository:

```yaml
name: lgtm

on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
  pull_request_review:            # re-checks when someone approves
    types: [submitted, dismissed]

permissions:
  contents: read
  checks: write                   # post the pass/fail check
  pull-requests: write            # read reviews, add labels

jobs:
  lgtm:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          ref: ${{ github.event.pull_request.head.sha }}

      - name: Fetch the target branch
        env:
          BASE_REF: ${{ github.event.pull_request.base.ref }}
        run: git fetch --no-tags origin "$BASE_REF"

      - uses: donaldwasserman/lgtm@v1
        with:
          base-ref: origin/${{ github.event.pull_request.base.ref }}
```

The action posts a check called **`LGTM / review-gate`** on every pull request:

- **Green** — the change is small, *or* it is big and someone approved it.
- **Red** — the change is big and nobody has approved it yet (or `lgtm` itself
  failed). It turns green on its own once someone approves.

To actually block merges, make that check required in your branch protection
settings — but only after you've seen it work on a real pull request.

Full reference (all options, what each check result means, trust modes,
troubleshooting): **[docs/action.md](docs/action.md)**.

## Command line and Docker

`lgtm` compares the code before and after a change. Give it either a git
checkout and the branch the change targets, or two folders:

```bash
lgtm --repo . --base-ref origin/main       # compare this branch with main
lgtm --base old/ --head new/               # compare two folders
```

With `--base-ref`, it compares against the point where the branch split from
`origin/main`. It never fetches anything, so fetch the full history and the
target branch first.

### Install

```bash
# Docker (Linux, amd64 and arm64). Mount the checkout read-only; no network needed.
docker run --rm --network none -v "$PWD:/repo:ro" \
  ghcr.io/donaldwasserman/lgtm:1 --repo /repo --base-ref origin/main

# Prebuilt binary (Linux and macOS): pick lgtm_<version>_<os>_<arch>.tar.gz
# from https://github.com/donaldwasserman/lgtm/releases, e.g.
curl -fsSL https://github.com/donaldwasserman/lgtm/releases/download/v1.0.0/lgtm_1.0.0_linux_amd64.tar.gz | tar -xz lgtm

# From source (needs Go 1.25+ and a C compiler)
go install github.com/donaldwasserman/lgtm/cmd/lgtm@latest
```

Git mode needs `git` installed; the Docker image includes it.

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--repo` | | Git checkout to analyze (use with `--base-ref`) |
| `--base-ref` | | Branch or commit the change targets, e.g. `origin/main` |
| `--head-ref` | `HEAD` | The change itself |
| `--base` | | Folder with the code before the change (use with `--head`) |
| `--head` | | Folder with the code after the change |
| `--theta-depth` | `7` | Edit depth that forces review |
| `--theta-breadth` | `6` | Number of files that forces review |
| `--epsilon-trivial` | `1` | Total depth at or below which a change is too small to matter |
| `--trusted` | `false` | Treat the author as trusted (skips review). You decide who that is |
| `--output` | | Also write the report to this file |
| `--exit-zero` | `false` | Exit `0` whatever the verdict; only errors exit non-zero |
| `--version` | | Print the version |

### Result

**Exit code:** `0` no review needed, `1` review needed, `2` error (including a
branch that can't be found). With `--exit-zero`, read `verdict` from the
report instead.

The report is JSON on standard output:

```console
$ lgtm --repo . --base-ref origin/main
{
  "schemaVersion": 1,
  "verdict": "no-review",
  "requiresReview": false,
  "metrics": { "DepthMod": 6, "DepthNew": 1, "Breadth": 1, "DepthTotal": 6,
               "Trusted": false, "Unparsed": false },
  "thresholds": { "ThetaDepth": 7, "ThetaBreadth": 6, "EpsilonTrivial": 1 },
  "files": [ { "path": "a.go", "kind": "modify" } ]
}
```

| Field | Meaning |
| --- | --- |
| `schemaVersion` | `1`. Goes up only if a field is renamed, removed, or changes meaning |
| `verdict` | `"review-required"` or `"no-review"` |
| `requiresReview` | Same, as `true`/`false` |
| `metrics` | The measurements above (`DepthMod` = depth of edits, `DepthNew` = depth of new code) |
| `thresholds` | The limits used |
| `parseErrors` | Files that couldn't be read. Only present if there are any; forces review |
| `files` | Files that changed, and how (`insert`, `delete`, `modify`) |

### Other CI systems

`lgtm` only looks at code. Approvals, trusted authors, and posting results are
up to the caller. For example:

```bash
git fetch --no-tags origin main
docker run --rm --network none --user "$(id -u):$(id -g)" \
  -v "$PWD:/repo:ro" -v "$PWD/out:/out" \
  ghcr.io/donaldwasserman/lgtm:1 \
  --repo /repo --base-ref origin/main --output /out/lgtm.json --exit-zero
jq -r .verdict out/lgtm.json
```

`--user` lets the container write the report into a folder your CI user owns.

## Supported languages

| Language | Extensions |
| --- | --- |
| Go | `.go` |
| Python | `.py` |
| Ruby | `.rb` |
| JavaScript | `.js` `.mjs` `.cjs` `.jsx` |
| TypeScript | `.ts` `.mts` `.cts` `.tsx` |
| Java | `.java` |
| Rust | `.rs` |

Other files are ignored, as are `.git`, `node_modules`, `vendor`, `target`,
`.venv`, `venv`, and `__pycache__` folders.

## More

- [docs/action.md](docs/action.md) — GitHub Action reference
- [docs/development.md](docs/development.md) — building, testing, and the formal model
- [docs/releasing.md](docs/releasing.md) — versions and how to publish one
- [CHANGELOG.md](CHANGELOG.md) — what changed in each version

MIT licensed. See [LICENSE](LICENSE).
