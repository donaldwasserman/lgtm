-- One satisfiable scenario per row of WORKFLOW_LOGIC.md, plus the review
-- reductions that the table's "latest review" wording leaves implicit.
--
-- These double as non-vacuity witnesses for check_properties.als: an
-- assertion there that held only because its antecedent was unsatisfiable
-- would show up here as an UNSAT scenario.
module check_scenarios

open util/integer
open check_run

-- Row 1: below the thresholds. Green with no reviews at all.
run scenario_gate_noReviewRequired {
  all g: Gate | {
    g.outcome = NotRequired
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    no g.reviews
    gateState[g] = SNotRequired
    conclusion[g] = Success
  }
} for 3 but exactly 1 Gate, exactly 0 Review, exactly 0 Reviewer

-- Row 2: above the thresholds, but approved. Green because a human vouched
-- for it, not because the change is simple.
run scenario_gate_requiredApproved {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    Review.verdict = APPROVED
    gateState[g] = SApproved
    conclusion[g] = Success
  }
} for 3 but exactly 1 Gate, exactly 1 Review, exactly 1 Reviewer

-- Row 3: above the thresholds, nobody has looked yet.
run scenario_gate_requiredAwaiting {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    no g.reviews
    gateState[g] = SAwaiting
    conclusion[g] = Failure
  }
} for 3 but exactly 1 Gate, exactly 0 Review, exactly 0 Reviewer

-- Row 4: a reviewer has blocked it.
run scenario_gate_changesRequested {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    Review.verdict = CHANGES_REQUESTED
    gateState[g] = SChangesRequested
    conclusion[g] = Failure
  }
} for 3 but exactly 1 Gate, exactly 1 Review, exactly 1 Reviewer

-- Row 5: exit code 2. No verdict about the change, so the gate fails closed.
run scenario_gate_analysisFailed {
  all g: Gate | {
    g.outcome = Failed
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    no g.reviews
    gateState[g] = SFailed
    conclusion[g] = Failure
  }
} for 3 but exactly 1 Gate, exactly 0 Review, exactly 0 Reviewer

-- Row 6: not a pull request. Nothing is published at all.
run scenario_gate_notPullRequest {
  all g: Gate | {
    g.isPullRequest = FALSE
    gateState[g] = SNotPublished
    conclusion[g] = NoCheck
  }
} for 3 but exactly 1 Gate, exactly 0 Review, exactly 0 Reviewer

-- A fork pull request, or a workflow without checks: write. The action warns
-- and continues, so the pull request simply has no check.
run scenario_gate_cannotPublish {
  all g: Gate | {
    g.isPullRequest = TRUE and g.checkPublishable = FALSE
    g.outcome = Required
    gateState[g] = SNotPublished
    conclusion[g] = NoCheck
  }
} for 3 but exactly 1 Gate, exactly 0 Review, exactly 0 Reviewer

-- An approval from one reviewer does not clear another reviewer's block.
run scenario_gate_blockOutranksApproval {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    some a, b: Review | {
      a != b
      a.verdict = APPROVED
      b.verdict = CHANGES_REQUESTED
      a.by != b.by
    }
    gateState[g] = SChangesRequested
  }
} for 3 but exactly 1 Gate, exactly 2 Review, exactly 2 Reviewer

-- Commenting after approving must not revoke the approval: COMMENTED never
-- enters the per-reviewer reduction.
run scenario_gate_commentAfterApproval {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    some a, c: Review | {
      a.verdict = APPROVED
      c.verdict = COMMENTED
      gt[int[c.revId], int[a.revId]]
    }
    gateState[g] = SApproved
  }
} for 3 but exactly 1 Gate, exactly 2 Review, exactly 1 Reviewer

-- The same reviewer approving and then requesting changes: the later verdict
-- is the one that counts.
run scenario_gate_approvalThenChanges {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    some a, b: Review | {
      a.verdict = APPROVED
      b.verdict = CHANGES_REQUESTED
      gt[int[b.revId], int[a.revId]]
    }
    gateState[g] = SChangesRequested
  }
} for 3 but exactly 1 Gate, exactly 2 Review, exactly 1 Reviewer

-- Requesting changes and then approving: the approval is the later verdict,
-- so the gate clears.
run scenario_gate_changesThenApproval {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    some a, b: Review | {
      b.verdict = CHANGES_REQUESTED
      a.verdict = APPROVED
      gt[int[a.revId], int[b.revId]]
    }
    gateState[g] = SApproved
  }
} for 3 but exactly 1 Gate, exactly 2 Review, exactly 1 Reviewer

-- A dismissed approval stops counting, leaving the gate waiting again.
run scenario_gate_dismissedApproval {
  all g: Gate | {
    g.outcome = Required
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    Review.verdict = DISMISSED
    gateState[g] = SAwaiting
  }
} for 3 but exactly 1 Gate, exactly 1 Review, exactly 1 Reviewer

-- An approval does not excuse a failed analysis: the gate still has no
-- verdict about the change.
run scenario_gate_failedOutranksApproval {
  all g: Gate | {
    g.outcome = Failed
    g.isPullRequest = TRUE and g.checkPublishable = TRUE
    g.reviews = Review
    Review.verdict = APPROVED
    gateState[g] = SFailed
    conclusion[g] = Failure
  }
} for 3 but exactly 1 Gate, exactly 1 Review, exactly 1 Reviewer
