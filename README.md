# lgtm

`lgtm` decides whether a pull request actually needs a human review.

It parses the base and head source trees with Tree-sitter, aligns their ASTs,
and reduces the change to four numbers — how deep it cuts into existing code,
how deep the new code nests, how many files it spreads across, and the overall
depth. Those numbers feed a decision rule that is specified formally in Alloy
(`alloy/pr_review.als`) and verified against machine-generated scenario tests.

A PR requires review when:

```
requiresReview = unparsed || (!topTwenty && (depthMod >= θ_depth ||
                              (breadth >= θ_breadth && depthTotal > ε_trivial)))
```

Trusted (top-20%) contributors are exempt, which overrides both depth and
breadth rules. A tree containing a file that could not be parsed overrides even
that: with no reliable metrics the gate refuses to wave the change through.

## Requirements

- Go 1.25+ (required for building and running `lgtm`)
- Java 11+ and `curl` — only for the Alloy formal-verification targets

## Install

```bash
go build -o bin/lgtm ./cmd/lgtm
# or
make build
```

## Usage

`lgtm` compares two directories, not two git refs. Materialize the base tree
yourself (`git archive`, a second worktree, etc.) and point the tool at both.

```bash
lgtm --base <dir> --head <dir> [flags]
```

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--base` | *(required)* | Path to the base (pre-PR) source tree |
| `--head` | *(required)* | Path to the head (post-PR) source tree |
| `--theta-depth` | `7` | High depth threshold (θ_depth) |
| `--theta-breadth` | `6` | High breadth threshold (θ_breadth) |
| `--epsilon-trivial` | `1` | Depth at or below which a change is trivial (ε_trivial) |
| `--top-twenty` | `false` | Submitter is a trusted top-20% contributor (exempts review) |
| `--help` | | Show usage |

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Review not required |
| `1` | Review required |
| `2` | Usage error or failure while scanning/diffing |

The decision is on the exit code, so `lgtm` gates a CI job directly. The JSON
report always goes to stdout regardless of the decision.

### Example

```console
$ lgtm --base /tmp/base --head /tmp/head
{
  "requiresReview": false,
  "metrics": {
    "DepthMod": 6,
    "DepthNew": 1,
    "Breadth": 1,
    "DepthTotal": 6,
    "TopTwenty": false,
    "Unparsed": false
  },
  "thresholds": {
    "ThetaDepth": 7,
    "ThetaBreadth": 6,
    "EpsilonTrivial": 1
  },
  "files": [
    {
      "path": "a.go",
      "kind": "modify"
    }
  ]
}
$ echo $?
0
```

`files` lists only the paths that actually changed; identical files are omitted
and do not count toward `Breadth`.

A file that fails to parse is reported and forces review:

```console
$ lgtm --base /tmp/base --head /tmp/broken
{
  "requiresReview": true,
  "metrics": { "...": "...", "Unparsed": true },
  "parseErrors": [
    { "path": "x.go", "side": "head", "reason": "syntax error" }
  ],
  "files": [ { "path": "x.go", "kind": "modify" } ]
}
$ echo $?
1
```

Comparing against a merge base:

```bash
BASE=$(git merge-base origin/main HEAD)
mkdir -p /tmp/lgtm-base
git archive "$BASE" | tar -x -C /tmp/lgtm-base
lgtm --base /tmp/lgtm-base --head "$PWD"
```

## Supported languages

Detected by extension; everything else is skipped, as are `.git`,
`node_modules`, `vendor`, `.venv`, `venv`, and `__pycache__` directories.

| Language | Extensions |
| --- | --- |
| Go | `.go` |
| Python | `.py` |
| Ruby | `.rb` |
| JavaScript | `.js` `.mjs` `.cjs` `.jsx` |
| TypeScript | `.ts` `.mts` `.cts` |
| TSX | `.tsx` |
| Java | `.java` |

## GitHub Action

The repo ships a composite action (`action.yml`). It builds the binary, runs
it, and publishes the verdict as a **check run** on the pull request head
commit. The check is red when the change needs review and nobody has approved
it, and it clears itself the moment someone does — no re-run required.

It can also label the pull request and request reviewers, which is routing
rather than enforcement; the check run is the merge gate.

### Drop-in workflow

Save as `.github/workflows/review-complexity.yml` in the repo you want to
check.

```yaml
name: review-complexity

on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
  # Re-evaluating on review is what lets the check clear itself.
  pull_request_review:
    types: [submitted, dismissed]

