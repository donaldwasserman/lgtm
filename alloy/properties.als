module properties
open pr_review

assert NewCodeExemption {
  all m: Metrics | {
    (not isUnparsed[m] and
     not highDepthExisting[m] and
     gte[int[m.depth_new], int[Thresholds.theta_depth]] and
     lt[int[m.breadth], int[Thresholds.theta_breadth]])
    implies not requiresReview[m]
  }
}

assert TrivialDepthExemption {
  all m: Metrics | {
    (not isUnparsed[m] and
     lte[int[m.depth_total], int[Thresholds.epsilon_trivial]] and
     not highDepthExisting[m])
    implies not requiresReview[m]
  }
}

assert HighDepthModRequiresReview {
  all m: Metrics | {
    (not isTopTwenty[m] and highDepthExisting[m])
    implies requiresReview[m]
  }
}

assert BroadNontrivialRequiresReview {
  all m: Metrics | {
    (not isTopTwenty[m] and broadAndNontrivial[m])
    implies requiresReview[m]
  }
}

assert LowEverythingNeverRequires {
  all m: Metrics | {
    (not isUnparsed[m] and
     not highDepthExisting[m] and
     lt[int[m.breadth], int[Thresholds.theta_breadth]])
    implies not requiresReview[m]
  }
}

assert MonotoneInDepthMod {
  all m1, m2: Metrics | {
    (not isTopTwenty[m1] and not isTopTwenty[m2] and
     gte[int[m1.depth_mod], int[m2.depth_mod]] and
     requiresReview[m2])
    implies requiresReview[m1]
  }
}

-- The top-20% exemption applies only when the change could actually be
-- measured; an unparseable tree is reviewed no matter who submitted it.
assert TrustedContributorExemption {
  all m: Metrics | {
    (isTopTwenty[m] and not isUnparsed[m]) implies not requiresReview[m]
  }
}

assert UnparsedAlwaysRequiresReview {
  all m: Metrics | {
    isUnparsed[m] implies requiresReview[m]
  }
}

check NewCodeExemption for 3
check TrivialDepthExemption for 3
check HighDepthModRequiresReview for 3
check BroadNontrivialRequiresReview for 3
check LowEverythingNeverRequires for 3
check MonotoneInDepthMod for 3
check TrustedContributorExemption for 3
check UnparsedAlwaysRequiresReview for 3
