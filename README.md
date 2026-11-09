# lgtm

[![Release](https://img.shields.io/github/v/release/donaldwasserman/lgtm)](https://github.com/donaldwasserman/lgtm/releases)
[![Marketplace](https://img.shields.io/badge/marketplace-LGTM%20Review%20Gate-blue?logo=github)](https://github.com/marketplace/actions/lgtm-review-gate)
[![CI](https://github.com/donaldwasserman/lgtm/actions/workflows/ci.yml/badge.svg)](https://github.com/donaldwasserman/lgtm/actions/workflows/ci.yml)

`lgtm` looks at a pull request and decides whether it needs a human review.

Small, contained changes pass on their own. Changes that make existing code
harder to follow, break what depends on it, spread across the codebase, or touch
something `lgtm` can't read are flagged and stay blocked until someone approves
them.

It runs as a GitHub Action (the usual way), a Docker image, or a command-line
tool.

## How it decides

`lgtm` reads the code before and after the change and reports **one score per
measure**. The gate (one threshold per measure) turns those scores into "needs
review" or not. You set the thresholds; every one can be switched `off`.

| Measure | Plain meaning | Default threshold |
| --- | --- | --- |
| **Files touched** | How many files outside tests have a change that could alter behaviour | 15 |
| **Modules touched** | How many packages those files belong to | 4 |
| **Lines added** | How many non-blank lines were added to source files outside tests | 1000 |
| **Uncovered files** | How many changed files `lgtm` can't read (another language, SQL, YAML) and you haven't told it to ignore | 1 |
| **Complexity increase** | The most any existing function got harder to read ([cognitive complexity](https://www.sonarsource.com/docs/CognitiveComplexity.pdf)) | 5 |
| **New-function complexity** | How hard to read the most complex new function is | 25 |
| **Significance** | The riskiest kind of change to existing code: none, low (statements, constant values), medium (conditions, error handling), high (an exported signature), crucial (an export removed, a superclass changed) | crucial |
| **Blast radius** | How many symbols call what changed, up to three calls away | 30 |
| **Edit depth** | How deep in the syntax tree the deepest change sits. It depends on each language's grammar, so it's reported but not used by default | off |

While the significance threshold is on, an exported signature change to
something that has callers also requires review.

Two rules ignore the thresholds:

- **A file it can't parse always requires review.** No measurement means no free pass.
- **A change to an always-review path requires review.** By default that's your
  GitHub workflows and `.lgtm.yml`; add your own.

Comments, formatting, imports, renamed locals and code moved within a module
don't count as touching a file. Neither does deleting a private function
nothing uses. New code is judged by new-function complexity and by the file,
module and line counts: a narrow addition of readable code passes.

Defaults and path lists can be changed in a `.lgtm.yml`, including what `lgtm`
skips. See **[docs/configuration.md](docs/configuration.md)** for the options
and ready-made recipes.

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

Full reference (all options, what each check result means, troubleshooting):
**[docs/action.md](docs/action.md)**.

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
curl -fsSL https://github.com/donaldwasserman/lgtm/releases/download/v0.0.4/lgtm_0.0.4_linux_amd64.tar.gz | tar -xz lgtm

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
| `--config` | `.lgtm.yml` in the base tree | Configuration file |
| `--preset` | `balanced` | Gate to start from: `balanced` or `strict` |
| `--theta-breadth` | `15` | Files touched that require review |
| `--theta-modules` | `4` | Modules touched that require review |
| `--theta-lines` | `1000` | Lines added that require review |
| `--theta-uncovered` | `1` | Uncovered files that require review |
| `--theta-cog` | `5` | Increase in one existing function's cognitive complexity that requires review |
| `--theta-new-function` | `25` | Cognitive complexity of one new function that requires review |
| `--theta-significance` | `crucial` | Significance level (`low`, `medium`, `high`, `crucial`) that requires review |
| `--theta-blast` | `30` | Blast radius that requires review |
| `--theta-depth` | `off` | Edit depth that requires review |
| `--exclude` | | Glob of paths to leave out of the scan; repeat for several. `!entry` removes a built-in entry |
| `--output` | | Also write the report to this file |
| `--exit-zero` | `false` | Exit `0` whatever the verdict; only errors exit non-zero |
| `--version` | | Print the version |

### Result

**Exit code:** `0` no review needed, `1` review needed, `2` error (including a
branch that can't be found). With `--exit-zero`, read `verdict` from the
report instead.

`--exclude` patterns work as described in
[docs/configuration.md](docs/configuration.md#path-lists). `node_modules`,
`vendor`, `dist`, `build` and similar are excluded by default, and
`--exclude '!build'` brings one back.

Every `--theta-*` flag takes a number or `off`. `0` is rejected: it would
flag every change.

The report is JSON on standard output:

Here `lib.Parse` gained a parameter, and `app.Run` calls it (abridged):

```console
$ lgtm --base before/ --head after/
{
  "schemaVersion": 3,
  "verdict": "review-required",
  "requiresReview": true,
  "reasons": ["called-exported-signature"],
  "scores": { "editDepth": 3, "newDepth": 4, "breadthFiles": 1, "breadthModules": 1,
              "linesAdded": 1, "uncoveredFiles": 0, "cogDelta": 0, "newFunctionComplexity": 0,
              "significance": "high", "calledSignificance": "high", "blastRadius": 1 },
  "facts": { "unparsed": false, "alwaysReview": false },
  "gate": { "thetaDepth": null, "thetaBreadth": 15, "thetaModules": 4, "thetaLines": 1000,
            "thetaUncovered": 1, "thetaCog": 5, "thetaNewFunction": 25,
            "thetaSignificance": "crucial", "thetaBlast": 30 },
  "overrides": [],
  "contributors": {
    "significance": [ { "symbol": "lib Parse", "file": "lib/lib.go", "line": 3, "level": "high", "parts": ["signature"] } ],
    "blastRadius": [ { "symbol": "lib Parse", "file": "lib/lib.go", "line": 3, "value": 1 } ]
  },
  "files": [ { "path": "lib/lib.go", "kind": "modify", "coverage": "analyzed" } ]
}
```

| Field | Meaning |
| --- | --- |
| `schemaVersion` | `3`. Goes up only if a field is renamed, removed, or changes meaning |
| `verdict` | `"review-required"` or `"no-review"` |
| `requiresReview` | Same, as `true`/`false` |
| `reasons` | Every rule that required review (`unparsed`, `always-review`, `edit-depth`, `breadth-files`, `breadth-modules`, `lines-added`, `uncovered-files`, `cog-delta`, `new-function-complexity`, `significance`, `called-exported-signature`, `blast-radius`) |
| `scores` | One score per measure. `null` means the measure isn't available for the change's languages, which never requires review |
| `facts` | Inputs to the rules that ignore thresholds |
| `gate` | The thresholds used, after any overrides. `null` means switched off |
| `overrides` | The path overrides that applied |
| `contributors` | What's behind each score, highest first: the symbol, file, and line where they apply. `deleted` marks a place that no longer exists |
| `parseErrors` | Files that couldn't be parsed. Only present if there are any; forces review |
| `files` | Files that changed, how (`insert`, `delete`, `modify`), and `coverage` (`analyzed`, `uncovered`, `ignored`) |

### Other CI systems

`lgtm` only looks at code. Approvals and posting results are up to the caller.
For example:

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

Any other changed file counts as uncovered unless it's on the ignore list, which
by default covers documentation, images and licenses.

## More

- [docs/configuration.md](docs/configuration.md) — `.lgtm.yml`, presets and recipes
- [docs/action.md](docs/action.md) — GitHub Action reference
- [docs/development.md](docs/development.md) — building, testing, and the formal model
- [docs/releasing.md](docs/releasing.md) — versions and how to publish one
- [CHANGELOG.md](CHANGELOG.md) — what changed in each version

MIT licensed. See [LICENSE](LICENSE).
