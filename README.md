# lgtm

[![Release](https://img.shields.io/github/v/release/donaldwasserman/lgtm)](https://github.com/donaldwasserman/lgtm/releases)
[![Marketplace](https://img.shields.io/badge/marketplace-LGTM%20Review%20Gate-blue?logo=github)](https://github.com/marketplace/actions/lgtm-review-gate)
[![CI](https://github.com/donaldwasserman/lgtm/actions/workflows/ci.yml/badge.svg)](https://github.com/donaldwasserman/lgtm/actions/workflows/ci.yml)

`lgtm` looks at a pull request and decides whether it needs a human review.

Small, shallow changes pass on their own. Changes that dig deep into existing
code, or spread across many files, are flagged and stay blocked until someone
approves them.

It runs as a GitHub Action (the usual way) or as a command-line tool.

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

      - name: Get the code before the change
        env:
          BASE_REF: ${{ github.event.pull_request.base.ref }}
        run: |
          git fetch --no-tags origin "$BASE_REF"
          mkdir -p "$RUNNER_TEMP/lgtm/base"
          git archive "$(git merge-base "origin/$BASE_REF" HEAD)" | tar -x -C "$RUNNER_TEMP/lgtm/base"

      - uses: donaldwasserman/lgtm@v1
        with:
          base: ${{ runner.temp }}/lgtm/base
          head: ${{ github.workspace }}
```

The action posts a check called **`LGTM / review-gate`** on every pull request:

- **Green** — the change is small, *or* it is big and someone approved it.
- **Red** — the change is big and nobody has approved it yet (or `lgtm` itself
  failed). It turns green on its own once someone approves.

To actually block merges, make that check required in your branch protection
settings — but only after you've seen it work on a real pull request.

Full reference (all options, what each check result means, trust modes,
troubleshooting): **[docs/action.md](docs/action.md)**.

## Command-line tool

Install one of these ways:

```bash
# Prebuilt binary (Linux and macOS): pick lgtm_<version>_<os>_<arch>.tar.gz
# from https://github.com/donaldwasserman/lgtm/releases, e.g.
curl -fsSL https://github.com/donaldwasserman/lgtm/releases/download/v1.0.0/lgtm_1.0.0_linux_amd64.tar.gz | tar -xz lgtm

# Docker
docker run --rm -v "$PWD:/src" ghcr.io/donaldwasserman/lgtm:1 --base /src/base --head /src/head

# From source (needs Go 1.25+ and a C compiler)
go install github.com/donaldwasserman/lgtm/cmd/lgtm@latest
```

`lgtm` compares two folders, not two git commits. Put the "before" code in one
folder and point `lgtm` at both:

```bash
BASE=$(git merge-base origin/main HEAD)
mkdir -p /tmp/lgtm-base
git archive "$BASE" | tar -x -C /tmp/lgtm-base
lgtm --base /tmp/lgtm-base --head .
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--base` | *(required)* | Folder with the code before the change |
| `--head` | *(required)* | Folder with the code after the change |
| `--theta-depth` | `7` | Edit depth that forces review |
| `--theta-breadth` | `6` | Number of files that forces review |
| `--epsilon-trivial` | `1` | Total depth at or below which a change is too small to matter |
| `--trusted` | `false` | Treat the author as trusted (skips review). You decide who that is |
| `--version` | | Print the version |

**Exit code:** `0` no review needed, `1` review needed, `2` error. The JSON
report always goes to standard output:

```console
$ lgtm --base /tmp/base --head /tmp/head
{
  "requiresReview": false,
  "metrics": { "DepthMod": 6, "DepthNew": 1, "Breadth": 1, "DepthTotal": 6,
               "Trusted": false, "Unparsed": false },
  "thresholds": { "ThetaDepth": 7, "ThetaBreadth": 6, "EpsilonTrivial": 1 },
  "files": [ { "path": "a.go", "kind": "modify" } ]
}
```

`files` lists only files that changed. Files that could not be read appear
under `parseErrors` and force `requiresReview: true`.

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
