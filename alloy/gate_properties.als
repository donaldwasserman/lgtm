-- Properties of the gate (gate.als). Every one is quantified over all
-- configurations, thresholds on or off.
--
-- Comparing two scores or two configurations uses `for 2`; the comparisons
-- depend only on order, so a 5-bit integer scope stands in for real values.
module gate_properties

open util/integer
open gate

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
     atLeast[a.called_significance, b.called_significance] and
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
        s.significance + s.called_significance + s.blast_radius)
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
