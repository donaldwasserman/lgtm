module scenarios
open pr_review

run scenario_newDeepSubsystem {
  Metrics.depth_mod = 1 and Metrics.depth_new = 7
  Metrics.depth_total = 7 and Metrics.breadth = 2
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  not requiresReview[Metrics]
} for 3

run scenario_coreRefactor {
  Metrics.depth_mod = 7 and Metrics.depth_new = 2
  Metrics.depth_total = 7 and Metrics.breadth = 3
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  requiresReview[Metrics]
} for 3

run scenario_wideRename {
  Metrics.depth_mod = 1 and Metrics.depth_new = 1
  Metrics.depth_total = 1 and Metrics.breadth = 7
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  not requiresReview[Metrics]
} for 3

run scenario_crossCutting {
  Metrics.depth_mod = 3 and Metrics.depth_new = 5
  Metrics.depth_total = 5 and Metrics.breadth = 7
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  requiresReview[Metrics]
} for 3

run scenario_isolatedTypo {
  Metrics.depth_mod = 1 and Metrics.depth_new = 1
  Metrics.depth_total = 1 and Metrics.breadth = 1
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  not requiresReview[Metrics]
} for 3

run scenario_edgeAtThreshold {
  Metrics.depth_mod = 7 and Metrics.depth_new = 1
  Metrics.depth_total = 7 and Metrics.breadth = 6
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  requiresReview[Metrics]
} for 3

run scenario_broadButTrivial {
  Metrics.depth_mod = 1 and Metrics.depth_new = 1
  Metrics.depth_total = 1 and Metrics.breadth = 6
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  not requiresReview[Metrics]
} for 3

run scenario_allAtBoundaries {
  Metrics.depth_mod = 7 and Metrics.depth_new = 7
  Metrics.depth_total = 7 and Metrics.breadth = 7
  Metrics.topTwenty = FALSE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  requiresReview[Metrics]
} for 3

-- Top-20% contributor exemption: same metrics as scenario_coreRefactor (#2),
-- but the trusted submitter is exempt, so review is NOT required.
run scenario_trustedCoreRefactor {
  Metrics.depth_mod = 7 and Metrics.depth_new = 2
  Metrics.depth_total = 7 and Metrics.breadth = 3
  Metrics.topTwenty = TRUE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  not requiresReview[Metrics]
} for 3

-- Top-20% contributor exemption: same metrics as scenario_edgeAtThreshold (#6),
-- but the trusted submitter is exempt, so review is NOT required.
run scenario_trustedEdgeAtThreshold {
  Metrics.depth_mod = 7 and Metrics.depth_new = 1
  Metrics.depth_total = 7 and Metrics.breadth = 6
  Metrics.topTwenty = TRUE
  Thresholds.theta_depth = 7 and Thresholds.theta_breadth = 6
  Thresholds.epsilon_trivial = 1
  not requiresReview[Metrics]
} for 3
