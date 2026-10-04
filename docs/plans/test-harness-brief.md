# Brief: build a test harness for lgtm

You are building a **separate repository** that tests
[`lgtm`](https://github.com/donaldwasserman/lgtm) from the outside: it runs
the `lgtm` binary (or its Docker image) on pairs of source trees and checks
the JSON report. Don't import lgtm's Go packages, and don't copy its
internals. The harness should keep passing while lgtm's implementation
changes underneath it.

lgtm is being rebuilt to report several new measures (below), which don't
exist yet. The harness has to work **today** against the current release,
and pick up each new measure as it lands without restructuring.

## Vocabulary

Use these words in file names, test names and output.

| Term | Meaning |
| --- | --- |
| **Symbol** | A named declaration (function, method, type, class, module), identified by language module + enclosing declaration + name. The *file* is not part of its identity: moving a function between two files of the same Go package does not change the symbol. |
| **Existing symbol** | A symbol present on the base side. |
| **Measure** | One named aspect lgtm quantifies (e.g. blast radius). |
| **Score** | The value one measure takes for one pull request, in that measure's own unit. There is never a combined score. |
| **Unavailable score** | A measure not implemented for a file's language. It does *not* force review. |
| **Unparsed file** | A file that couldn't be fully parsed (syntax errors included). It *always* forces review. |
| **Gate** | The configured rule that turns scores into "needs review". It's one threshold per measure, and each threshold can be switched off. |
| **Fixed rule** | A part of the gate that no configuration can switch off. |

## How to run lgtm

```bash
lgtm --base <dir> --head <dir> [flags]                     # two folders
lgtm --repo <dir> --base-ref <ref> [--head-ref <ref>]      # git checkout; base = merge-base
```

Today's flags:

| Flag | Default |
| --- | --- |
| `--theta-depth` | 7 |
| `--theta-breadth` | 6 |
| `--epsilon-trivial` | 1 |
| `--trusted` | off |
| `--output <file>` | none |
| `--exit-zero` | off |
| `--version` | — |

Exit codes: `0` = no review needed, `1` = review needed, `2` = lgtm failed.
lgtm prints the JSON report to stdout.

Current report (schema v1, abridged):

```json
{
  "schemaVersion": 1,
  "verdict": "...",
  "requiresReview": true,
  "metrics": { "DepthMod": 7, "DepthNew": 2, "Breadth": 3, "DepthTotal": 7,
               "Trusted": false, "Unparsed": false },
  "thresholds": { "ThetaDepth": 7, "ThetaBreadth": 6, "EpsilonTrivial": 1 },
  "parseErrors": [ { "path": "...", "side": "base|head", "reason": "..." } ],
  "files": [ { "path": "...", "kind": "insert|delete|modify" } ]
}
```

The **next schema** renames `metrics` to `scores` and bumps
`schemaVersion`. The field names below are a *proposal*; lgtm's
implementation will be the source of truth. **Put all knowledge of the
report's shape in one adapter module** that maps any schema version to the
harness's own canonical record, so a rename is a one-file fix.

### Canonical score record (harness side)

| Canonical name | Unit | Comes from today |
| --- | --- | --- |
| `edit_depth` | int | `metrics.DepthMod` |
| `new_depth` | int | `metrics.DepthNew` |
| `total_depth` | int | `metrics.DepthTotal` |
| `breadth_files` | int, files | `metrics.Breadth` |
| `breadth_modules` | int, modules | — (new) |
| `cog_delta` | int, Sonar points: largest increase in any existing function | — (new) |
| `new_function_complexity` | int, Sonar points: largest complexity of any new function | — (new) |
| `significance` | level: `none` < `low` < `medium` < `high` < `crucial` | — (new) |
| `blast_radius` | int, symbols | — (new) |
| `requires_review` | bool | `requiresReview` |
| `unparsed` | bool | `metrics.Unparsed` |

Each new score can be **absent** (lgtm doesn't report it yet: the test is
*pending*, not failing) or **unavailable** (lgtm reports that it can't
produce it for this language). Keep those two cases separate.

New scores will also carry a "top contributors" list (symbol + value), so
the harness can assert *which* function or symbol drove a score.

## Repository layout (suggested)

```
scenarios/<measure>/<language>/<scenario-name>/
  base/          source tree before the change (may be empty)
  head/          source tree after the change
  expect.yaml    expected scores and verdict
corpus/          pinned list of real repos + PR ranges for replay
runner/          runs lgtm, applies the adapter, checks expectations
golden/          recorded reports for snapshot comparison
```

`expect.yaml` should support:

- an exact value;
- `at_least` / `at_most`;
- `equals_base_of` (for paired metamorphic tests);
- `unavailable`;
- `pending: <reason>`, which marks a test as expected-to-be-absent until a
  given measure ships;
- the expected exit code;
- expected top-contributor symbol names.

Make the lgtm binary or image a parameter (`LGTM_BIN` or `LGTM_IMAGE`) so
the same suite can compare two lgtm versions.

## Languages

Every scenario family needs a version in **all seven**:

- Go
- Python
- Ruby
- JavaScript (`.js` / `.mjs` / `.cjs` / `.jsx`)
- TypeScript (`.ts`) and TSX (`.tsx`), which are separate grammars
- Java
- Rust

## Do this first: freeze today's behaviour

Edit depth and breadth must keep producing **exactly** today's values.
Before lgtm changes, record golden reports from the **current release** for:

- every scenario below, and
- a replay of real PRs (see "Corpus replay").

Later runs must match `edit_depth`, `new_depth`, `total_depth` and
`breadth_files` exactly. Also pin the version used to record them.

## Scenarios by measure

Exact values are given where the definition settles them. Elsewhere, assert
direction (`at_least`, equal, zero).

### Cognitive complexity (`cog_delta`, `new_function_complexity`)

The definition is G. Ann Campbell's *Cognitive Complexity* (SonarSource).

Scoring rules:

- **+1 plus the current nesting level** for each `if`, ternary,
  `switch`/`match`, loop, and `catch`/`rescue`/`except`. Each of these also
  raises nesting by one.
- **+1, with no nesting penalty**, for `else if`/`elif`/`elsif`, `else`,
  `goto`, labeled `break`/`continue`, and a recursive call.
- **+1 per run of the same boolean operator.** `a && b && c || d` scores 2.
- Nested functions and lambdas raise nesting but add nothing themselves.
- A whole `switch`/`match` counts once; its cases add nothing.

`cog_delta` is head minus base for functions that exist on both sides; the
PR's score is the largest of these. Brand-new functions count only toward
`new_function_complexity`, never toward `cog_delta`.

Scenarios:

1. **Paper examples.** Port Sonar's `sumOfPrimes` (expect **7**) and
   `getWords` (expect **1**) to each language. Add each as a new function
   (check `new_function_complexity`), and separately as an edit that grows
   an existing trivial function into it (check `cog_delta`).
2. **One rule each:** one scenario per rule, with exact expected values:
   - a nested `if` inside a loop: +1 for the loop, then +2 for the `if`;
   - a flat else-if chain: no nesting penalty;
   - mixed boolean runs;
   - direct recursion;
   - a lambda inside an `if`.
3. **Language quirks:**
   - Go: `else if` is nested in the tree but must not count as nesting;
     `select`.
   - Rust: `match`, plus `if let` / `while let`; the `?` operator counts 0.
   - Python: `elif`, comprehensions with `if`.
   - Ruby: `elsif`, `unless`, `rescue`, statement modifiers (`x if y`).
   - Java/TS/JS: `try`/`catch`, ternaries, optional chaining (counts 0).
4. **Zero-delta changes:**
   - a comment-only edit;
   - a whitespace-only edit;
   - renaming a local variable;
   - reordering two independent functions;
   - moving a function to another file in the same module.

   All must give `cog_delta = 0`.
5. **Guard-clause refactor:** flattening nested `if`s into early returns
   gives `cog_delta = 0`, because the delta never goes negative in the PR
   score. Record the per-function value in the report as negative.
6. **Max, not sum:** PR A adds +4 to ten functions, PR B adds +40 to one.
   Expect `cog_delta` 4 for A and 40 for B.
7. **New-function default:** one new function scoring 25 or more flags
   review under default settings; one scoring 24 doesn't (all else
   trivial).

