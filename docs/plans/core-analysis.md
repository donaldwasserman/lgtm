# Plan: scores for blast radius, cognitive complexity and significance

Status: agreed design, 2026-10-04. Nothing is implemented yet; the Alloy
models are drafts in `alloy/drafts/`.

Vocabulary is defined in [GLOSSARY.md](../../GLOSSARY.md). The two
decisions that shape everything else are recorded as ADRs:

- [ADR 0001](../adr/0001-module-scoped-symbols.md): symbols are
  identified by module, not file.
- [ADR 0002](../adr/0002-cli-scores-action-gates.md): the CLI reports
  scores; the gate is configured in the action.

## What we're building

`lgtm` stops deciding on its own. It reports **one score per measure**, each
in that measure's own unit. The GitHub Action holds the **gate**: one
threshold per measure, any of which can be switched off, plus four **fixed
rules** that no configuration can disable. The default gate still runs in
Go, and its tests are generated from the Alloy model. The JSON scores are a
documented, versioned output, so a team can also write its own gate in a
workflow step. Custom rule expressions come later.

### Measures

| Measure | Unit | PR score | Default threshold | Status |
| --- | --- | --- | --- | --- |
| Edit depth | nesting + call depth | max | 7 | frozen: today's `DepthMod`, exactly |
| Breadth (files) | files | count | 6, *and* total depth > 1 | frozen |
| Breadth (modules) | modules | count | off | new |
| Cognitive complexity delta | Sonar points | max increase over existing functions | 5 | new |
| New-function complexity | Sonar points | max over new functions | 25 | new |
| Significance level | none < low < medium < high < crucial | highest level | crucial, plus the compound rule | new |
| Blast radius | symbols | size of the union over changed symbols | 50 | new |

The **compound rule** is part of significance. An exported signature change
(high) forces review when the blast radius is above 0, as long as the
significance threshold is on.

Each score's report entry lists the top contributing symbols, so the check
summary can say *which* function or symbol triggered review.

Defaults ship as informed guesses. A separate harness repo collects real
score distributions to tune them later; its brief is
[test-harness-brief.md](test-harness-brief.md).

### Fixed rules

These hold for every gate configuration, including all thresholds off:

1. An **unparsed file** always requires review.
2. A trusted author never skips review of an unparsed file.
3. Raising any score never turns review off.
4. Code in a language a measure supports, which lgtm then **fails to
   analyze**, requires review. This is treated like an unparsed file, so
   trust doesn't skip it either.

A measure not implemented for a file's language gives an **unavailable
score**. That never forces review; the gate treats that threshold as off.
All seven current languages get every measure at launch:

- Go
- Python
- Ruby
- JavaScript
- TypeScript/TSX
- Java
- Rust

So unavailable scores only matter for languages added later.

### Policies the measures carry

- **New code exemption.** The new measures look only at existing symbols,
  except new-function complexity. A narrow pure addition whose new
  functions score under 25 passes.
- **Deletion exemption.** The new measures treat deleting an existing symbol
  as no change when all three hold:
  - it isn't exported;
  - the resolved call graph finds no caller;
  - its name appears nowhere else in the repo as plain text.

  Edit depth stays frozen, so it still counts the deletion.
- **When unsure:** lenient about imprecise measurement, strict about
  missing coverage.
  - A call the resolver misses only shrinks blast radius.
  - A change the significance classifier can't place counts as medium.
  - A construct in a supported language that fails analysis triggers fixed
    rule 4.
- **Trusted authors** stay exempt from every threshold, but never from the
  fixed rules.

## The Alloy models

Run them with:

```bash
for f in gate blast_radius significance; do
  java -Djava.awt.headless=true -jar alloy/alloy.jar exec -f -o alloy/output/drafts alloy/drafts/$f.als
done
```

All assertions below hold (`UNSAT` = no counterexample), and every example
is satisfiable, unless marked otherwise.

### `gate.als`: the gate

`Scores` and `Gate` are plain sigs, so properties can compare two changes or
two configurations. A threshold that's switched off is a `lone Int` with no
value; an unavailable score is modeled the same way. `reaches[score, theta]`
needs both to be present.

