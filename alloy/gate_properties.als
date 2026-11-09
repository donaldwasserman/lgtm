-- Properties of the gate (gate.als). Every one is quantified over all
-- configurations, thresholds on or off.
--
-- Comparing two scores or two configurations uses `for 2`; the comparisons
-- depend only on order, so a 5-bit integer scope stands in for real values.
module gate_properties

open util/integer
open gate

----------------------------------------------------------------------------
-- Rules no threshold can switch off.

assert UnparsedAlwaysRequiresReview {
  all s: Scores, g: Gate | s.unparsed = TRUE implies requiresReview[s, g]
}

assert AlwaysReviewPathAlwaysRequiresReview {
  all s: Scores, g: Gate | s.always_review = TRUE implies requiresReview[s, g]
}

-- "a is at least b" with an unavailable score as the bottom.
pred atLeast[a, b: lone Int] { no b or (some a and gte[int[a], int[b]]) }

pred sameInputs[a, b: Scores] {
  a.touches_existing = b.touches_existing
  a.unparsed = b.unparsed and a.always_review = b.always_review
}

-- Raising any score - including from unavailable to available - never turns
-- review off.
assert MonotoneInScores {
  all a, b: Scores, g: Gate |
    (sameInputs[a, b] and
     atLeast[a.edit_depth, b.edit_depth] and
     atLeast[a.breadth_files, b.breadth_files] and
     atLeast[a.lines_added, b.lines_added] and
     atLeast[a.uncovered_files, b.uncovered_files] and
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
  g1.theta_depth in g2.theta_depth
  g1.theta_breadth in g2.theta_breadth
  g1.theta_modules in g2.theta_modules
  g1.theta_lines in g2.theta_lines
  g1.theta_uncovered in g2.theta_uncovered
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
  no g.theta_depth + g.theta_breadth + g.theta_modules + g.theta_lines +
     g.theta_uncovered + g.theta_cog + g.theta_new_function +
     g.theta_significance + g.theta_blast
}

assert AllOffLeavesOnlyForcedRules {
  all s: Scores, g: Gate | allOff[g] implies (requiresReview[s, g] iff forced[s])
}

-- A language with no symbol support still gets the measures that need none.
assert UnavailableFallsBackToUniversal {
  all s: Scores, g: Gate |
    (no s.breadth_modules + s.cog_delta + s.new_function_complexity +
        s.significance + s.called_significance + s.blast_radius)
    implies (requiresReview[s, g] iff (forced[s] or universalFires[s, g]))
}

-- Narrow pure additions with readable new functions still pass.
assert NewCodeExemption {
  all s: Scores, g: Gate |
    (not forced[s] and s.touches_existing = FALSE and
     not universalFires[s, g] and
     not reaches[s.breadth_modules, g.theta_modules] and
     not reaches[s.new_function_complexity, g.theta_new_function])
    implies not requiresReview[s, g]
}

check UnparsedAlwaysRequiresReview for 2 but 5 Int
check AlwaysReviewPathAlwaysRequiresReview for 2 but 5 Int
check MonotoneInScores for 2 but 5 Int
check SwitchingOffNeverForcesReview for 2 but 5 Int
check AllOffLeavesOnlyForcedRules for 2 but 5 Int
check UnavailableFallsBackToUniversal for 2 but 5 Int
check NewCodeExemption for 2 but 5 Int
