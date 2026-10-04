-- Example changes and configurations. Each `run` must be satisfiable, and
-- each becomes a case in eval/evaluator_alloy_test.go (make generate): the
-- instance supplies the scores and thresholds, and the body's
-- `requiresReview` / `not requiresReview` supplies the expected answer.
--
-- A field left without a value is meaningful: an unset threshold is
-- switched off, and an unset new score is unavailable. Every field that
-- should have a value must be pinned with `one`, or the solver picks one -
-- and since int[] of an empty set is 0, pinning a value to 0 is not enough.
module gate_scenarios

open util/integer
open gate

pred defaults[g: Gate] {
  int[g.theta_depth] = 7 and int[g.theta_breadth] = 6
  int[g.epsilon_trivial] = 1
  no g.theta_modules
  int[g.theta_cog] = 5
  int[g.theta_new_function] = 25
  int[g.theta_significance] = 4
  int[g.theta_blast] = 50
}

pred v1Only[g: Gate] {
  int[g.theta_depth] = 7 and int[g.theta_breadth] = 6
  int[g.epsilon_trivial] = 1
  no g.theta_modules + g.theta_cog + g.theta_new_function +
     g.theta_significance + g.theta_blast
}

pred noNewScores[s: Scores] {
  no s.breadth_modules + s.cog_delta + s.new_function_complexity +
     s.significance + s.blast_radius
}

pred plain[s: Scores] {
  s.unparsed = FALSE and s.analysis_failed = FALSE and s.trusted = FALSE
}

pred v1Scores[s: Scores, edit, total, files: Int] {
  int[s.edit_depth] = edit and int[s.depth_total] = total
  int[s.breadth_files] = files
}

-- Every new score available, with the given values. `one` matters: int[] of
-- an empty set is 0, so `int[s.cog_delta] = 0` alone would also admit an
-- unavailable score.
pred newScores[s: Scores, modules, cog, newFn, level, blast: Int] {
  one s.breadth_modules and one s.cog_delta and one s.new_function_complexity
  one s.significance and one s.blast_radius
  int[s.breadth_modules] = modules and int[s.cog_delta] = cog
  int[s.new_function_complexity] = newFn and int[s.significance] = level
  int[s.blast_radius] = blast
}

------------------------------------------------------------------------
-- The v1 examples, unchanged: default gate, new scores unavailable.

