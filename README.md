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

The repo ships a composite action (`action.yml`). It builds the binary, runs it,
and exposes the decision as the `requires-review` output. It can also label the
pull request and request reviewers. A failing check does **not** block a merge
on its own — see [Enforcing the result](#enforcing-the-result).

### Drop-in workflow

Save as `.github/workflows/review-complexity.yml` in the repo you want to
check. This is the recommended setup: the check stays green and reports its
verdict by labelling the PR, so flagged changes are routed to a reviewer rather
than blocked.

```yaml
name: review-complexity

on:
  pull_request:

# Needed to label the PR and request reviewers.
permissions:
  contents: read
  pull-requests: write

jobs:
  lgtm:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Determine merge base
        id: base
        run: echo "ref=$(git merge-base "origin/$GITHUB_BASE_REF" HEAD)" >> "$GITHUB_OUTPUT"

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
          fail-on-review-required: 'false'
          label: 'needs-review'
```

Then create the `needs-review` label in the repo, and pick a merge gate from
[Enforcing the result](#enforcing-the-result).

To make the check itself fail on a flagged PR instead, drop
`fail-on-review-required` (it defaults to `'true'`) along with `label` and the
`permissions` block — but read the polarity note in that section first.

To exempt trusted contributors, add:

```yaml
          top-twenty: ${{ contains(fromJson('["alice","bob"]'), github.actor) }}
```

`lgtm` compares two directories, so the workflow materialises the base tree
itself; the action does no git work of its own.

> **Paths must already be resolved.** `base` and `head` are passed to the tool
> verbatim and are never expanded by a shell, so use `${{ runner.temp }}` and
> `${{ github.workspace }}`. Writing `"$RUNNER_TEMP/lgtm/base"` passes that text
> through literally and the run fails with exit 2.

`.github/workflows/review.yml` runs the same thing against this repo using the
local `uses: ./` form.

### Action inputs

Beyond the four threshold inputs above:

| Input | Default | Description |
| --- | --- | --- |
| `label` | `''` | Label added when review is required, removed when it is not. Empty disables labelling. |
| `request-reviewers` | `''` | Comma-separated reviewers to request. A plain login requests a user; `org/team` requests a team. |
| `fail-on-review-required` | `'true'` | Whether the step fails when review is required. Set to `'false'` to report the result without failing. |
| `github-token` | `${{ github.token }}` | Token used to label and request reviewers. Needs `pull-requests: write`. |

Outputs: `requires-review` (`"true"`/`"false"`) and `report` (the JSON).

Labelling and reviewer requests need the job to grant:

```yaml
permissions:
  contents: read
  pull-requests: write
```

Neither is fatal: if the token cannot label or the reviewer is the PR author or
is already requested, the action logs a warning and carries on.

## Enforcing the result

Exiting non-zero puts a red check on the pull request. **That alone does not
prevent a merge** — a check blocks merging only once it is marked required in
the repository's branch protection rules or rulesets, which is configured in
repository settings, not here.

Before requiring the check, note the polarity. `lgtm` exits 1 to mean *this
change needs a human to look at it*, but a required check must go green to
merge. Requiring it therefore means "only changes that need no review may
merge": a PR that genuinely warrants review becomes unmergeable until it is
split or shrunk, and no amount of human approval turns the check green. That is
a legitimate way to enforce small PRs, but it is not a review workflow.

To route flagged PRs to a reviewer instead, let the check pass and act on the
result:

The [drop-in workflow](#drop-in-workflow) above is already configured this way.
To also request reviewers, extend its `with:` block:

```yaml
        with:
          base: ${{ runner.temp }}/lgtm/base
          head: ${{ github.workspace }}
          fail-on-review-required: 'false'   # signal, not a wall
          label: 'needs-review'
          request-reviewers: 'alice,my-org/platform-team'
```

The merge gate is then one of:

- **Requested reviewers** — enable "Require a pull request before merging" with
  at least one approval. The action requests reviewers only on flagged PRs, so
  unflagged ones still need the repository's baseline approvals.
- **The label** — require a status check that fails while `needs-review` is
  present and no approval exists, or use a ruleset that blocks merging on that
  label.
- **CODEOWNERS** — for path-based ownership, independent of `lgtm`.

Because the label is removed when a PR stops being flagged, a PR that is split
or shrunk clears itself on the next run.

To gate on the decision in a later step of your own workflow, read the output:

```yaml
- uses: donaldwasserman/lgtm@main
  id: lgtm
  with:
    base: ${{ runner.temp }}/lgtm/base
    head: ${{ github.workspace }}
    fail-on-review-required: 'false'
- if: steps.lgtm.outputs.requires-review == 'true'
  env:
    REPORT: ${{ steps.lgtm.outputs.report }}
  run: echo "$REPORT"
```

## Formal verification

The decision rule lives in Alloy and the Go implementation is tested against
solver-produced instances rather than hand-written expectations.

```bash
make setup       # download alloy.jar (Alloy 6.2.0)
make check       # check the seven assertions in alloy/properties.als
make scenarios   # confirm every scenario in alloy/scenarios.als is satisfiable
make generate    # solve scenarios -> XML -> regenerate eval/evaluator_alloy_test.go
make test        # go test ./...
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
