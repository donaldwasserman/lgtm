module pr_review

open util/integer

one sig Thresholds {
  theta_depth: one Int,
  theta_breadth: one Int,
  epsilon_trivial: one Int
}

abstract sig BOOL {}
one sig TRUE, FALSE extends BOOL {}

one sig Metrics {
  depth_mod: one Int,
  depth_new: one Int,
  breadth: one Int,
  depth_total: one Int,
  topTwenty: one BOOL,
  unparsed: one BOOL
}

fact ValidModel {
  int[Metrics.depth_mod] >= 0 and int[Metrics.depth_mod] <= 7
  int[Metrics.depth_new] >= 0 and int[Metrics.depth_new] <= 7
  int[Metrics.breadth] >= 0 and int[Metrics.breadth] <= 7
  int[Metrics.depth_total] >= 0 and int[Metrics.depth_total] <= 7
  int[Thresholds.theta_depth] >= 0 and int[Thresholds.theta_depth] <= 7
  int[Thresholds.theta_breadth] >= 0 and int[Thresholds.theta_breadth] <= 7
  int[Thresholds.epsilon_trivial] >= 0 and int[Thresholds.epsilon_trivial] <= 7
}

pred highDepthExisting[m: Metrics] {
  gte[int[m.depth_mod], int[Thresholds.theta_depth]]
}

pred broadAndNontrivial[m: Metrics] {
  gte[int[m.breadth], int[Thresholds.theta_breadth]] and
  gt[int[m.depth_total], int[Thresholds.epsilon_trivial]]
}

pred isTopTwenty[m: Metrics] {
  m.topTwenty = TRUE
}

-- Some file on either side could not be parsed, so the metrics below
-- do not describe the whole change.
pred isUnparsed[m: Metrics] {
  m.unparsed = TRUE
}

-- A trusted (top-20%) contributor is exempt from review,
-- overriding the normal depth/breadth rules.
--
-- An unparseable tree overrides everything, the exemption included: when the
-- metrics cannot describe the change, the gate must not wave it through.
pred requiresReview[m: Metrics] {
  isUnparsed[m] or
  ((not isTopTwenty[m]) and
   (highDepthExisting[m] or broadAndNontrivial[m]))
}