run scenario_newDeepSubsystem {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 1, 7, 2] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_coreRefactor {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 7, 7, 3] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_wideRename {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 1, 1, 7] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_crossCutting {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 3, 5, 7] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_isolatedTypo {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 1, 1, 1] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_edgeAtThreshold {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 7, 7, 6] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_broadButTrivial {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 1, 1, 6] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_allAtBoundaries {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    v1Scores[s, 7, 7, 7] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_trustedCoreRefactor {
  all s: Scores, g: Gate | defaults[g] and noNewScores[s] and
    s.unparsed = FALSE and s.analysis_failed = FALSE and s.trusted = TRUE and
    v1Scores[s, 7, 7, 3] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_unparsedTrivial {
  all s: Scores, g: Gate | defaults[g] and noNewScores[s] and
    s.unparsed = TRUE and s.analysis_failed = FALSE and s.trusted = FALSE and
    v1Scores[s, 1, 1, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_trustedUnparsed {
  all s: Scores, g: Gate | defaults[g] and noNewScores[s] and
    s.unparsed = TRUE and s.analysis_failed = FALSE and s.trusted = TRUE and
    v1Scores[s, 1, 1, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

------------------------------------------------------------------------
-- Fixed rules.

run scenario_analysisFailed {
  all s: Scores, g: Gate | defaults[g] and newScores[s, 1, 0, 0, 0, 0] and
    s.unparsed = FALSE and s.analysis_failed = TRUE and s.trusted = FALSE and
    v1Scores[s, 1, 1, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_trustedAnalysisFailed {
  all s: Scores, g: Gate | defaults[g] and newScores[s, 1, 0, 0, 0, 0] and
    s.unparsed = FALSE and s.analysis_failed = TRUE and s.trusted = TRUE and
    v1Scores[s, 1, 1, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_allOffUnparsed {
  all s: Scores, g: Gate |
    no g.theta_depth + g.theta_breadth + g.theta_modules + g.theta_cog +
       g.theta_new_function + g.theta_significance + g.theta_blast and
    int[g.epsilon_trivial] = 1 and noNewScores[s] and
    s.unparsed = TRUE and s.analysis_failed = FALSE and s.trusted = FALSE and
    v1Scores[s, 1, 1, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_allOffDeepAndWide {
  all s: Scores, g: Gate |
    no g.theta_depth + g.theta_breadth + g.theta_modules + g.theta_cog +
       g.theta_new_function + g.theta_significance + g.theta_blast and
    int[g.epsilon_trivial] = 1 and plain[s] and
    v1Scores[s, 20, 20, 30] and int[s.breadth_modules] = 9 and
    int[s.cog_delta] = 40 and int[s.new_function_complexity] = 60 and
    int[s.significance] = 4 and int[s.blast_radius] = 100 and
    s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

------------------------------------------------------------------------
-- The new measures under the default gate.

run scenario_newFunctionAt25 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 25, 0, 0] and
    v1Scores[s, 0, 3, 1] and s.touches_existing = FALSE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_newFunctionAt24 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 24, 0, 0] and
    v1Scores[s, 0, 3, 1] and s.touches_existing = FALSE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_cogDeltaAt5 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 5, 0, 0, 0] and
    v1Scores[s, 3, 3, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_cogDeltaAt4 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 4, 0, 0, 0] and
    v1Scores[s, 3, 3, 1] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_crucialChange {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 4, 0] and
    v1Scores[s, 2, 2, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_trustedCrucialChange {
  all s: Scores, g: Gate | defaults[g] and newScores[s, 1, 0, 0, 4, 200] and
    s.unparsed = FALSE and s.analysis_failed = FALSE and s.trusted = TRUE and
    v1Scores[s, 2, 2, 1] and s.touches_existing = TRUE and
    not requiresReview[s, g]
} for 9 Int, exactly 1 Scores, exactly 1 Gate

run scenario_calledExportedSignature {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 3, 1] and
    v1Scores[s, 2, 2, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_uncalledExportedSignature {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 3, 0] and
    v1Scores[s, 2, 2, 1] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_exportedSignatureBlastUnavailable {
  all s: Scores, g: Gate | defaults[g] and plain[s] and
    one s.breadth_modules and one s.cog_delta and one s.new_function_complexity and
    one s.significance and
    int[s.breadth_modules] = 1 and int[s.cog_delta] = 0 and
    int[s.new_function_complexity] = 0 and int[s.significance] = 3 and
    no s.blast_radius and
    v1Scores[s, 2, 2, 1] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_blastAt50 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 1, 50] and
    v1Scores[s, 2, 2, 1] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_blastAt49 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 1, 49] and
    v1Scores[s, 2, 2, 1] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_manyModulesDefaultOff {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 5, 0, 0, 0, 0] and
    v1Scores[s, 1, 1, 5] and
    s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_manyModulesThresholdOn {
  all s: Scores, g: Gate |
    int[g.theta_depth] = 7 and int[g.theta_breadth] = 6 and
    int[g.epsilon_trivial] = 1 and int[g.theta_modules] = 3 and
    int[g.theta_cog] = 5 and int[g.theta_new_function] = 25 and
    int[g.theta_significance] = 4 and int[g.theta_blast] = 50 and
    plain[s] and newScores[s, 5, 0, 0, 0, 0] and
    v1Scores[s, 1, 1, 5] and
    s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_v1OnlyIgnoresNewScores {
  all s: Scores, g: Gate | v1Only[g] and plain[s] and
    v1Scores[s, 2, 2, 1] and int[s.breadth_modules] = 1 and
    int[s.cog_delta] = 40 and int[s.new_function_complexity] = 60 and
    int[s.significance] = 4 and int[s.blast_radius] = 100 and
    s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate
