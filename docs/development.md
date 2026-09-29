# Development

## Requirements

- Go 1.25+ and a C compiler (the Tree-sitter code parser is C, so builds need `CGO_ENABLED=1`)
- Java 11+ and `curl` — only for the formal-model checks

## Common commands

```bash
make build        # build bin/lgtm
make test         # Go tests
make test-action  # tests for the logic inside action.yml (needs ruby and jq)
make docker       # build the Docker image as lgtm:dev
make all          # everything below, then build
make clean        # remove build and model output
```

`ci.yml` runs all of these on every pull request. Releases are covered in
[releasing.md](releasing.md).

## The formal model

The review rule and the check results are written as models in
[Alloy](https://alloytools.org), a tool that searches for cases where a rule
breaks. The Go tests are generated from the model, so code and model can't
quietly drift apart.

| File | What it describes |
| --- | --- |
| `alloy/pr_review.als` | "Does this change need a person?" |
| `alloy/properties.als` | Rules that must always hold for it |
| `alloy/scenarios.als` | Example cases; these become Go tests |
| `alloy/check_run.als` | "What does the check on the pull request say?" (the table in [action.md](action.md#what-the-check-means)) |
| `alloy/check_properties.als` | Rules that must always hold for the check |
| `alloy/check_scenarios.als` | One example per row of that table |

```bash
make setup            # download Alloy 6.2.0
make check            # fail if any rule in properties.als can be broken
make scenarios        # fail if any example in scenarios.als is impossible
make check-action     # same as check, for check_properties.als
make scenarios-action # same as scenarios, for check_scenarios.als
make generate         # rebuild eval/evaluator_alloy_test.go from scenarios.als
make verify           # generate + build + test
```

`eval/evaluator_alloy_test.go` is generated. Edit `alloy/scenarios.als`, then
run `make generate`.

Model output goes to `alloy/output/` and `alloy/runtime/` (both ignored by git).

### Rules checked

Review rule (`properties.als`):

- Deep **new** code alone never forces review.
- Very small changes never force review.
- Deep edits to existing code always force review.
- Changes that are both wide and non-trivial always force review.
- Small, narrow, shallow changes never force review.
- Making edits deeper never turns review *off*.
- A trusted author skips review — unless a file couldn't be read.
- A file that couldn't be read always forces review.

Check results (`check_properties.als`):

- A pull request always gets a check.
- A crash is never green, even if approved.
- Green means "under the limits" or "over the limits and approved". Nothing else.
- One "changes requested" beats any number of approvals.
- Approving is enough to turn it green — no new push needed.
- Comment-only reviews change nothing.
- Only each reviewer's latest verdict counts.
- An unreadable file can't go green without approval.
- A trusted author's readable change stays green.

## Code layout

```
cmd/lgtm/          command-line entry point
cmd/genalloy/      turns Alloy examples into eval/evaluator_alloy_test.go
eval/              the review rule (mirrors alloy/pr_review.als)
internal/parse/    reads source files with Tree-sitter; one entry per language
internal/diff/     lines up before/after code and classifies each change
internal/metrics/  turns the comparison into the numbers the rule uses
internal/model/    shared types
alloy/             formal models
scripts/           tests for action.yml
```
