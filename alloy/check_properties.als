-- Assertions about the check-run layer in check_run.als.
--
-- The properties that matter here are safety properties about what the gate
-- can and cannot say: that it always says something on a pull request, that
-- it never reports green on an unevaluated change, and that green carries
-- exactly one of two meanings.
module check_properties

open gate
open check_run

-- The evaluation's analyzer outcome agrees with the gate's decision on its
-- scores and configuration. Failed is exempt: a crashed analyzer has no
-- verdict to agree with.
pred faithful[g: Evaluation] {
  g.outcome = Failed or (g.outcome = Required iff requiresReview[g.scores, g.config])
}

-- A required check that is never published blocks its pull request forever,
-- so every publishable evaluation must produce a conclusion.
assert AlwaysPublishesOnPullRequests {
  all g: Evaluation | publishable[g] implies conclusion[g] != NoCheck
}

-- A crashed analyzer must not read as approval to merge. This is why the
-- action uses failure rather than neutral: neutral counts as passing for
-- required checks.
assert AnalysisFailureFailsClosed {
  all g: Evaluation | (publishable[g] and g.outcome = Failed) implies conclusion[g] = Failure
}

-- An approval does not excuse a failed analysis: there is still no verdict
-- about the change.
assert ApprovalDoesNotExcuseFailure {
  all g: Evaluation | {
    (publishable[g] and g.outcome = Failed and hasApproval[g])
    implies conclusion[g] = Failure
  }
}

-- The docs' central claim: a green check means the change is below the
-- thresholds, or it is above them and someone has approved it. Nothing else
-- is ever green.
assert GreenMeansSimpleOrApproved {
  all g: Evaluation | {
    isGreen[g] implies
      (g.outcome = NotRequired or
       (g.outcome = Required and hasApproval[g] and not hasBlock[g]))
  }
}

-- One outstanding request for changes outranks any number of approvals.
assert ChangesRequestedOutranksApproval {
  all g: Evaluation | {
    (publishable[g] and g.outcome = Required and hasBlock[g])
    implies conclusion[g] = Failure
  }
}

-- The check clears itself: approving is sufficient to turn it green, with no
-- re-analysis and no change to the diff.
assert ApprovalClearsTheCheck {
  all g: Evaluation | {
    (publishable[g] and g.outcome = Required and
     hasApproval[g] and not hasBlock[g])
    implies isGreen[g]
  }
}

-- Commenting is inert. Two evaluations that agree on everything except
-- comment-only reviews reach the same state, which is what stops a comment
-- left after an approval from revoking it.
assert CommentsDoNotChangeTheVerdict {
  all g1, g2: Evaluation | {
    (g1.outcome = g2.outcome and
     g1.isPullRequest = g2.isPullRequest and
     g1.checkPublishable = g2.checkPublishable and
     { rv: g1.reviews | rv.verdict in verdictKinds }
       = { rv: g2.reviews | rv.verdict in verdictKinds })
    implies gateState[g1] = gateState[g2]
  }
}

-- Only a reviewer's most recent verdict counts; a superseded one never does.
assert LatestVerdictWins {
  all g: Evaluation, r: Reviewer, rv: verdicts[g, r] | {
    (some rv2: verdicts[g, r] | gt[int[rv2.revId], int[rv.revId]])
    implies latest[g, r] != rv
  }
}

-- Bridging to the gate: an unparseable tree can never go green on its own.
-- It can still go green once approved, which is the intended escape hatch.
assert UnparsedNeverGreenWithoutApproval {
  all g: Evaluation | {
    (publishable[g] and faithful[g] and
     g.scores.unparsed = TRUE and not hasApproval[g])
    implies not isGreen[g]
  }
}

-- Bridging to the gate: the trusted-contributor exemption survives the check
-- layer, so a trusted contributor's measured change is green with no reviews
-- at all.
assert TrustedContributorStaysGreen {
  all g: Evaluation | {
    (publishable[g] and faithful[g] and g.outcome != Failed and
     g.scores.trusted = TRUE and not unmeasured[g.scores])
    implies isGreen[g]
  }
}

-- Bridging to the gate: code lgtm failed to analyze can never go green on its
-- own either (fixed rule 4).
assert AnalysisFailureNeverGreenWithoutApproval {
  all g: Evaluation | {
    (publishable[g] and faithful[g] and
     g.scores.analysis_failed = TRUE and not hasApproval[g])
    implies not isGreen[g]
  }
}

check AnalysisFailureNeverGreenWithoutApproval for 4
check AlwaysPublishesOnPullRequests for 4
check AnalysisFailureFailsClosed for 4
check ApprovalDoesNotExcuseFailure for 4
check GreenMeansSimpleOrApproved for 4
check ChangesRequestedOutranksApproval for 4
check ApprovalClearsTheCheck for 4
check CommentsDoNotChangeTheVerdict for 4
check LatestVerdictWins for 4
check UnparsedNeverGreenWithoutApproval for 4
check TrustedContributorStaysGreen for 4