| Assertion | Meaning |
| --- | --- |
| `UnparsedAlwaysRequiresReview` | fixed rule 1, every configuration |
| `AnalysisFailureAlwaysRequiresReview` | fixed rule 4 |
| `TrustedSkipsOnlyMeasuredChanges` | fixed rule 2, plus "trust exempts everything else" |
| `MonotoneInScores` | fixed rule 3; an unavailable score counts as the lowest value, so a score becoming available can't turn review off |
| `SwitchingOffNeverForcesReview` | switching a threshold off never adds review |
| `AllOffLeavesOnlyFixedRules` | all thresholds off ⇒ only the fixed rules remain |
| `FrozenV1` | new thresholds off ⇒ exactly the v1 rule |
| `ConservativeExtension` | everything v1 gates, the new gate gates |
| `UnavailableFallsBackToV1` | all new scores unavailable ⇒ v1 |
| `NewCodeExemption` | narrow pure additions with readable new functions pass |

Default-configuration examples, which become generated Go tests:

- a new function at 25 gates; at 24 it doesn't;
- a called exported signature change gates; an uncalled one doesn't;
- a blast radius of 50 gates;
- all new scores unavailable doesn't gate.

The examples run with an 8-bit integer scope, so 50 fits. The checks run with
5 bits: they compare scores with thresholds, which only depends on order, so
small values stand in for real ones.

A threshold of 0 is ruled out (the CLI rejects it). Alloy showed that in v1,
`theta-depth 0` makes every change need review, pure additions included.
"Off" is now the way to disable a threshold.

### `blast_radius.als`: affected set, blast radius, deletion exemption

The affected set is the changed symbols plus their callers within 3 hops on
the base graph, unrolled to match a bounded BFS. The blast radius is the
affected set minus the changed symbols.

| Assertion | Result |
| --- | --- |
| `BoundedWithinClosure`, `UncalledHasNoRadius`, `NewSymbolsHaveNoBaseRadius` | hold |
| `AffectedMonotoneInChangeSet` | holds |
| `MonotoneInEdges`: more edges never shrink it | holds. A resolver that misses calls is lenient, never strict |
| `ExemptionSoundUnderMissedCalls` | holds (see below) |
| `RadiusMonotoneInChangeSet` | **counterexample, kept on purpose**: also changing a caller removes it from the radius |

`ExemptionSoundUnderMissedCalls` covers the case where the resolved graph
misses edges of the real one. Every real call names its callee in the
caller's text, so the text check means an exempt deletion has no caller in
the *real* graph either. Removing the text check makes Alloy find a
counterexample, so the property really does depend on that check.

### `significance.als`: significance level per changed symbol

The level is the max over the parts of a symbol that changed:

| Part | Level |
| --- | --- |
| Rename (alpha-equivalent) | none |
| Statement | low |
| Condition, ErrorFlow | medium |
| Unclassified | medium |
| Signature | high if exported, else medium |
| Visibility | crucial if an export is removed, else medium |
| Supertypes | crucial |

The assertions — body-only ≤ medium, exported signature ≥ high, export
removal = crucial, unclassified ≥ medium, rename-only = none, monotone in
parts — all hold. Moving a symbol between files of one module never reaches
this model: under ADR 0001 a move isn't a change.

## Build order

### Phase 1 — promote the Alloy models

1. Replace `alloy/pr_review.als` and `properties.als` with `gate.als`. Keep
   v1's scenarios as `FrozenV1` examples.
2. Add `blast_radius.als` and `significance.als` to `make check`, each with
   its own output directory. Turn `RadiusMonotoneInChangeSet` into an
   expect-counterexample run.
3. Extend `cmd/genalloy`:
   - gate examples become `eval` tests, with switched-off thresholds and
     unavailable scores;
   - blast-radius instances (small graphs → expected affected set) become
     graph fixtures;
   - significance instances (parts → level) become classifier fixtures.
4. Update `alloy/check_run.als` if the check-run states change. Expect them
   not to: the action still gets "review required" or not.

### Phase 2 — groundwork

1. **Fix the call-depth crash.** `parse.longestPath` recurses forever on
   mutual recursion (queued as a separate task). Use Tarjan SCC plus DP
   over the resulting DAG. Edit depth's values must not change for code
   without call cycles.
2. **Record child field names** (`model.Node.Field`) using
   `FieldNameForChild`.
3. **Module-scoped symbols** (ADR 0001): `Symbol{Module, Container, Name,
   Kind}`, with a module resolver per language:
   - Go: package directory.
   - Java: package + class.
   - Python: dotted module path.
   - Rust: module path from `mod` declarations and files.
   - JavaScript/TypeScript: the file, with exports from ESM and CommonJS.
   - Ruby: constant path, merged across reopened classes.

   The differ pairs declarations across files within a module before
   diffing, and each `NodeChange` gets its owning symbol.
