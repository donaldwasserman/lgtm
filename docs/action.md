# GitHub Action reference

For a copy-paste workflow, see the [Quick start](../README.md#quick-start-github-action).

What the action does on each run:

1. Pulls the `lgtm` Docker image that matches this version of the action.
2. Works out whether the author is trusted (only if `trust-mode` is set).
3. Runs `lgtm` in that image on the before/after code — offline, with the code
   mounted read-only.
4. If review is needed, checks whether someone has approved.
5. Posts the result as a check on the pull request.
6. Optionally adds a label and requests reviewers.

Needs a Linux runner with Docker, `gh`, and `jq`. GitHub-hosted
`ubuntu-latest` has all three.

## Versions

| You write | You get |
| --- | --- |
| `donaldwasserman/lgtm@v1` | The newest `1.x.x`. Fixes arrive automatically; nothing breaks. *Recommended.* |
| `donaldwasserman/lgtm@v1.2.3` | Exactly that version, forever. |
| `donaldwasserman/lgtm@<commit SHA>` | Exactly that commit (must be on `main`). |
| `donaldwasserman/lgtm@main` | Unreleased code. May change or break at any time. |

The action pulls the matching image — `ghcr.io/donaldwasserman/lgtm:1.2.3`,
`:sha-<commit>`, or `:main` — so the analysis code always matches the action
you picked. Set `image` to override.

See [CHANGELOG.md](../CHANGELOG.md) for what changed.

## Triggers

Both are needed:

- `pull_request` with `ready_for_review` — runs when code changes, including
  when a draft is marked ready.
- `pull_request_review` — runs when someone approves, so the check can turn
  green without a new push.

On `pull_request_review`, GitHub checks out the target branch by default. Set
`ref: ${{ github.event.pull_request.head.sha }}` on `actions/checkout` or the
action will analyze the wrong code. Also set `fetch-depth: 0` and fetch the
target branch, as in the Quick start — `lgtm` needs the history to find where
the branch split off.

## Inputs

| Input | Default | Meaning |
| --- | --- | --- |
| `base-ref` | *(empty)* | Branch the pull request targets, e.g. `origin/main` |
| `head-ref` | `HEAD` | The change itself |
| `repo` | the workspace | Path to the git checkout |
| `base` | *(empty)* | Instead of `base-ref`: folder with the code before the change |
| `head` | *(empty)* | Instead of `base-ref`: folder with the code after the change |
| `image` | matches the action version | `lgtm` image to run. Used as-is if already on the runner, so a workflow can build its own |
| `theta-depth` | `7` | Edit depth that forces review |
| `theta-breadth` | `6` | Number of files that forces review |
| `epsilon-trivial` | `1` | Total depth at or below which a change is too small to matter |
| `trust-mode` | `none` | Who skips review: `none`, `codeowners`, `top-count`, `top-percent`. See [Trust modes](#trust-modes) |
| `trust-count` | `10` | For `top-count`: how many top contributors are trusted |
| `trust-percent` | `20` | For `top-percent`: what share of contributors are trusted |
| `author` | pull request author | Whose trust is checked |
| `check-name` | `LGTM / review-gate` | Name of the check. Empty turns the check off |
| `label` | *(empty)* | Label added to flagged pull requests |
| `request-reviewers` | *(empty)* | Comma-separated users or `org/team`s to request on flagged pull requests |
| `github-token` | `${{ github.token }}` | Needs `checks: write` and `pull-requests: write` |

Set either `base-ref`, or both `base` and `head`. Anything else is reported as
a failed analysis. Write paths as `${{ runner.temp }}/...`, not
`$RUNNER_TEMP/...` — the value is used as-is, without shell expansion.

**Pick `check-name` once and leave it.** It is the exact name you mark as
required in branch protection. Renaming it quietly stops the old requirement,
and every open pull request goes green.

## Outputs

| Output | Values |
| --- | --- |
| `requires-review` | `true`, `false`, or empty if `lgtm` failed |
| `check-conclusion` | `success`, `failure`, or empty if no check was posted |
| `report` | The full JSON report, or empty if `lgtm` failed |

An empty `requires-review` means "unknown", not "no". If you branch on it,
test for `== 'true'`, or use `check-conclusion == 'failure'` so that a failure
is treated as "needs review":

```yaml
- uses: donaldwasserman/lgtm@v0
  id: lgtm
  with:
    base-ref: origin/${{ github.event.pull_request.base.ref }}
- if: steps.lgtm.outputs.check-conclusion == 'failure'
  env:
    REPORT: ${{ steps.lgtm.outputs.report }}
  run: echo "$REPORT"
```

The job itself only fails when `lgtm` breaks. "Needs review" shows up on the
check, not as a failed job.

## What the check means

The check is posted on the pull request's latest commit.

| Situation | Result | Title shown |
| --- | --- | --- |
| Change is below the limits | ✅ pass | No review required |
| Change is over the limits, approved | ✅ pass | Review required — approved |
| Change is over the limits, not approved yet | ❌ fail | Review required — awaiting approval |
| Change is over the limits, a reviewer requested changes | ❌ fail | Changes requested |
| `lgtm` crashed or couldn't run | ❌ fail | Analysis failed — could not evaluate |
| Not a pull request (e.g. a push to `main`) | nothing posted | — |

**Green does not mean "simple".** It means "simple, *or* a person looked at
it". The title tells you which.

How approvals are counted:

- Only each reviewer's **latest** approve / request-changes counts.
- Comment-only reviews are ignored.
- A dismissed approval stops counting.
- One open "changes requested" keeps it red, however many approvals there are.
- Authors can't approve their own pull requests (GitHub enforces this).

A crash is a failure, not a neutral result, on purpose: GitHub treats
"neutral" as passing, so a broken tool would let everything through.

## Trust modes

A trusted author's change skips review — unless a file couldn't be read. Set
`trust-mode` to one of:

| Mode | Trusted when |
| --- | --- |
| `none` *(default)* | Never |
| `codeowners` | The author owns at least one changed file in CODEOWNERS |
| `top-count` | The author is one of the top `trust-count` contributors by commits |
| `top-percent` | The author is in the top `trust-percent`% of contributors by commits |

Anything that goes wrong while checking trust — no CODEOWNERS file, GitHub API
unreachable, a team that can't be looked up — means **not trusted**, with a
warning in the log. Errors can only make the gate stricter.

**Start with `codeowners`.** It needs no extra permissions and depends only on
what's in the repository.

### `codeowners`

- Reads the first of `.github/CODEOWNERS`, `CODEOWNERS`, `docs/CODEOWNERS`.
- Checks every file in the pull request, including ones `lgtm` doesn't analyze
  (like `.md` or `.yml`).
- **The last matching line wins**, as in GitHub. With `* @alice` then
  `*.go @bob`, only `@bob` owns `x.go`.
- `@org/team` owners need a token with `read:org`. The default token usually
  doesn't have it, so team members won't be trusted unless you pass a token
  that does.

Supported patterns:

| Pattern | Matches |
| --- | --- |
| `*` | anything within one folder level |
| `?` | one character |
| `**` | any number of folder levels |
| starts with `/`, or has a `/` in the middle | from the repository root |
| ends with `/` | that folder and everything in it |
| no `/` at all | a file name at any level |

So `docs/*` matches `docs/a.md` but not `docs/a/b.md`; `docs/` matches both.
`[Section]` headers and `!` lines are ignored.

### `top-count` and `top-percent`

Rank people by total commits (bots excluded) using GitHub's contributors list.

- **Rounds up.** Top 20% of 3 people is 1 person, not 0.
- **Ties are included.** With commit counts `10, 5, 5, 5, 1` and a cut of 2,
  four people are trusted.
- On a repo where one person writes most commits, that person skips review on
  almost everything. That's why this repository uses `none`.
- GitHub's list stops at 500 people and can lag, so on large repos prefer
  `top-count` or `codeowners`.

## Making it a required check

Posting the check doesn't block anything by itself. In your repository's
branch protection (or rulesets), require the status check `LGTM / review-gate`
(or your `check-name`).

Do this **after** you've seen the check pass and fail correctly on a real pull
request. Otherwise a broken setup blocks every merge.

## Labels and reviewers

These help route work; they don't block anything.

```yaml
with:
  label: 'needs-review'
  request-reviewers: 'alice,my-org/platform-team'
```

Create the label first. It marks the change as *big*, not as *blocked*: an
approved pull request keeps it, so you can still find big changes later. It's
removed only if the pull request shrinks back under the limits.

## Troubleshooting

| Problem | Cause |
| --- | --- |
| No check appears | Workflow is missing `checks: write`. Look for `could not publish` in the log. |
| No check on pull requests from forks | Forks get a read-only token. Not supported. Don't switch to `pull_request_target` — it runs untrusted code with write access. |
| A big pull request is green | Someone approved it. The check title says so. |
| Approving doesn't turn it green | Missing the `pull_request_review` trigger, or `actions/checkout` is missing `ref: ${{ github.event.pull_request.head.sha }}`. |
| "Analysis failed" with "cannot resolve" or "no merge base" | The checkout is shallow or the target branch wasn't fetched. Use `fetch-depth: 0` and fetch the branch, as in the Quick start. |
| "Analysis failed" pulling the image | The runner couldn't download the image. Check the job log for the registry error. |
| Nobody is ever trusted | `trust-mode` defaults to `none`. If set, check the log for warnings (no CODEOWNERS, API errors, missing `read:org`). |
| Everyone is trusted | A ranking mode on a repo with few contributors. Use `codeowners` or `none`. |
