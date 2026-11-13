# GitHub Action reference

For a copy-paste workflow, see the [Quick start](../README.md#quick-start-github-action).

What the action does on each run:

1. Pulls the `lgtm` Docker image that matches this version of the action.
2. Runs `lgtm` in that image on the before/after code — offline, with the code
   mounted read-only. Settings come from `.lgtm.yml` on the base commit, then
   this action's inputs.
3. If review is needed, checks whether someone has approved the latest commit.
4. Posts the result as a check on the pull request, with annotations on the
   lines behind each rule that fired.
5. Optionally adds a label and requests reviewers.

Needs a Linux runner with Docker, `gh`, and `jq`. GitHub-hosted
`ubuntu-latest` has all three.

## Versions

| You write | You get |
| --- | --- |
| `donaldwasserman/lgtm@v0` | The newest `0.x.x`. Fixes arrive automatically. *Recommended.* Before 1.0, a new version can change defaults; see the changelog. |
| `donaldwasserman/lgtm@v0.0.4` | Exactly that version, forever. |
| `donaldwasserman/lgtm@<commit SHA>` | Exactly that commit (must be on `main`). |
| `donaldwasserman/lgtm@main` | Unreleased code. May change or break at any time. |

The action pulls the matching image — `ghcr.io/donaldwasserman/lgtm:0.0.4`,
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
| `preset` | *(empty)* | `balanced` or `strict`. Empty uses `.lgtm.yml`, then `balanced` |
| `theta-breadth` | *(preset: 25)* | Files touched (existing code changed; new files don't count) that require review |
| `theta-modules` | *(preset: 4)* | Modules touched that require review |
| `theta-lines` | *(preset: 7000)* | Lines added that require review |
| `theta-uncovered` | *(preset: 1)* | Uncovered files that require review |
| `theta-cog` | *(preset: 5)* | Increase in one existing function's cognitive complexity that requires review |
| `theta-new-function` | *(preset: 25)* | Cognitive complexity of one new function that requires review |
| `theta-significance` | *(preset: `crucial`)* | Significance level (`low`, `medium`, `high`, `crucial`) that requires review. While on, an exported signature change to something with callers also does |
| `theta-blast` | *(preset: 30)* | Blast radius (callers within three hops of code whose logic changed) that requires review |
| `theta-test-deletions` | *(preset: 100)* | Non-blank lines removed from test files that require review. Moved tests don't count |
| `theta-depth` | *(preset: off)* | Edit depth that requires review |
| `exclude` | | Globs of paths to leave out of the scan, one per line (e.g. `**/*.pb.go`). `!entry` removes a built-in entry, such as `!build` |
| `check-name` | `LGTM / review-gate` | Name of the check. Empty turns the check off |
| `label` | *(empty)* | Label added to flagged pull requests |
| `request-reviewers` | *(empty)* | Comma-separated users or `org/team`s to request on flagged pull requests |
| `github-token` | `${{ github.token }}` | Needs `checks: write` and `pull-requests: write` |

Every `theta-*` input takes a number or `off`, and an empty input leaves the
setting to `.lgtm.yml` or the preset. What each measure means is in the
[README](../README.md#how-it-decides). Prefer `.lgtm.yml`
([docs/configuration.md](configuration.md)): it's read from the base commit,
while inputs live in a workflow the pull request can edit.

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

- Only reviews from people with **write access** count, either way.
- An approval counts only on the **latest commit**. A new push needs a new
  approval. A request for changes stands until it's withdrawn or dismissed.
- Only each reviewer's **latest** approve / request-changes counts.
- Comment-only reviews are ignored.
- A dismissed approval stops counting.
- One open "changes requested" keeps it red, however many approvals there are.
- Authors can't approve their own pull requests (GitHub enforces this).

A crash is a failure, not a neutral result, on purpose: GitHub treats
"neutral" as passing, so a broken tool would let everything through.

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
| An approval didn't count | It was left before the latest push, or the reviewer lacks write access. |
| Every pull request needs review | A file in a language `lgtm` can't read keeps changing (look for `uncovered-files`). Add it to `uncovered-ignore`; see [configuration](configuration.md#repositories-with-many-config-files). |
| A green check after someone edited the workflow | Pull requests can change their own workflow. See [Keeping the gate honest](configuration.md#keeping-the-gate-honest). |
| No check on pull requests from forks | Forks get a read-only token. Not supported. Don't switch to `pull_request_target` — it runs untrusted code with write access. |
| A big pull request is green | Someone approved it. The check title says so. |
| Approving doesn't turn it green | Missing the `pull_request_review` trigger, or `actions/checkout` is missing `ref: ${{ github.event.pull_request.head.sha }}`. |
| "Analysis failed" with "cannot resolve" or "no merge base" | The checkout is shallow or the target branch wasn't fetched. Use `fetch-depth: 0` and fetch the branch, as in the Quick start. |
| "Analysis failed" pulling the image | The runner couldn't download the image. Check the job log for the registry error. |
