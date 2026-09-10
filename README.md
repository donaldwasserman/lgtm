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
| TypeScript | `.ts` `.tsx` `.mts` `.cts` |
| Java | `.java` |

## GitHub Action

The repo ships a composite action (`action.yml`). It builds the binary and runs
it, so a non-zero exit fails the step and blocks the PR.

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: actions/setup-go@v5
  with:
    go-version: '1.25'
- name: Determine merge base
  id: base
  run: echo "ref=$(git merge-base origin/${{ github.base_ref }} HEAD)" >> "$GITHUB_OUTPUT"
- name: Extract base tree
  run: |
    mkdir -p "$RUNNER_TEMP/lgtm/base"
    git archive "${{ steps.base.outputs.ref }}" | tar -x -C "$RUNNER_TEMP/lgtm/base"
- uses: donaldwasserman/lgtm@main
  with:
    base: "$RUNNER_TEMP/lgtm/base"
    head: "$GITHUB_WORKSPACE"
    theta-depth: '7'
    theta-breadth: '6'
    epsilon-trivial: '1'
    top-twenty: ${{ contains(fromJson('["trusted-user-1"]'), github.actor) }}
```

`.github/workflows/review.yml` runs this against the repo itself using the
local `uses: ./` form.

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