### Syntactic significance (`significance`)

The PR's level is the highest level among changes to existing symbols. For
a single changed symbol, the level is the max over which parts changed:

| What changed | Level |
| --- | --- |
| alpha-rename of locals only | none |
| statements in a body | low |
| a condition (if/loop/switch predicate) | medium |
| error flow (catch/rescue/except, throw/raise, Rust `?`, Go `panic`/`recover`) | medium |
| signature of a **non-exported** symbol | medium |
| signature of an **exported** symbol | high |
| visibility widened, or narrowed but still exported | medium |
| export **removed** (made private, deleted, or renamed while exported) | crucial |
| supertype (extends/implements, Go embedding, Rust trait bounds) | crucial |
| a change lgtm can't classify | at least medium |

Scenarios: one per row, in every language. Then the visibility forms each
language must get right:

| Language | Visibility forms |
| --- | --- |
| Go | capitalized vs lowercase name |
| Java | `public` / `protected` / package-private / `private` |
| TypeScript | `export`, `export default`, class `private` / `#private` |
| JavaScript | ESM `export`; CommonJS `module.exports = {...}` and `exports.x =` |
| Python | leading `_`; `__all__` overrides it when present |
| Rust | `pub`, `pub(crate)`, no modifier |
| Ruby | `private` / `protected` sections, `private :sym`, `private def`. A class reopened in another file is the same module, so a method moved between the two files is not a removal. |

