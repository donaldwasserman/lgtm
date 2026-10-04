-- DRAFT: the gate (docs/adr/0002-cli-scores-action-gates.md).
--
-- lgtm reports one score per measure; the gate turns scores into "needs
-- review". Each threshold can be switched off, which is modeled as the
-- threshold having no value. A score lgtm cannot produce for the change's
-- languages is "unavailable", modeled as the score having no value.
--
-- Scores and Gate are plain sigs, not `one sig`, so properties that compare
-- two changes or two configurations are not vacuous.
module gate

open util/integer

abstract sig BOOL {}
one sig TRUE, FALSE extends BOOL {}

-- Significance levels: 0 none, 1 low, 2 medium, 3 high, 4 crucial.
fun HIGH: Int { 3 }

sig Scores {
  -- Frozen v1 measures: always available.
  edit_depth: one Int,
  depth_total: one Int,
  breadth_files: one Int,
  -- New measures: absent means unavailable.
  breadth_modules: lone Int,
  cog_delta: lone Int,
  new_function_complexity: lone Int,
  significance: lone Int,
  blast_radius: lone Int,
  -- Some existing symbol changed (an exempt deletion does not count).
  touches_existing: one BOOL,
  -- Inputs to the fixed rules.
  unparsed: one BOOL,
  analysis_failed: one BOOL,
  trusted: one BOOL
}

-- One team's configuration. Absent threshold = switched off.
sig Gate {
  theta_depth: lone Int,
  theta_breadth: lone Int,
  epsilon_trivial: one Int,
  theta_modules: lone Int,
  theta_cog: lone Int,
  theta_new_function: lone Int,
  theta_significance: lone Int,
  theta_blast: lone Int
}

fact Ranges {
  all s: Scores {
    int[s.edit_depth] >= 0
    int[s.depth_total] >= 0
    int[s.breadth_files] >= 0
    all i: s.breadth_modules | int[i] >= 0
    all i: s.cog_delta | int[i] >= 0
    all i: s.new_function_complexity | int[i] >= 0
    all i: s.significance | int[i] >= 0 and int[i] <= 4
    all i: s.blast_radius | int[i] >= 0
  }
  -- "Off" is the only way to disable a threshold; the CLI rejects 0, which
  -- would otherwise fire on every change (Alloy found this in v1).
  all g: Gate {
    all t: g.theta_depth + g.theta_breadth + g.theta_modules + g.theta_cog +
           g.theta_new_function + g.theta_blast | int[t] >= 1
    all t: g.theta_significance | int[t] >= 1 and int[t] <= 4
    int[g.epsilon_trivial] >= 0
  }
}

-- The new measures look only at existing symbols, except new-function
-- complexity, which exists to look at new code.
fact NewMeasuresLookAtExistingSymbols {
  all s: Scores | s.touches_existing = FALSE implies {
    all i: s.cog_delta | int[i] = 0
    all i: s.significance | int[i] = 0
    all i: s.blast_radius | int[i] = 0
  }
}

-- A score reaches a threshold only if both are present.
pred reaches[score: lone Int, theta: lone Int] {
  some score and some theta and gte[int[score], int[theta]]
}

pred unmeasured[s: Scores] {
  s.unparsed = TRUE or s.analysis_failed = TRUE
}

-- The v1 rule, frozen. Its thresholds can now be switched off too.
pred v1Fires[s: Scores, g: Gate] {
  reaches[s.edit_depth, g.theta_depth] or
  (reaches[s.breadth_files, g.theta_breadth] and
   gt[int[s.depth_total], int[g.epsilon_trivial]])
}

-- Significance, including the compound rule: an exported signature change
-- (high) gates when something calls it, whenever the significance threshold
-- is on at all.
pred significanceFires[s: Scores, g: Gate] {
  some g.theta_significance and some s.significance and
  (gte[int[s.significance], int[g.theta_significance]] or
   (gte[int[s.significance], HIGH] and some s.blast_radius and
    gt[int[s.blast_radius], 0]))
}

pred anyThresholdFires[s: Scores, g: Gate] {
  v1Fires[s, g] or
  reaches[s.breadth_modules, g.theta_modules] or
  reaches[s.cog_delta, g.theta_cog] or
  reaches[s.new_function_complexity, g.theta_new_function] or
  reaches[s.blast_radius, g.theta_blast] or
  significanceFires[s, g]
}

