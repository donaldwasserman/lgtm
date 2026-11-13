-- Example changes and configurations. Each `run` must be satisfiable, and
-- each becomes a case in eval/evaluator_alloy_test.go (make generate): the
-- instance supplies the scores and thresholds, and the body's
-- `requiresReview` / `not requiresReview` supplies the expected answer.
--
-- A field left without a value is meaningful: an unset threshold is
-- switched off, and an unset score is unavailable. Every field that
-- should have a value must be pinned with `one`, or the solver picks one -
-- and since int[] of an empty set is 0, pinning a value to 0 is not enough.
module gate_scenarios

open util/integer
open gate

-- Lines are written here in hundreds, so the default of 7000 fits the 8-bit
-- scope: the gate only compares them. cmd/genalloy scales them back.
--
-- The "balanced" preset. Edit depth is off.
pred defaults[g: Gate] {
  no g.theta_depth
  int[g.theta_breadth] = 25
  int[g.theta_modules] = 4
  int[g.theta_lines] = 70
  int[g.theta_uncovered] = 1
  int[g.theta_cog] = 5
  int[g.theta_new_function] = 25
  int[g.theta_significance] = 4
  int[g.theta_blast] = 30
  int[g.theta_test_deletions] = 100
}

pred noNewScores[s: Scores] {
  no s.breadth_modules + s.cog_delta + s.new_function_complexity +
     s.significance + s.called_significance + s.blast_radius
}

pred plain[s: Scores] {
  s.unparsed = FALSE and s.always_review = FALSE
}

pred universal[s: Scores, edit, files, lines, uncovered: Int] {
  universalTests[s, edit, files, lines, uncovered, 0]
}

pred universalTests[s: Scores, edit, files, lines, uncovered, testsRemoved: Int] {
  int[s.edit_depth] = edit and int[s.breadth_files] = files
  int[s.lines_added] = lines and int[s.uncovered_files] = uncovered
  int[s.test_lines_removed] = testsRemoved
}

-- Every symbol-based score available, with the given values. `one` matters:
-- int[] of an empty set is 0, so `int[s.cog_delta] = 0` alone would also
-- admit an unavailable score.
pred newScores[s: Scores, modules, cog, newFn, level, called, blast: Int] {
  one s.breadth_modules and one s.cog_delta and one s.new_function_complexity
  one s.significance and one s.called_significance and one s.blast_radius
  int[s.breadth_modules] = modules and int[s.cog_delta] = cog
  int[s.new_function_complexity] = newFn and int[s.significance] = level
  int[s.called_significance] = called and int[s.blast_radius] = blast
}

pred allOff[g: Gate] {
  no g.theta_depth + g.theta_breadth + g.theta_modules + g.theta_lines +
     g.theta_uncovered + g.theta_cog + g.theta_new_function +
     g.theta_significance + g.theta_blast + g.theta_test_deletions
}

------------------------------------------------------------------------
-- The default gate.