4. **Export predicate per language:**
   - Go: capitalized name.
   - Java: `public` / `protected`.
   - TypeScript: `export`.
   - JavaScript: ESM `export`, `module.exports`, `exports.x`.
   - Python: no leading `_`; `__all__` wins when it exists.
   - Rust: `pub` / `pub(crate)`.
   - Ruby: `private` / `protected` sections, `private :sym`, `private def`.
5. **Report schema.** Rename `eval.Metrics` → `Scores` and `"metrics"` →
   `"scores"`, and bump `schemaVersion`. Thresholds become optional
   ("off"), and the CLI rejects 0. Unavailable scores are reported
   explicitly, along with the top contributors for each score.

### Phase 3 — cognitive complexity

- **Algorithm:** one depth-first pass per function, carrying the nesting
  level: O(n). It follows Campbell's spec; pin the version we implement.
  - +1, plus the current nesting level, for: `if`, ternary,
    `switch`/`match`, loops, `catch`/`rescue`/`except`. Each of these also
    increases nesting.
  - +1 with no nesting penalty for: `else if` / `elif` / `elsif`, `else`,
    `goto`, labeled `break`/`continue`, and recursion.
  - +1 per run of the same boolean operator.
  - Nested functions and lambdas add nesting but no increment.
- **Per-language tables** go in `Spec`. Else-if shapes need field names:
  - Go's and Rust's `else if` sits inside the `alternative` field /
    `else_clause` and must not count as nested.
  - Python has `elif_clause`; Ruby has `elsif`.
- **Scores:**
  - complexity delta = the largest `head − base` over functions that exist
    on both sides (matched by module-scoped symbol);
  - new-function complexity = the largest score among new functions;
  - the report also carries each function's own delta, which can be
    negative.