permissions:
  contents: read
  checks: write          # publish the check run
  pull-requests: write   # label the PR and read its reviews

jobs:
  lgtm:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          # On pull_request_review the default ref is the base branch, so
          # without this you would analyze the wrong tree.
          ref: ${{ github.event.pull_request.head.sha }}

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Determine merge base
        id: base
        env:
          # GITHUB_BASE_REF is not set on pull_request_review.
          BASE_REF: ${{ github.event.pull_request.base.ref }}
        run: |
          git fetch --no-tags origin "$BASE_REF"
          echo "ref=$(git merge-base "origin/$BASE_REF" HEAD)" >> "$GITHUB_OUTPUT"

      - name: Extract base tree
        run: |
          mkdir -p "$RUNNER_TEMP/lgtm/base"
          git archive "${{ steps.base.outputs.ref }}" | tar -x -C "$RUNNER_TEMP/lgtm/base"

      - uses: donaldwasserman/lgtm@main
        with:
          base: ${{ runner.temp }}/lgtm/base
          head: ${{ github.workspace }}
          theta-depth: '7'
          theta-breadth: '6'
          epsilon-trivial: '1'
          check-name: 'LGTM / review-gate'
          label: 'needs-review'
```

Both triggers matter. `pull_request_review` is what re-evaluates on approval;
`ready_for_review` covers a draft going up for review, which does not fire
`synchronize`.

### What the check means

The check run is published against `github.event.pull_request.head.sha` — the
commit branch protection actually evaluates — and reports one of:

| Conclusion | Title | Meaning |
| --- | --- | --- |
| `success` | No review required | Below the thresholds. |
| `success` | Review required — approved | Above the thresholds, but approved. |
| `failure` | Review required — awaiting approval | Above the thresholds, no approval yet. |
| `failure` | Changes requested | Above the thresholds, a reviewer blocked it. |
| `failure` | Analysis failed — could not evaluate | `lgtm` did not run. Fails closed. |

**Green does not mean "this change is simple."** It means *either* the change
is below the thresholds *or* a human has approved it. A 40-file refactor with
one approval is green, and that is the intended behaviour: the check asks
"has this been looked at by someone, if it needed to be?", not "is this
small?". The check's title says which of the two you are looking at.

An approval counts when it is the reviewer's most recent verdict on the PR.
Reviews that only comment are ignored, a dismissed approval stops counting,
and one outstanding "changes requested" keeps the check red regardless of how
many approvals sit alongside it. GitHub already prevents an author from
approving their own pull request, so no extra self-approval handling is
needed.

A failed analysis is deliberately `failure` rather than `neutral`: `neutral`
counts as passing for required checks, so a crashed analyzer would silently
wave pull requests through.

### Action inputs

| Input | Default | Description |
| --- | --- | --- |
| `base` | *(required)* | Resolved path to the base tree |
| `head` | *(required)* | Resolved path to the head tree |
| `theta-depth` | `7` | High depth threshold |
| `theta-breadth` | `6` | High breadth threshold |
| `epsilon-trivial` | `1` | Trivial-depth upper bound |
| `top-twenty` | `false` | Author is a trusted top-20% contributor |
| `check-name` | `LGTM / review-gate` | Name of the published check run. Empty disables it |
| `label` | *(empty)* | Label applied to flagged pull requests |
| `request-reviewers` | *(empty)* | Comma-separated reviewers to request on flagged pull requests |
| `github-token` | `${{ github.token }}` | Needs `checks: write` and `pull-requests: write` |

`base` and `head` are passed to the binary verbatim and are not expanded by a
shell, so use `${{ runner.temp }}/...` rather than `"$RUNNER_TEMP/..."`.

`check-name` is the exact string you type into branch protection. Renaming it
later un-requires the old name silently, and every open pull request goes
green — pick one and leave it alone.

### Outputs

| Output | Values |
| --- | --- |
| `requires-review` | `true`, `false`, or **empty** when `lgtm` could not run |
| `check-conclusion` | `success` or `failure`; empty when no check was published |
| `report` | The full JSON report; empty when `lgtm` could not run |

`requires-review` is empty rather than `false` on a tool failure, because
claiming `false` would wave the change through. That means a naive
`!= 'true'` test reads a crash as "no review needed" — within a pull request,
gate on `check-conclusion` instead if you want fail-closed behaviour:

```yaml
- uses: donaldwasserman/lgtm@main
  id: lgtm
  with:
    base: ${{ runner.temp }}/lgtm/base
    head: ${{ github.workspace }}