run scenario_coreRefactor {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 5, 0, 1, 0, 0] and
    universal[s, 3, 3, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_isolatedTypo {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 1, 0, 0] and
    universal[s, 5, 1, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_deepEditAlone {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 1, 0, 0] and
    universal[s, 20, 1, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_breadthAt25 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 3, 0, 0, 1, 0, 0] and
    universal[s, 2, 25, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_breadthAt24 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 3, 0, 0, 1, 0, 0] and
    universal[s, 2, 24, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_modulesAt4 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 4, 0, 0, 1, 0, 0] and
    universal[s, 2, 4, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_modulesAt3 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 3, 0, 0, 1, 0, 0] and
    universal[s, 2, 3, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_linesAtThreshold {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 0, 0, 0] and
    universal[s, 0, 1, 70, 0] and s.touches_existing = FALSE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_linesBelowThreshold {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 0, 0, 0] and
    universal[s, 0, 1, 69, 0] and s.touches_existing = FALSE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_oneUncoveredFile {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 0, 0, 0, 0, 0, 0] and
    universal[s, 0, 0, 0, 1] and s.touches_existing = FALSE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_noUncoveredFiles {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 0, 0, 0, 0, 0, 0] and
    universal[s, 0, 0, 0, 0] and s.touches_existing = FALSE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_newFunctionAt25 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 25, 0, 0, 0] and
    universal[s, 0, 1, 1, 0] and s.touches_existing = FALSE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_newFunctionAt24 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 24, 0, 0, 0] and
    universal[s, 0, 1, 1, 0] and s.touches_existing = FALSE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_cogDeltaAt5 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 5, 0, 1, 0, 0] and
    universal[s, 3, 1, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_cogDeltaAt4 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 4, 0, 1, 0, 0] and
    universal[s, 3, 1, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_crucialChange {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 4, 0, 0] and
    universal[s, 2, 1, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_calledExportedSignature {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 3, 3, 1] and
    universal[s, 2, 1, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_uncalledExportedSignature {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 3, 0, 0] and
    universal[s, 2, 1, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

-- The refinement called_significance exists for: an uncalled exported
-- signature change does not gate just because some other changed symbol
-- has callers.

run scenario_uncalledSignatureBesideCalledEdit {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 2, 0, 0, 3, 2, 10] and
    universal[s, 2, 2, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_blastAt30 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 2, 2, 30] and
    universal[s, 2, 1, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_blastAt29 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 2, 2, 29] and
    universal[s, 2, 1, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

-- A statement or constant edit in a function everything calls: blast
-- radius counts only changes of at least medium significance, so it is 0.
run scenario_lowEditInWidelyCalledFunction {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 1, 0, 0, 1, 1, 0] and
    universal[s, 2, 1, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

-- A new form, endpoint and business logic: many new files in several
-- modules, wired in with a small edit. Only the wiring counts as breadth.
run scenario_featureAddition {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 2, 0, 18, 1, 1, 0] and
    universal[s, 2, 2, 6, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_testDeletionsAt100 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 0, 0, 0, 0, 0, 0] and
    universalTests[s, 0, 0, 0, 0, 100] and s.touches_existing = FALSE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_testDeletionsAt99 {
  all s: Scores, g: Gate | defaults[g] and plain[s] and newScores[s, 0, 0, 0, 0, 0, 0] and
    universalTests[s, 0, 0, 0, 0, 99] and s.touches_existing = FALSE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_exportedSignatureBlastUnavailable {
  all s: Scores, g: Gate | defaults[g] and plain[s] and
    one s.breadth_modules and one s.cog_delta and one s.new_function_complexity and
    one s.significance and
    int[s.breadth_modules] = 1 and int[s.cog_delta] = 0 and
    int[s.new_function_complexity] = 0 and int[s.significance] = 3 and
    no s.blast_radius and no s.called_significance and
    universal[s, 2, 1, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_noSymbolSupportStillCountsFiles {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    universal[s, 2, 25, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_noSymbolSupportPassesWhenSmall {
  all s: Scores, g: Gate | defaults[g] and plain[s] and noNewScores[s] and
    universal[s, 2, 2, 1, 0] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

------------------------------------------------------------------------
-- Rules no threshold can switch off.

run scenario_unparsedTrivial {
  all s: Scores, g: Gate | defaults[g] and noNewScores[s] and
    s.unparsed = TRUE and s.always_review = FALSE and
    universal[s, 1, 1, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_alwaysReviewPath {
  all s: Scores, g: Gate | defaults[g] and newScores[s, 0, 0, 0, 0, 0, 0] and
    s.unparsed = FALSE and s.always_review = TRUE and
    universal[s, 0, 0, 0, 0] and s.touches_existing = FALSE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_allOffUnparsed {
  all s: Scores, g: Gate | allOff[g] and noNewScores[s] and
    s.unparsed = TRUE and s.always_review = FALSE and
    universal[s, 1, 1, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_allOffAlwaysReview {
  all s: Scores, g: Gate | allOff[g] and noNewScores[s] and
    s.unparsed = FALSE and s.always_review = TRUE and
    universal[s, 0, 0, 0, 0] and s.touches_existing = FALSE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_allOffDeepAndWide {
  all s: Scores, g: Gate | allOff[g] and plain[s] and newScores[s, 9, 40, 60, 4, 4, 100] and
    universalTests[s, 20, 30, 1, 9, 120] and s.touches_existing = TRUE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

------------------------------------------------------------------------
-- Thresholds a team can switch back on or off.

run scenario_depthSwitchedOn {
  all s: Scores, g: Gate | int[g.theta_depth] = 7 and
    int[g.theta_breadth] = 25 and int[g.theta_modules] = 4 and int[g.theta_lines] = 70 and
    int[g.theta_uncovered] = 1 and int[g.theta_cog] = 5 and int[g.theta_new_function] = 25 and
    int[g.theta_significance] = 4 and int[g.theta_blast] = 30 and
    int[g.theta_test_deletions] = 100 and
    plain[s] and newScores[s, 1, 0, 0, 1, 0, 0] and
    universal[s, 7, 1, 1, 0] and s.touches_existing = TRUE and requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate

run scenario_uncoveredSwitchedOff {
  all s: Scores, g: Gate | no g.theta_depth and no g.theta_uncovered and
    int[g.theta_breadth] = 25 and int[g.theta_modules] = 4 and int[g.theta_lines] = 70 and
    int[g.theta_cog] = 5 and int[g.theta_new_function] = 25 and
    int[g.theta_significance] = 4 and int[g.theta_blast] = 30 and
    int[g.theta_test_deletions] = 100 and
    plain[s] and newScores[s, 0, 0, 0, 0, 0, 0] and
    universal[s, 0, 0, 0, 3] and s.touches_existing = FALSE and not requiresReview[s, g]
} for 8 Int, exactly 1 Scores, exactly 1 Gate