pred requiresReview[s: Scores, g: Gate] {
  unmeasured[s] or (s.trusted = FALSE and anyThresholdFires[s, g])
}

----------------------------------------------------------------------------
-- Fixed rules: hold for every Gate, every threshold on or off.

assert UnparsedAlwaysRequiresReview {
  all s: Scores, g: Gate | s.unparsed = TRUE implies requiresReview[s, g]
}

-- Fixed rule 4: a supported language lgtm fails to analyze. Treated like an
-- unparsed file, so trust does not skip it either.
assert AnalysisFailureAlwaysRequiresReview {
  all s: Scores, g: Gate | s.analysis_failed = TRUE implies requiresReview[s, g]
}

assert TrustedSkipsOnlyMeasuredChanges {
  all s: Scores, g: Gate |
    (s.trusted = TRUE and not unmeasured[s]) implies not requiresReview[s, g]
}

-- "a is at least b" with an unavailable score as the bottom.
pred atLeast[a, b: lone Int] { no b or (some a and gte[int[a], int[b]]) }

pred sameInputs[a, b: Scores] {
  a.touches_existing = b.touches_existing
  a.unparsed = b.unparsed and a.analysis_failed = b.analysis_failed
  a.trusted = b.trusted
}

-- Raising any score - including from unavailable to available - never turns
-- review off.
assert MonotoneInScores {
  all a, b: Scores, g: Gate |
    (sameInputs[a, b] and
     atLeast[a.edit_depth, b.edit_depth] and
     atLeast[a.depth_total, b.depth_total] and
     atLeast[a.breadth_files, b.breadth_files] and
     atLeast[a.breadth_modules, b.breadth_modules] and
     atLeast[a.cog_delta, b.cog_delta] and
     atLeast[a.new_function_complexity, b.new_function_complexity] and
     atLeast[a.significance, b.significance] and
     atLeast[a.blast_radius, b.blast_radius] and
     requiresReview[b, g])
    implies requiresReview[a, g]
}

----------------------------------------------------------------------------
-- Configuration properties.

-- g1's thresholds are g2's with some switched off.
pred fewerOn[g1, g2: Gate] {
  g1.epsilon_trivial = g2.epsilon_trivial
  g1.theta_depth in g2.theta_depth
  g1.theta_breadth in g2.theta_breadth
  g1.theta_modules in g2.theta_modules
  g1.theta_cog in g2.theta_cog
  g1.theta_new_function in g2.theta_new_function
  g1.theta_significance in g2.theta_significance
  g1.theta_blast in g2.theta_blast
}

assert SwitchingOffNeverForcesReview {
  all s: Scores, g1, g2: Gate |
    (fewerOn[g1, g2] and requiresReview[s, g1]) implies requiresReview[s, g2]
}

pred allOff[g: Gate] {
  no g.theta_depth + g.theta_breadth + g.theta_modules + g.theta_cog +
     g.theta_new_function + g.theta_significance + g.theta_blast
}

assert AllOffLeavesOnlyFixedRules {
  all s: Scores, g: Gate | allOff[g] implies (requiresReview[s, g] iff unmeasured[s])
}

pred newMeasuresOff[g: Gate] {
  no g.theta_modules + g.theta_cog + g.theta_new_function +
     g.theta_significance + g.theta_blast
}

-- With the new thresholds off, the gate is exactly v1 (plus fixed rule 4).
assert FrozenV1 {
  all s: Scores, g: Gate | newMeasuresOff[g] implies
    (requiresReview[s, g] iff
      (unmeasured[s] or (s.trusted = FALSE and v1Fires[s, g])))
}

-- Every change v1 gates, the new gate gates.
assert ConservativeExtension {
  all s: Scores, g: Gate |
    (unmeasured[s] or (s.trusted = FALSE and v1Fires[s, g])) implies requiresReview[s, g]
}

-- Every new score unavailable: the gate falls back to v1.
assert UnavailableFallsBackToV1 {
  all s: Scores, g: Gate |
    (no s.breadth_modules + s.cog_delta + s.new_function_complexity +
        s.significance + s.blast_radius)
    implies (requiresReview[s, g] iff
              (unmeasured[s] or (s.trusted = FALSE and v1Fires[s, g])))
}

-- Narrow pure additions with readable new functions still pass.
assert NewCodeExemption {
  all s: Scores, g: Gate |
    (not unmeasured[s] and s.touches_existing = FALSE and
     not reaches[s.edit_depth, g.theta_depth] and
     not reaches[s.breadth_files, g.theta_breadth] and
     not reaches[s.breadth_modules, g.theta_modules] and
     not reaches[s.new_function_complexity, g.theta_new_function])
    implies not requiresReview[s, g]
}

----------------------------------------------------------------------------
-- The shipped defaults, as examples. These become generated Go tests.

pred defaults[g: Gate] {
  int[g.theta_depth] = 7 and int[g.theta_breadth] = 6
  int[g.epsilon_trivial] = 1
  no g.theta_modules
  int[g.theta_cog] = 5
  int[g.theta_new_function] = 25
  int[g.theta_significance] = 4
  int[g.theta_blast] = 50
}

pred quiet[s: Scores] {
  s.unparsed = FALSE and s.analysis_failed = FALSE and s.trusted = FALSE
  int[s.edit_depth] = 1 and int[s.depth_total] = 1 and int[s.breadth_files] = 1
  int[s.breadth_modules] = 1
}

run defaults_newFunctionAt25 {
  some s: Scores, g: Gate | defaults[g] and quiet[s] and
    s.touches_existing = FALSE and
    int[s.new_function_complexity] = 25 and int[s.cog_delta] = 0 and
    int[s.significance] = 0 and int[s.blast_radius] = 0 and
    requiresReview[s, g]
} for 1 but 8 Int

run defaults_newFunctionAt24 {
  some s: Scores, g: Gate | defaults[g] and quiet[s] and
    s.touches_existing = FALSE and
    int[s.new_function_complexity] = 24 and int[s.cog_delta] = 0 and
    int[s.significance] = 0 and int[s.blast_radius] = 0 and
    not requiresReview[s, g]
} for 1 but 8 Int

run defaults_calledExportedSignature {
  some s: Scores, g: Gate | defaults[g] and quiet[s] and
    s.touches_existing = TRUE and
    int[s.new_function_complexity] = 0 and int[s.cog_delta] = 0 and
    int[s.significance] = 3 and int[s.blast_radius] = 1 and
    requiresReview[s, g]
} for 1 but 8 Int

run defaults_uncalledExportedSignature {
  some s: Scores, g: Gate | defaults[g] and quiet[s] and
    s.touches_existing = TRUE and
    int[s.new_function_complexity] = 0 and int[s.cog_delta] = 0 and
    int[s.significance] = 3 and int[s.blast_radius] = 0 and
    not requiresReview[s, g]
} for 1 but 8 Int

run defaults_blastAt50 {
  some s: Scores, g: Gate | defaults[g] and quiet[s] and
    s.touches_existing = TRUE and
    int[s.new_function_complexity] = 0 and int[s.cog_delta] = 0 and
    int[s.significance] = 1 and int[s.blast_radius] = 50 and
    requiresReview[s, g]
} for 1 but 8 Int

run defaults_unavailableScoresFallBack {
  some s: Scores, g: Gate | defaults[g] and quiet[s] and
    s.touches_existing = TRUE and
    no s.cog_delta and no s.new_function_complexity and
    no s.significance and no s.blast_radius and
    not requiresReview[s, g]
} for 1 but 8 Int

check UnparsedAlwaysRequiresReview for 2 but 5 Int
check AnalysisFailureAlwaysRequiresReview for 2 but 5 Int
check TrustedSkipsOnlyMeasuredChanges for 2 but 5 Int
check MonotoneInScores for 2 but 5 Int
check SwitchingOffNeverForcesReview for 2 but 5 Int
check AllOffLeavesOnlyFixedRules for 2 but 5 Int
check FrozenV1 for 2 but 5 Int
check ConservativeExtension for 2 but 5 Int
check UnavailableFallsBackToV1 for 2 but 5 Int
check NewCodeExemption for 2 but 5 Int