- if: steps.lgtm.outputs.check-conclusion == 'failure'
  env:
    REPORT: ${{ steps.lgtm.outputs.report }}
  run: echo "$REPORT"
```

The job itself fails only when `lgtm` breaks. A change that requires review is
reported by the check run, not by a red job, so the two signals stay distinct.

### Making it a required check

Publishing the check does not block anything on its own. To gate merges, add a
branch protection rule or ruleset on your default branch requiring the status
check named `LGTM / review-gate` (or whatever you set `check-name` to).

Do this **after** you have watched the check go green and red correctly on a
real pull request. Marking it required before it has ever run successfully
gates your default branch on a check that may be broken, on the same commit
that broke it.

### Routing: labels and reviewers

`label` and `request-reviewers` are about getting the right eyes on a flagged
pull request, not about blocking it:

```yaml
        with:
          label: 'needs-review'
          request-reviewers: 'alice,my-org/platform-team'
```

A plain login requests a user; `org/team` requests a team. Create the label in
the repo first.

The label tracks whether the change is *complex*, not whether it is currently
*blocked* — an approved pull request keeps its `needs-review` label, so "which
changes were complex?" stays an answerable question after the fact. It is
removed only when a pull request shrinks back below the thresholds.

### Troubleshooting

**The check never appears.** The workflow needs `checks: write`. Without it the
API call 403s and the action logs a warning rather than failing, so check the
job log for `could not publish the ... check run`.

**Nothing appears on pull requests from forks.** Fork pull requests get a
read-only `GITHUB_TOKEN`, so no check can be published. Fork support is out of
scope; if you need it, split the workflow in two and publish the check from a
privileged `workflow_run` workflow that consumes an artifact from the
unprivileged one. Do not reach for `pull_request_target` — it builds
fork-authored code with a write-scoped token.

**A complex pull request is green.** It has been approved. See
[What the check means](#what-the-check-means).

**The check shows in the UI but the merge gate ignores it.** Something is
publishing against the merge commit rather than the head commit. The action
refuses to guess here and errors out when no head SHA is available, so this
should only happen with a hand-rolled variant.

**Approving does not turn it green.** The workflow is missing the
`pull_request_review` trigger, or its `actions/checkout` is missing
`ref: ${{ github.event.pull_request.head.sha }}` and is analyzing the base
branch.
## Formal verification

The decision rule lives in Alloy and the Go implementation is tested against
solver-produced instances rather than hand-written expectations.

```bash
make setup       # download alloy.jar (Alloy 6.2.0)
make check       # check the seven assertions in alloy/properties.als
make scenarios   # confirm every scenario in alloy/scenarios.als is satisfiable
make generate    # solve scenarios -> XML -> regenerate eval/evaluator_alloy_test.go
make test        # go test ./...
make test-action # exercise the action's approval logic against saved payloads
make verify      # generate + build + test
make all         # check + scenarios + generate + build
make clean       # remove bin/ and Alloy output
```

`make check` fails if any assertion yields a counterexample; `make scenarios`
fails if any expected scenario is UNSAT. `make generate` regenerates
`eval/evaluator_alloy_test.go` from the instance XML in `alloy/runtime/`, so
that file is generated — edit `alloy/scenarios.als` instead.

Alloy solution files and logs are written to `alloy/output/` (gitignored);
instance XML for the generator goes to `alloy/runtime/`. Both are removed by
`make clean`.

Properties currently checked (`alloy/properties.als`):

- `NewCodeExemption` — deep *new* code alone never forces review
- `TrivialDepthExemption` — trivial-depth changes never force review
- `HighDepthModRequiresReview`
- `BroadNontrivialRequiresReview`
- `LowEverythingNeverRequires`
- `MonotoneInDepthMod` — raising `depthMod` never flips review off
- `TrustedContributorExemption` — a trusted submitter is exempt, provided the
  tree parsed
- `UnparsedAlwaysRequiresReview` — an unparseable tree always requires review

## Layout

```
cmd/lgtm/        CLI entrypoint
cmd/genalloy/    generates eval/evaluator_alloy_test.go from Alloy instances
eval/            RequiresReview decision logic (mirrors alloy/pr_review.als)
internal/parse/  Tree-sitter scanning and per-language specs
internal/diff/   AST alignment and insert/delete/modify classification
internal/metrics/ diff result -> eval.Metrics
internal/model/  language-agnostic AST and change types
alloy/           formal spec, properties, scenarios
```
