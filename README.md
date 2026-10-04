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

`lgtm` reads the code before and after the change and reports **one score per
measure**. The gate (one threshold per measure) turns those scores into "needs
review" or not. You set the thresholds; every one can be switched `off`.

| Measure | Plain meaning | Default threshold |
| --- | --- | --- |
| **Edit depth** | How deep the change reaches into code that already existed | 7 |
| **Files touched** | How many files changed | 6, if total depth is above 1 |
| **Modules touched** | How many packages changed | off |
| **Complexity increase** | The most any existing function got harder to read ([cognitive complexity](https://www.sonarsource.com/docs/CognitiveComplexity.pdf)) | 5 |
| **New-function complexity** | How hard to read the most complex new function is | 25 |
| **Significance** | The riskiest kind of change to existing code: none, low (statements), medium (conditions, error handling), high (an exported signature), crucial (an export removed, a superclass changed) | crucial |
| **Blast radius** | How many symbols call what changed, up to three calls away | 50 |

While the significance threshold is on, an exported signature change to
something that has callers also requires review.

Some rules can't be switched off:

- **A file it can't parse always requires review.** No measurement means no free pass.
- **Code it fails to analyze always requires review**, for the same reason.
- **Trusted authors skip every threshold** (off by default), but never the two
  rules above. You choose who counts as trusted — see
  [Trust modes](docs/action.md#trust-modes).

New code is judged only by new-function complexity (and file and module
counts): a narrow addition of readable code passes. Deleting a private
function nothing uses doesn't count against you either.

The gate is written down in a formal model and the code is tested against
it. See [docs/development.md](docs/development.md) for the details, and
[GLOSSARY.md](GLOSSARY.md) for the vocabulary.

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

      - uses: donaldwasserman/lgtm@v0
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
  ghcr.io/donaldwasserman/lgtm:0 --repo /repo --base-ref origin/main

# Prebuilt binary (Linux and macOS): pick lgtm_<version>_<os>_<arch>.tar.gz
# from https://github.com/donaldwasserman/lgtm/releases, e.g.
curl -fsSL https://github.com/donaldwasserman/lgtm/releases/download/v0.0.1/lgtm_0.0.1_linux_amd64.tar.gz | tar -xz lgtm

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
| `--theta-depth` | `7` | Edit depth that requires review |
| `--theta-breadth` | `6` | Files touched that require review (with total depth above `--epsilon-trivial`) |
| `--epsilon-trivial` | `1` | Total depth at or below which a wide change is too small to matter |
| `--theta-modules` | `off` | Modules touched that require review |
| `--theta-cog` | `5` | Increase in one existing function's cognitive complexity that requires review |
| `--theta-new-function` | `25` | Cognitive complexity of one new function that requires review |
| `--theta-significance` | `crucial` | Significance level (`low`, `medium`, `high`, `crucial`) that requires review |
| `--theta-blast` | `50` | Blast radius that requires review |
| `--trusted` | `false` | Treat the author as trusted (skips review). You decide who that is |
| `--output` | | Also write the report to this file |
| `--exit-zero` | `false` | Exit `0` whatever the verdict; only errors exit non-zero |
| `--version` | | Print the version |

### Result

**Exit code:** `0` no review needed, `1` review needed, `2` error (including a
branch that can't be found). With `--exit-zero`, read `verdict` from the
report instead.

Every `--theta-*` flag takes a number or `off`. `0` is rejected: it would
flag every change.

The report is JSON on standard output:

Here `lib.Parse` gained a parameter, and `app.Run` calls it (abridged):

```console
$ lgtm --base before/ --head after/
{
  "schemaVersion": 2,
  "verdict": "review-required",
  "requiresReview": true,
  "reasons": ["called-exported-signature"],
  "scores": { "editDepth": 3, "newDepth": 4, "depthTotal": 4, "breadthFiles": 1,
              "breadthModules": 1, "cogDelta": 0, "newFunctionComplexity": 0,
              "significance": "high", "calledSignificance": "high", "blastRadius": 1 },
  "facts": { "trusted": false, "unparsed": false, "analysisFailed": false },
  "gate": { "thetaDepth": 7, "thetaBreadth": 6, "epsilonTrivial": 1, "thetaModules": null,
            "thetaCog": 5, "thetaNewFunction": 25, "thetaSignificance": "crucial", "thetaBlast": 50 },
  "contributors": {
    "significance": [ { "symbol": "lib Parse", "file": "lib/lib.go", "level": "high", "parts": ["signature"] } ],
    "blastRadius": [ { "symbol": "lib Parse", "file": "lib/lib.go", "value": 1 } ]
  },
  "files": [ { "path": "lib/lib.go", "kind": "modify" } ]
}
```

| Field | Meaning |
| --- | --- |
| `schemaVersion` | `2`. Goes up only if a field is renamed, removed, or changes meaning |
| `verdict` | `"review-required"` or `"no-review"` |
| `requiresReview` | Same, as `true`/`false` |
| `reasons` | Every rule that required review (`edit-depth`, `breadth-files`, `breadth-modules`, `cog-delta`, `new-function-complexity`, `significance`, `called-exported-signature`, `blast-radius`, `unparsed`, `analysis-failed`) |
| `scores` | One score per measure. `null` means the measure isn't available for the change's languages, which never requires review |
| `facts` | Inputs to the rules that can't be switched off |
| `gate` | The thresholds used. `null` means switched off |
| `contributors` | The functions and symbols behind each score, highest first |
| `parseErrors` | Files that couldn't be parsed. Only present if there are any; forces review |
| `files` | Files that changed, and how (`insert`, `delete`, `modify`) |

### Other CI systems

`lgtm` only looks at code. Approvals, trusted authors, and posting results are
up to the caller. For example:

```bash
git fetch --no-tags origin main
docker run --rm --network none --user "$(id -u):$(id -g)" \
  -v "$PWD:/repo:ro" -v "$PWD/out:/out" \
  ghcr.io/donaldwasserman/lgtm:0 \
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
