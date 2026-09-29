# lgtm

`lgtm` decides whether a pull request actually needs a human review.

It parses the base and head source trees with Tree-sitter, aligns their ASTs,
and reduces the change to four numbers — how deep it cuts into existing code,
how deep the new code nests, how many files it spreads across, and the overall
depth. Those numbers feed a decision rule that is specified formally in Alloy
(`alloy/pr_review.als`) and verified against machine-generated scenario tests.

A PR requires review when:

```
requiresReview = unparsed || (!trusted && (depthMod >= θ_depth ||
                             (breadth >= θ_breadth && depthTotal > ε_trivial)))
```

Trusted contributors are exempt, which overrides both depth and breadth rules.
`trusted` is an opaque input: *which* contributors are trusted is a policy the
caller configures, deliberately outside the decision rule — see
[Trust modes](#trust-modes). A tree containing a file that could not be parsed
overrides even that: with no reliable metrics the gate refuses to wave the
change through.

## Requirements

- Docker, to run the published image; or Go 1.25+ and a C compiler to build
  natively (Tree-sitter is cgo)
- `git`, for git mode (bundled in the image)
- Java 11+ and `curl` — only for the Alloy formal-verification targets

## Install

The image is published for `linux/amd64` and `linux/arm64`:

```bash
docker pull ghcr.io/donaldwasserman/lgtm:main
```

Or build it, or the binary, from source:

```bash
make docker-build   # image tagged lgtm:local
make build          # native binary at bin/lgtm
```

## Usage

`lgtm` compares two source trees. It takes them either as two directories, or
as a git checkout plus the ref the change targets, in which case it diffs the
head commit against its merge base with that ref:

```bash
lgtm --base <dir> --head <dir> [flags]
lgtm --repo <dir> --base-ref <ref> [--head-ref <ref>] [flags]
```

`lgtm` only evaluates code. It never touches the network, and it knows
nothing about GitHub: who is trusted, whether the change has been approved and
where the verdict is published are the caller's business. The
[GitHub Action](#github-action) is one such caller; any other CI can be
another, using the [report](#report) and [exit codes](#exit-codes).

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--base` | | Path to the base (pre-PR) source tree. Directory mode |
| `--head` | | Path to the head (post-PR) source tree. Directory mode |
| `--repo` | | Path to a git checkout. Git mode, with `--base-ref` |
| `--base-ref` | | Ref the change targets; the base tree is its merge base with `--head-ref` |
| `--head-ref` | `HEAD` | Ref of the change itself |
| `--theta-depth` | `7` | High depth threshold (θ_depth) |
| `--theta-breadth` | `6` | High breadth threshold (θ_breadth) |
| `--epsilon-trivial` | `1` | Depth at or below which a change is trivial (ε_trivial) |
| `--trusted` | `false` | Submitter is a trusted contributor (exempts review). The caller decides who is trusted |
| `--output` | | Also write the JSON report to this file |
| `--exit-zero` | `false` | Exit `0` for both verdicts; failures still exit non-zero |
| `--help` | | Show usage |

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Review not required |
| `1` | Review required |
| `2` | Usage error or failure while scanning/diffing |

The decision is on the exit code, so `lgtm` gates a CI job directly. The JSON
report always goes to stdout regardless of the decision. With `--exit-zero`,
both verdicts exit `0` and only a failure is non-zero — for a pipeline that
should record the verdict rather than stop on it. Read `verdict` from the
report in that case.

Git mode never fetches. A ref that does not resolve, or a shallow clone with no
merge base, exits `2` rather than diffing against a guess: fetch full history
and the base branch first.

### Report

| Field | Description |
| --- | --- |
| `schemaVersion` | `1`. Bumped when a field is renamed, removed or changes meaning; additions do not bump it |
| `verdict` | `"review-required"` or `"no-review"` |
| `requiresReview` | The same verdict as a boolean |
| `metrics` | `DepthMod`, `DepthNew`, `Breadth`, `DepthTotal`, `Trusted`, `Unparsed` |
| `thresholds` | The thresholds the verdict was computed with |
| `parseErrors` | Files that could not be parsed, with `path`, `side` and `reason`. Omitted when empty |
| `files` | Changed paths with their `kind` |

### Example

```console
$ lgtm --base /tmp/base --head /tmp/head
{
  "schemaVersion": 1,
  "verdict": "no-review",
  "requiresReview": false,
  "metrics": {
    "DepthMod": 6,
    "DepthNew": 1,
    "Breadth": 1,
    "DepthTotal": 6,
    "Trusted": false,
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
  "schemaVersion": 1,
  "verdict": "review-required",
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

Comparing the current branch against `main`:

```bash
lgtm --repo . --base-ref origin/main
```

### Docker

The image's entrypoint is `lgtm`, and it works on whatever is mounted at
`/repo`. Mount the checkout read-only; the container needs no network:

```bash
docker run --rm --network none -v "$PWD:/repo:ro" \
  ghcr.io/donaldwasserman/lgtm:main --repo /repo --base-ref origin/main
```

`make docker-run BASE_REF=origin/main` does the same with a locally built
image.

In another CI system, check out with full history, fetch the target branch,
and act on the exit code — or collect the report and decide later:

```bash
git fetch --no-tags origin main
docker run --rm --network none --user "$(id -u):$(id -g)" \
  -v "$PWD:/repo:ro" -v "$PWD/out:/out" \
  ghcr.io/donaldwasserman/lgtm:main \
  --repo /repo --base-ref origin/main --output /out/lgtm.json --exit-zero
jq -r .verdict out/lgtm.json
```

`--user` lets the container write the report into a directory owned by the CI
user. The trusted-contributor exemption is the caller's to decide; pass
`--trusted` when it applies.

## Supported languages

Detected by extension; everything else is skipped, as are `.git`,
`node_modules`, `vendor`, `target`, `.venv`, `venv`, and `__pycache__` directories.

| Language | Extensions |
| --- | --- |
| Go | `.go` |
| Python | `.py` |
| Ruby | `.rb` |
| JavaScript | `.js` `.mjs` `.cjs` `.jsx` |
| TypeScript | `.ts` `.mts` `.cts` |
| TSX | `.tsx` |
| Java | `.java` |
| Rust | `.rs` |

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

      - name: Fetch base branch
        env:
          # GITHUB_BASE_REF is not set on pull_request_review.
          BASE_REF: ${{ github.event.pull_request.base.ref }}
        run: git fetch --no-tags origin "$BASE_REF"

      - uses: donaldwasserman/lgtm@main
        with:
          base-ref: origin/${{ github.event.pull_request.base.ref }}
          theta-depth: '7'
          theta-breadth: '6'
          epsilon-trivial: '1'
          trust-mode: 'codeowners'   # see "Trust modes"; 'none' exempts nobody
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
| `base-ref` | *(empty)* | Ref the PR targets, e.g. `origin/main`. Git mode |
| `head-ref` | `HEAD` | Ref of the change, for git mode |
| `repo` | `github.workspace` | Resolved path to the checkout, for git mode |
| `base` | *(empty)* | Resolved path to the base tree. Directory mode |
| `head` | *(empty)* | Resolved path to the head tree. Directory mode |
| `image` | `ghcr.io/donaldwasserman/lgtm:main` | The `lgtm` image. Used as-is if already present locally |
| `theta-depth` | `7` | High depth threshold |
| `theta-breadth` | `6` | High breadth threshold |
| `epsilon-trivial` | `1` | Trivial-depth upper bound |
| `trust-mode` | `none` | `none`, `top-percent`, `top-count`, or `codeowners` |
| `trust-percent` | `20` | Percentile cutoff for `top-percent` |
| `trust-count` | `10` | Rank cutoff for `top-count` |
| `author` | PR author login | Who the exemption is evaluated against |
| `check-name` | `LGTM / review-gate` | Name of the published check run. Empty disables it |
| `label` | *(empty)* | Label applied to flagged pull requests |
| `request-reviewers` | *(empty)* | Comma-separated reviewers to request on flagged pull requests |
| `github-token` | `${{ github.token }}` | Needs `checks: write` and `pull-requests: write` |

Set either `base-ref` or both `base` and `head`; anything else is reported
as a failed analysis. Paths are passed verbatim and not expanded by a shell,
so use `${{ runner.temp }}/...` rather than `"$RUNNER_TEMP/..."`.

The action splits in two. The evaluation runs in the `lgtm` container, offline
and with the trees mounted read-only. Everything that talks to GitHub —
resolving trust, reading approvals, publishing the check, labelling — runs on
the runner around it, so the action needs a Linux runner with Docker.

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
    base-ref: origin/${{ github.event.pull_request.base.ref }}
- if: steps.lgtm.outputs.check-conclusion == 'failure'
  env:
    REPORT: ${{ steps.lgtm.outputs.report }}
  run: echo "$REPORT"
```

The job itself fails only when `lgtm` breaks. A change that requires review is
reported by the check run, not by a red job, so the two signals stay distinct.

### Trust modes

The trusted-contributor exemption is resolved by the action before `lgtm` runs;
the binary receives only the resulting boolean. Set `trust-mode` to exactly one
of:

| Mode | Trusted when | Needs |
| --- | --- | --- |
| `none` *(default)* | never | — |
| `top-percent` | the author is in the top `trust-percent`% of contributors | contributors API |
| `top-count` | the author is one of the top `trust-count` contributors | contributors API |
| `codeowners` | the author owns **at least one** changed file | a CODEOWNERS file |

`codeowners` is the mode to reach for first: it needs no extra token scope, no
network ranking, and it is reproducible from the repository contents alone.

**Ranking modes** rank by all-time commit count from
`GET /repos/{owner}/{repo}/contributors`, which is keyed by GitHub login and so
compares directly against the pull request author. Accounts of type `Bot` are
excluded, so a busy `dependabot[bot]` neither inflates the denominator nor
occupies a top slot.

Two rules are worth knowing because they are easy to get wrong:

- **The cutoff rounds up.** The top 20% of three contributors is one person,
  not zero. A mode you deliberately enabled should not be silently inert on a
  small repo.
- **Ties at the cutoff are included.** With commit counts `10, 5, 5, 5, 1`, a
  cutoff of two expands to four. Neither the contributors API nor
  `git shortlog` specifies how ties are ordered, and a merge gate must not flip
  on an unspecified sort.

A consequence worth planning around: on a repo where one person writes most
commits, any ranking mode exempts that person from nearly every pull request.
That is why this repository's own workflow uses `none`. Note also that the
contributors API caps at 500 entries and is served from a periodic cache, so
`top-percent` computes against a wrong denominator on very large repos —
prefer `top-count` or `codeowners` there.

**CODEOWNERS mode** reads the first of `.github/CODEOWNERS`, `CODEOWNERS`, or
`docs/CODEOWNERS` from the head tree, and matches it against the pull request's
real file list from the API — not the `files[]` that `lgtm` reports, which
contains only files it could parse, so ownership over `.md` or `.yml` paths
would otherwise be invisible.

Ownership is **last matching rule wins**, as CODEOWNERS specifies. Given

```
*        @alice
*.go     @bob
```

the owner of `x.go` is `@bob` alone. Treating any matching rule as ownership
would trust `@alice` for every Go file.

The supported glob subset:

| Form | Meaning |
| --- | --- |
| `*` | any characters within one path segment; never crosses `/` |
| `?` | a single character within one segment |
| `**` | zero or more whole segments |
| leading `/`, or any embedded `/` | anchored at the repository root |
| trailing `/` | a directory, and everything beneath it |
| no `/` at all | matches a basename at any depth |

So `docs/*` owns `docs/a.md` but not `docs/a/b.md`, while `docs/` owns both.
Write a trailing slash when you mean a directory. Section headers
(`[Section]`) are skipped, and `!`-negated patterns are skipped because
CODEOWNERS does not support negation.

`@org/team` owners are expanded to their members with `gh api`, which needs the
`read:org` scope. The default `GITHUB_TOKEN` does not carry it for org teams —
when expansion fails the action emits a warning and those members are **not**
trusted. Every failure path here resolves the same way: no CODEOWNERS file, an
unreachable API, or an unexpandable team all mean *not trusted*, so the gate
can only ever become stricter, never looser.

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

**Nobody is ever exempt.** `trust-mode` defaults to `none`. If it is set and
still nothing is exempt, check the job log: the action warns when there is no
CODEOWNERS file, when the contributors API is unreachable, and when an
`@org/team` could not be expanded for lack of `read:org`. Each of those
resolves to *not trusted* by design.

**Analysis failed with "cannot resolve" or "no merge base".** The checkout is
shallow or the base branch was never fetched. Use `fetch-depth: 0` and fetch
the base branch before the action, as in the drop-in workflow.

**Analysis failed pulling the image.** The runner could not pull `image`. The
published package must be public for workflows in other repositories to pull
it anonymously.

**Everyone is exempt.** A ranking mode on a repo with few contributors trusts
whoever writes most of the commits. Use `codeowners`, or `none`.
## Formal verification

The decision rule lives in Alloy and the Go implementation is tested against
solver-produced instances rather than hand-written expectations.

```bash
make setup            # download alloy.jar (Alloy 6.2.0)
make check            # check the assertions in alloy/properties.als
make scenarios        # confirm every scenario in alloy/scenarios.als is satisfiable
make check-action     # check the assertions in alloy/check_properties.als
make scenarios-action # confirm every scenario in alloy/check_scenarios.als is satisfiable
make generate         # solve scenarios -> XML -> regenerate eval/evaluator_alloy_test.go
make test             # go test ./...
make test-action      # exercise the action's approval and trust logic against fixtures
make verify           # generate + build + test
make all              # check + scenarios + check-action + scenarios-action + generate + build
make clean            # remove bin/ and Alloy output
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
  tree parsed. Note this binds the exemption *bit*; how a caller decides who is
  trusted (see [Trust modes](#trust-modes)) is deliberately outside the model,
  which is what lets the policy change without reopening the proof
- `UnparsedAlwaysRequiresReview` — an unparseable tree always requires review

### The check-run layer

`alloy/pr_review.als` answers "does this change need a human?".
`alloy/check_run.als` answers the different question the pull request's status
check actually asks: "what does the gate say?" — which also depends on whether
a human has since shown up. It models the analyzer outcome, the per-reviewer
reduction over review states, and the state/conclusion mapping in
`WORKFLOW_LOGIC.md`, and its branch precedence mirrors the "Publish check run"
step of `action.yml`.

Properties checked (`alloy/check_properties.als`):

- `AlwaysPublishesOnPullRequests` — a publishable evaluation always produces a
  conclusion, so a required check is never silently absent
- `AnalysisFailureFailsClosed` — a crashed analyzer is never green
- `ApprovalDoesNotExcuseFailure` — nor is it green once approved
- `GreenMeansSimpleOrApproved` — green means below the thresholds, or above
  them and approved; nothing else
- `ChangesRequestedOutranksApproval` — one block outranks any number of
  approvals
- `ApprovalClearsTheCheck` — approving is sufficient to turn it green, with no
  re-analysis and no change to the diff
- `CommentsDoNotChangeTheVerdict` — comment-only reviews are inert, so a
  comment left after an approval cannot revoke it
- `LatestVerdictWins` — a superseded verdict never counts
- `UnparsedNeverGreenWithoutApproval` — bridges to `pr_review`: an unparseable
  tree cannot go green on its own
- `TrustedContributorStaysGreen` — bridges to `pr_review`: the top-20%
  exemption survives the check layer

`alloy/check_scenarios.als` holds one satisfiable scenario per row of
`WORKFLOW_LOGIC.md` plus the review reductions the table leaves implicit.
They double as non-vacuity witnesses: an assertion above that held only
because its antecedent was unsatisfiable would surface here as an UNSAT
scenario.

## Layout

```
cmd/lgtm/        CLI entrypoint, including git mode
cmd/genalloy/    generates eval/evaluator_alloy_test.go from Alloy instances
eval/            RequiresReview decision logic (mirrors alloy/pr_review.als)
internal/parse/  Tree-sitter scanning and per-language specs
internal/diff/   AST alignment and insert/delete/modify classification
internal/metrics/ diff result -> eval.Metrics
internal/model/  language-agnostic AST and change types
alloy/           formal spec, properties, scenarios
                 pr_review.als  + properties.als       + scenarios.als
                 check_run.als  + check_properties.als + check_scenarios.als
scripts/         action-level tests (approval reduction fixtures)
Dockerfile       builder, test and runtime stages of the lgtm image
.github/workflows/
                 review.yml   runs the gate on this repo's PRs
                 ci.yml       Go tests, action tests, image smoke test
                 publish.yml  multi-arch image to GHCR on push to main
```

Publishing needs one manual step: after the first run of `publish.yml`, set
the `lgtm` package on GHCR to public, or consumers cannot pull it.
