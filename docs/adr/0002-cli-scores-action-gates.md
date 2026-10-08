# The CLI reports scores; the gate is configured in the action

`lgtm` reports one score per measure and does not decide on its own whether a
pull request needs review. The gate — one threshold per measure, each of which
can be switched off — is configured through the GitHub Action's inputs, which
ships sensible defaults. The default gate is still evaluated in Go and tested
against the Alloy model, so the action passes its inputs to `lgtm` rather than
re-implementing the comparison in shell. The JSON scores are a versioned,
documented output, so a team can also gate on them in its own workflow step.

A small set of fixed rules sits outside the configurable thresholds and holds
for every configuration: an unparsed file always requires review; a trusted
author never skips review of an unparsed file; raising a score never turns
review off; and code in a language a measure supports, but which lgtm fails
to analyze, requires review. A measure not implemented for a language yields
an *unavailable score* instead, which the gate treats as that threshold being
off — otherwise adding a language would mean every pull request in it needs
review until each measure catches up.

## Considered options

- **A single composite risk score** (weighted sum, as in some defect-prediction
  literature). Rejected: the weights would be invented, and a single number
  cannot say which measure triggered review.
- **Gate in `action.yml` (bash/jq).** Rejected as the default: the generated
  Alloy tests reach only Go, so the decision would be the one unverified part.
- **User-written rule expressions.** Deferred: Alloy can verify only rules we
  ship, not ones users write. Planned for a later release.