- **Tests:**
  - Go cross-checked against
    [`uudashr/gocognit`](https://github.com/uudashr/gocognit);
  - Sonar's paper examples (`sumOfPrimes` = 7, `getWords` = 1) ported to
    all seven languages;
  - every place we deliberately differ from other implementations is
    documented (see [omen#514](https://github.com/panbanda/omen/issues/514)
    for where JS/TS implementations disagree).

### Phase 4 — significance

**Classifier.** For each pair of matched symbols that differ, compare
children field by field:

| Changed field / node | Part |
| --- | --- |
| `parameters`, result/return type, type parameters, receiver | Signature |
| superclass, interfaces, Go embedded fields, Rust trait bounds | Supertypes |
| modifiers, `visibility_modifier`, `export` wrapper, Go name case, Ruby visibility forms | Visibility |
| the `condition` field of if/while/for, the `switch` value | Condition |
| catch/rescue/except, throw/raise, Rust `?`, Go `panic`/`recover` | ErrorFlow |
| anything else in a body, including a new member added to an existing class | Statement |
| anything the rules above can't place | Unclassified |

**Rename detection.** Hash the function body with each locally bound
identifier replaced by its binding index (de Bruijn style). Equal hashes
mean the only change is a rename of locals (level none). This runs in O(n).

**Deleted symbols.** A deleted exported symbol is an export removal, which
is crucial. A deleted unexported symbol goes through the deletion exemption
(below).

### Phase 5 — blast radius and the deletion exemption

**Graph.** Built from the base scan. Calls are resolved by name in three
tiers:

1. the same file;
2. the same module;
3. imports, then a global name match.

Names that match more than N definitions are dropped. k = 3 and N are
**fixed**, and documented rather than configurable.

**Algorithm.**

- Build reverse adjacency.
- Run a multi-source BFS from the changed existing symbols, stopping at
  depth 3: O(V+E).
- Score: the size of the union, minus the changed symbols.
- Report the top symbols by direct caller count.

**Deletion exemption.** Uses the same graph, plus a plain-text scan of the
head tree for the deleted symbol's name. `ExemptionSoundUnderMissedCalls`
is why the scan is needed.

**Not now:**

| Option | Why not now |
| --- | --- |
| Betweenness centrality | O(VE), and noisy on a name-resolved graph |
| `golang.org/x/tools/go/callgraph` | Go only, and needs a buildable module |
| GitHub stack-graphs | archived Sep 2025 |
| SCIP indexers | one external toolchain per language |
| GumTree tree differencing | only if moves inside a body prove noisy |

### Phase 6 — action and docs

- Add action inputs, one per threshold, each accepting `off`; defaults as in
  the table above.
- Rewrite the check summary to list each score, its threshold, whether it
  fired, and its top contributors. Add a section on unavailable scores.
- README "How it decides" becomes the measures table plus the fixed rules.
  `docs/development.md` lists the new models and assertions.
- Release: no v2. v1 is treated as never released. When this ships, the
  owner deletes and recreates the existing `v1` tags, the GitHub release
  and the Marketplace listing by hand.

## Decision log (from the grilling sessions)

| # | Decision |
| --- | --- |
| 1 | CLI reports scores; action holds the gate (ADR 0002). |
| 2 | Keep edit depth and breadth alongside the new measures; they catch things cognitive complexity misses. |
| 3 | Lenient about imprecise measurement, strict about missing coverage; unclassified = medium. |
| 4 | Report new-function complexity; the default gate flags very complex new functions. |
| 5 | genalloy generates fixtures for blast radius and significance; cognitive complexity is checked against gocognit and the Sonar paper. |
| 6 | Symbols are module-scoped (ADR 0001). |
| 7, 12 | Deletion exemption: unexported, no resolved caller, name not mentioned elsewhere. |
| 8 | One score per measure; no composite score. |
| 9 | The default gate runs in Go; the JSON scores are a public output. |
| 10 | One threshold per measure, each can be switched off; custom rules later. |
| 11 | The four fixed rules. |
| 13 | Breadth reported in files and modules; the default gate uses files. |
| 14 | New-function complexity defaults to 25. |
| 15 | All seven languages at launch; unavailable scores are for later languages. |
| 16 | Scores in each measure's own unit. |
| 17 | PR scores aggregate as listed in the measures table. |
| 18 | No compatibility period: defaults on at once; rename to `scores` with a schema bump. |
| 19 | Edit depth frozen; call depth stays within one file. |
| 20 | Build order: Alloy → groundwork → cognitive complexity → significance → blast radius. |
| 21 | Ruby open classes, Ruby visibility forms and JS CommonJS + ESM exports are fully supported at launch. |
| 22 | Defaults ship as guesses; the harness repo collects data to tune them. |
| 23 | No v2; v1 is treated as never released. |
| 24 | k = 3 and the name-match cut-off are fixed internally. |
| 25 | The compound rule is controlled by the significance threshold. |
| 26 | The trusted-author exemption is unchanged; it never covers an unparsed file. |
| 27 | Edit depth stays exactly frozen, deletions included. An exempt deletion is ignored by the new measures but still counts toward edit depth. |
| 28 | Fixed rule 4 overrides trust: a trusted author doesn't skip review of code lgtm fails to analyze. |

### Implementation notes

- **"Frozen" edit depth means its definition, not v1's bugs.** v1's call-depth
  search crashed on mutual recursion and, on graphs where two call paths
  share a callee, returned the longest chain or a shorter one depending on
  Go's map order (measured: 2 instead of 3 in about 13% of runs on a
  four-function example). The SCC rewrite always returns the true longest
  chain. Edit depth can therefore differ from a v1 run, but only where v1
  was crashing or nondeterministic.
- **Two comparisons.** Edit depth and file breadth keep the path-based
  differ. The new measures compare module-scoped symbols (ADR 0001).

## Notes on the source report

The Gemini report ("Deterministic Evaluation of Pull Request Risk") gave us
the starting map: Campbell's metric, the ChangeDistiller taxonomy, bounded
reverse reachability. Parts of it don't hold up, though:

- Its numeric weights and 0–100 composite score aren't in the cited papers.
- "AxiomRefract (2024)" is a vendor's marketing page, not research.
- "Agent-LSP (2026)" and the "8.7% per file" figure have no source I could
  find.
- Its ΔCogC formula sums over nodes rather than comparing per function.

## References

- G. A. Campbell, *Cognitive Complexity* —
  https://www.sonarsource.com/docs/CognitiveComplexity.pdf
- B. Fluri, H. Gall, "Classifying Change Types for Qualifying Change
  Couplings", ICPC 2006.
- B. Fluri et al., "Change Distilling", IEEE TSE 33(11), 2007 —
  https://pinzger.github.io/papers/Fluri2007-changedistiller.pdf
- J.-R. Falleri et al., "Fine-grained and Accurate Source Code Differencing"
  (GumTree), ASE 2014.
- R. Tarjan, "Depth-First Search and Linear Graph Algorithms", 1972.
- stack-graphs (archived) — https://github.com/github/stack-graphs
- Gemini report — https://share.gemini.google/pf8H4rRFm1Oj
