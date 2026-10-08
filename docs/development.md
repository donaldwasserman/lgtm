# Development

## Requirements

- Go 1.25+ and a C compiler (the Tree-sitter code parser is C, so builds need `CGO_ENABLED=1`),
  and `git` for the git-mode tests — or just Docker, using the `docker-*` targets
- Java 11+ and `curl` — only for the formal-model checks

## Common commands

```bash
make build        # build bin/lgtm
make test         # Go tests
make test-action  # tests for the logic inside action.yml (needs ruby and jq)
make docker-build # build the image as lgtm:local
make docker-test  # run the Go tests inside the image's build stage, as CI does
make docker-run BASE_REF=origin/main  # run the local image on this checkout
make all          # everything below, then build
make clean        # remove build and model output
```

## Workflows

| Workflow | Runs when | Does |
| --- | --- | --- |
| `ci.yml` | every push to `main` and pull request | Go tests (in the image), action tests, model checks, image smoke test |
| `review.yml` | pull requests | runs the action, from this branch, on this repo's own pull requests |
| `publish.yml` | push to `main`, or a version tag | pushes the image to `ghcr.io` |
| `release.yml` | a version tag | builds binaries into a draft release |
| `promote.yml` | a release is published | moves the `1`, `1.2`, `latest` image tags and the `v1` git tag |

Releases are covered in [releasing.md](releasing.md).

## The formal model

The gate, the measures behind it and the check results are written as models
in [Alloy](https://alloytools.org), a tool that searches for cases where a
rule breaks. The gate's Go tests are generated from the model, so code and
model can't quietly drift apart. Vocabulary is in
[GLOSSARY.md](../GLOSSARY.md).

| File | What it describes |
| --- | --- |
| `alloy/gate.als` | The gate: scores, thresholds that can be switched off, fixed rules |
| `alloy/gate_properties.als` | Rules that must hold for every gate configuration |
| `alloy/gate_scenarios.als` | Example changes and configurations; these become Go tests |
| `alloy/blast_radius.als` | Affected set, blast radius and the deletion exemption |
| `alloy/significance.als` | Significance level of a change to one existing symbol |
| `alloy/check_run.als` | "What does the check on the pull request say?" (the table in [action.md](action.md#what-the-check-means)) |
| `alloy/check_properties.als` | Rules that must always hold for the check |
| `alloy/check_scenarios.als` | One example per row of that table |

```bash
make setup            # download Alloy 6.2.0
make check            # fail if any rule in gate_properties.als can be broken
make scenarios        # fail if any example in gate_scenarios.als is impossible
make check-measures   # same, for blast_radius.als and significance.als
make check-action     # same as check, for check_properties.als
make scenarios-action # same as scenarios, for check_scenarios.als
make generate         # rebuild eval/evaluator_alloy_test.go from gate_scenarios.als
make verify           # generate + build + test
```

`eval/evaluator_alloy_test.go` is generated. Edit `alloy/gate_scenarios.als`,
then run `make generate`. In a scenario, a threshold with no value is
switched off and a score with no value is unavailable. Pin every value with
`one`, because Alloy reads `int[]` of an empty set as 0.

Model output goes to `alloy/output/` and `alloy/runtime/` (both ignored by git).

### Rules checked

Gate (`gate_properties.als`), for every configuration:

- An unparsed file always requires review (fixed rule).
- Code lgtm fails to analyze always requires review (fixed rule).
- A trusted author skips every threshold, but never a fixed rule.
- Raising any score never turns review off. An unavailable score counts as
  the lowest value.
- Switching a threshold off never turns review on.
- With every threshold off, only the fixed rules remain.
- With the new thresholds off, the gate is exactly the original depth and
  breadth rule.
- With every new score unavailable, the gate falls back to that same rule.
- Narrow pure additions with readable new functions pass.

Measures (`blast_radius.als`, `significance.als`):

- The 3-hop affected set never claims more than full reachability, and never
  shrinks when more is changed or more calls are found.
- New symbols and uncalled symbols have no blast radius.
- The blast radius *can* shrink when a caller is changed too (a satisfiable
  run documents this).
- The deletion exemption is sound even when the call graph misses calls,
  because of the plain-text name check.
- Body-only changes are at most medium; an exported signature change is at
  least high; removing an export is crucial; an unclassified change is at
  least medium; renaming locals is none; the level never falls as more parts
  change.

Check results (`check_properties.als`):

- A pull request always gets a check.
- A crash is never green, even if approved.
- Green means "under the limits" or "over the limits and approved". Nothing else.
- One "changes requested" beats any number of approvals.
- Approving is enough to turn it green — no new push needed.
- Comment-only reviews change nothing.
- Only each reviewer's latest verdict counts.
- An unparsed file, or code lgtm failed to analyze, can't go green without
  approval.
- A trusted author's measured change stays green.

## Code layout

```
cmd/lgtm/          command-line entry point, including git mode
cmd/genalloy/      turns Alloy examples into eval/evaluator_alloy_test.go
eval/              the gate (mirrors alloy/gate.als)
internal/parse/    reads source files with Tree-sitter; one entry per language
internal/diff/     lines up before/after code and classifies each change
internal/metrics/  turns the comparison into the scores the gate reads
internal/model/    shared types
alloy/             formal models
scripts/           tests for action.yml, and release helpers
Dockerfile         build, test and runtime stages of the image
```