Moves:

- moving an exported function between files of the same module → `none`;
- moving it to a different module → `crucial` (an export removal plus a new
  export).

### Blast radius (`blast_radius`)

Blast radius is computed on the **base** call graph. It is the set of
symbols that call a changed existing symbol, directly or through up to
**3** intermediate callers, minus the changed symbols themselves. The PR's
score is the size of the union across all changed symbols. Calls are
resolved by name in three tiers: same file, then same module, then a global
name match. A name that matches too many definitions is ignored.

1. **Chain:** `e → d → c → b → a` (each calls the next). Changing `a` gives
   3 (b, c, d); `e` is 4 hops away and doesn't count.
2. **Fan-in:** 60 callers of one function, spread across several files and
   modules. Expect 60.
3. **Uncalled:** a changed function nobody calls gives 0.
4. **New code:** a new function called by existing code gives 0, because
   the base graph doesn't contain it.
5. **Cycles:** `a` and `b` call each other. lgtm must **not crash**; the
   current release has a stack-overflow bug on exactly this. Use this
   scenario as a regression test in every language.
6. **Shrinking radius:** changing `a` gives N. Changing both `a` and its
   only caller `b` can give a *smaller* number. That's correct, so assert
   it as documented behaviour rather than a monotonicity violation.
7. **More callers never lower it:** the same change, with a base tree that
   adds one more caller of the changed symbol, gives a radius at least as
   large.
8. **Common names:** a change to a method named `String`/`Close`/`get`/
   `toString` defined in many places must not attribute every call site in
   the repo.
9. **Cross-module resolution, per language:**
   - Go imports;
   - Python `from x import y`;
   - JS/TS ESM and CommonJS;
   - Java imports;
   - Rust `use`;
   - Ruby `require` with open classes.

### Breadth (`breadth_files`, `breadth_modules`)

- Six files in one module: files = 6, modules = 1.
- Six files across six modules: files = 6, modules = 6.
- The default gate uses files; the module threshold is off by default.

### Deleting a function nobody uses

Deleting an existing symbol is ignored by the new measures, like new code,
only when:

- it isn't exported, and
- nothing calls it, and
- its name appears **nowhere else in the repo as plain text**.

Scenarios:

- Delete a private, uncalled function with no other mention of its name →
  the new scores ignore it (`significance` none, `blast_radius` 0).
  `edit_depth` still counts it, unchanged from today, so check it against
  the golden snapshot.
- The same, but its name appears as a string elsewhere (e.g. a route table
  `"handleWebhook"`) → not exempt.
- Delete an exported uncalled function → `crucial` (export removed).

## Fixed rules (must hold for every gate configuration)

Run each across a grid of threshold settings, including every threshold
switched off.

1. An unparsed file (on either side) → review required.
2. `--trusted` with an unparsed file → review still required.
3. Raising any single score, all else equal, never flips review from
   required to not required.
4. Code in a supported language that lgtm fails to analyze → review
   required. A language lgtm doesn't support → `unavailable`, never forced
   review.

## Generic properties (every scenario)

- **No-op:** identical base and head → every score 0 or `none`, exit 0.
- **Determinism:** two runs give byte-identical JSON, including when files
  are created in a different order on disk.
- **Format-only:** reformatting the code (gofmt/black/prettier/rustfmt
  style) changes no score except possibly edit depth. Record which ones
  move, to learn what they react to.
- **Exit code** agrees with `requiresReview`. `--exit-zero` always gives 0
  unless lgtm failed.

## Corpus replay (data collection, not pass/fail)

Pin a list of public repos, at least two per language, plus
`donaldwasserman/lgtm` itself. For each, take the last ~200 merged PRs.
Run lgtm in git mode (`--repo`, `--base-ref <PR base>`, `--head-ref <PR
merge commit>`) and record every score. Output:

- a per-measure distribution (min / percentiles / max) per language;
- the share of PRs each threshold would flag on its own, and in total;
- every lgtm crash or exit code 2, with the PR it happened on.

Defaults currently ship as informed guesses. This data is what will tune
them later. The crash list is pass/fail.

## Definition of done (first pass)

- Runner, adapter and `expect.yaml` format working against the current
  release.
- Golden snapshots of today's edit depth and breadth recorded.
- Every scenario above exists in all seven languages. Those for unshipped
  measures are marked `pending`.
- Corpus replay runs and writes a report.
- One command runs everything; a CI workflow runs it on a schedule and
  against a given lgtm ref.
