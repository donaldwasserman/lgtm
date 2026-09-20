-- The check-run layer that action.yml publishes on top of the pr_review
-- decision. pr_review answers "does this change need a human?"; this module
-- answers "what does the pull request's status check say?", which is a
-- different question because it also depends on whether a human has since
-- shown up.
--
-- The state/conclusion mapping formalized here is the table in
-- WORKFLOW_LOGIC.md, and the precedence between states is the order of the
-- branches in the "Publish check run" step of action.yml.
module check_run

open util/integer
open pr_review

-- What the analyzer produced. Failed covers exit code 2 and any path that
-- leaves the action without a verdict at all, such as a failed build.
abstract sig Outcome {}
one sig NotRequired, Required, Failed extends Outcome {}

-- The states GitHub reports for a submitted review.
abstract sig ReviewState {}
one sig APPROVED, CHANGES_REQUESTED, DISMISSED, COMMENTED, PENDING extends ReviewState {}

-- Only these carry a verdict. COMMENTED and PENDING are dropped before the
-- per-reviewer reduction, which is what stops a comment left after an
-- approval from revoking it. DISMISSED is retained rather than dropped, so a
-- dismissed approval stops counting instead of being invisible.
fun verdictKinds: set ReviewState { APPROVED + CHANGES_REQUESTED + DISMISSED }

sig Reviewer {}

sig Review {
  by: one Reviewer,
  verdict: one ReviewState,
  -- Submission order, standing in for GitHub's monotonic review id.
  revId: one Int
}

-- Review ids are globally unique, so "the reviewer's most recent verdict" is
-- always well defined.
fact UniqueReviewOrder {
  all disj a, b: Review | int[a.revId] != int[b.revId]
  all r: Review | int[r.revId] >= 0
}

-- One evaluation of one pull request: what the analyzer said, what reviews
-- existed at that moment, and whether a check could be published at all.
sig Gate {
  outcome: one Outcome,
  reviews: set Review,
  -- FALSE outside a pull request (a push build, say).
  isPullRequest: one BOOL,
  -- FALSE when check-name is empty, or when the token cannot write checks,
  -- which is how fork pull requests behave.
  checkPublishable: one BOOL
}

-- The five titled states of WORKFLOW_LOGIC.md, plus the row where nothing is
-- published:
--   SNotRequired      "No review required"
--   SApproved         "Review required - approved"
--   SAwaiting         "Review required - awaiting approval"
--   SChangesRequested "Changes requested"
--   SFailed           "Analysis failed - could not evaluate"
--   SNotPublished     no check run at all
abstract sig State {}
one sig SNotRequired, SApproved, SAwaiting, SChangesRequested, SFailed, SNotPublished extends State {}

abstract sig Conclusion {}
one sig Success, Failure, NoCheck extends Conclusion {}

-- ---- the per-reviewer reduction ----

-- The verdict-carrying reviews r left on g.
fun verdicts[g: Gate, r: Reviewer]: set Review {
  { rv: g.reviews | rv.by = r and rv.verdict in verdictKinds }
}

-- r's most recent verdict on g, if r left one at all.
fun latest[g: Gate, r: Reviewer]: lone Review {
  { rv: verdicts[g, r] | no rv2: verdicts[g, r] | gt[int[rv2.revId], int[rv.revId]] }
}

pred approvedBy[g: Gate, r: Reviewer] { latest[g, r].verdict = APPROVED }
pred blockedBy[g: Gate, r: Reviewer]  { latest[g, r].verdict = CHANGES_REQUESTED }

pred hasApproval[g: Gate] { some r: Reviewer | approvedBy[g, r] }
pred hasBlock[g: Gate]    { some r: Reviewer | blockedBy[g, r] }

-- ---- the published state ----

pred publishable[g: Gate] {
  g.isPullRequest = TRUE and g.checkPublishable = TRUE
}

-- Branch order matches action.yml: an outstanding block outranks an approval
-- from someone else, and the analyzer's own failure outranks everything.
fun gateState[g: Gate]: one State {
  (not publishable[g]) => SNotPublished
  else (g.outcome = Failed) => SFailed
  else (g.outcome = NotRequired) => SNotRequired
  else hasBlock[g] => SChangesRequested
  else hasApproval[g] => SApproved
  else SAwaiting
}

-- The table's State -> Conclusion column.
fun conclusionOf[s: State]: one Conclusion {
  (s = SNotRequired) => Success
  else (s = SApproved) => Success
  else (s = SNotPublished) => NoCheck
  else Failure
}

fun conclusion[g: Gate]: one Conclusion { conclusionOf[gateState[g]] }

pred isGreen[g: Gate] { conclusion[g] = Success }
