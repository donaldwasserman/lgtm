-- The gate (docs/adr/0002-cli-scores-action-gates.md).
--
-- lgtm reports one score per measure; the gate turns scores into "needs
-- review". Each threshold can be switched off, which is modeled as the
-- threshold having no value. A score lgtm cannot produce for the change's
-- languages is "unavailable", modeled as the score having no value.
--
-- Scores and Gate are plain sigs, not `one sig`, so properties that compare
-- two changes or two configurations are not vacuous.
--
-- eval.RequiresReview implements requiresReview below. Its tests are
-- generated from gate_scenarios.als (make generate). Properties live in
-- gate_properties.als.
module gate

open util/integer

abstract sig BOOL {}
one sig TRUE, FALSE extends BOOL {}

-- Significance levels: 0 none, 1 low, 2 medium, 3 high, 4 crucial.
fun MEDIUM: Int { 2 }
fun HIGH: Int { 3 }

sig Scores {
  -- Measures that work on any file: always available.
  edit_depth: one Int,
  breadth_files: one Int,
  lines_added: one Int, -- in hundreds in the scenarios; see gate_scenarios.als
  uncovered_files: one Int,
  -- Non-blank lines that disappeared from test files: removed on the base
  -- side and not found again anywhere on the head side, so moving a test
  -- counts for nothing.
  test_lines_removed: one Int,
  -- Measures that need a language's symbols: absent means unavailable.
  breadth_modules: lone Int,
  cog_delta: lone Int,
  new_function_complexity: lone Int,
  significance: lone Int,
  -- The highest significance among changed symbols that something calls.
  -- It is what the compound rule reads: an exported signature change gates
  -- only when that symbol has callers, not when some other one does.
  called_significance: lone Int,
  -- Callers of changed symbols whose change is at least medium (a
  -- condition, error handling or more). A statement or constant edit in a
  -- widely called function does not count.
  blast_radius: lone Int,
  -- Some existing symbol changed (an exempt deletion does not count).
  touches_existing: one BOOL,
  -- Inputs to the rules no threshold applies to.
  unparsed: one BOOL,
  always_review: one BOOL
}

-- One team's configuration. Absent threshold = switched off.
sig Gate {
  theta_depth: lone Int,
  theta_breadth: lone Int,
  theta_modules: lone Int,
  theta_lines: lone Int,
  theta_uncovered: lone Int,
  theta_cog: lone Int,
  theta_new_function: lone Int,
  theta_significance: lone Int,
  theta_blast: lone Int,
  theta_test_deletions: lone Int
}

fact Ranges {
  all s: Scores {
    int[s.edit_depth] >= 0
    int[s.breadth_files] >= 0
    int[s.lines_added] >= 0
    int[s.uncovered_files] >= 0
    int[s.test_lines_removed] >= 0
    all i: s.breadth_modules | int[i] >= 0
    all i: s.cog_delta | int[i] >= 0
    all i: s.new_function_complexity | int[i] >= 0
    all i: s.significance | int[i] >= 0 and int[i] <= 4
    all i: s.called_significance | int[i] >= 0 and int[i] <= 4
    all i: s.blast_radius | int[i] >= 0
  }
  -- "Off" is the only way to disable a threshold; the CLI rejects 0, which
  -- would otherwise fire on every change (Alloy found this in v1).
  all g: Gate {
    all t: g.theta_depth + g.theta_breadth + g.theta_modules + g.theta_lines +
           g.theta_uncovered + g.theta_cog + g.theta_new_function + g.theta_blast +
           g.theta_test_deletions | int[t] >= 1
    all t: g.theta_significance | int[t] >= 1 and int[t] <= 4
  }
}

-- The new measures look only at existing symbols, except new-function
-- complexity, which exists to look at new code.
fact NewMeasuresLookAtExistingSymbols {
  all s: Scores | s.touches_existing = FALSE implies {
    all i: s.cog_delta | int[i] = 0
    all i: s.significance | int[i] = 0
    all i: s.called_significance | int[i] = 0
    all i: s.blast_radius | int[i] = 0
  }
}

-- Called significance is significance restricted to symbols with callers,
-- so never above significance. Both come from the same call graph, so it is
-- available exactly when blast radius is. Blast radius counts only callers
-- of changes of at least medium significance, so a positive radius means
-- some such change has a caller.
fact CalledSignificanceIsARestriction {
  all s: Scores {
    some s.called_significance iff (some s.significance and some s.blast_radius)
    all c: s.called_significance | lte[int[c], int[s.significance]]
    (some s.blast_radius and int[s.blast_radius] > 0) implies
      (some s.called_significance and gte[int[s.called_significance], MEDIUM])
  }
}

-- A score reaches a threshold only if both are present.
pred reaches[score: lone Int, theta: lone Int] {
  some score and some theta and gte[int[score], int[theta]]
}

-- Review regardless of every threshold.
pred forced[s: Scores] {
  s.unparsed = TRUE or s.always_review = TRUE
}

-- The measures that do not depend on a language's symbols.
pred universalFires[s: Scores, g: Gate] {
  reaches[s.edit_depth, g.theta_depth] or
  reaches[s.breadth_files, g.theta_breadth] or
  reaches[s.lines_added, g.theta_lines] or
  reaches[s.uncovered_files, g.theta_uncovered] or
  reaches[s.test_lines_removed, g.theta_test_deletions]
}

-- Significance, including the compound rule: an exported signature change
-- (high) to a symbol something calls gates, whenever the significance
-- threshold is on at all.
pred significanceFires[s: Scores, g: Gate] {
  some g.theta_significance and some s.significance and
  (gte[int[s.significance], int[g.theta_significance]] or
   (some s.called_significance and gte[int[s.called_significance], HIGH]))
}

pred anyThresholdFires[s: Scores, g: Gate] {
  universalFires[s, g] or
  reaches[s.breadth_modules, g.theta_modules] or
  reaches[s.cog_delta, g.theta_cog] or
  reaches[s.new_function_complexity, g.theta_new_function] or
  reaches[s.blast_radius, g.theta_blast] or
  significanceFires[s, g]
}

pred requiresReview[s: Scores, g: Gate] {
  forced[s] or anyThresholdFires[s, g]
}
