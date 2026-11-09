# The CLI reports scores; the gate is configured in the action

`lgtm` reports one score per measure and does not decide on its own whether a
pull request needs review. The gate — one threshold per measure, each of which
can be switched off — is configured through presets, a `.lgtm.yml` read from
the base commit, and the GitHub Action's inputs, with sensible defaults. The default gate is still evaluated in Go and tested
against the Alloy model, so the action passes its inputs to `lgtm` rather than
re-implementing the comparison in shell. The JSON scores are a versioned,
documented output, so a team can also gate on them in its own workflow step.

A small set of fixed rules sits outside the configurable thresholds and holds
for every configuration: an unparsed file always requires review, a change to
an always-review path always requires review, and raising a score never turns
review off. A measure not implemented for a language yields
an *unavailable score* instead, which the gate treats as that threshold being
off — otherwise adding a language would mean every pull request in it needs
review until each measure catches up. A file in a language lgtm does not parse
at all is different: it is an *uncovered file*, which requires review unless
the team ignores it.

## Considered options

- **A single composite risk score** (weighted sum, as in some defect-prediction
  literature). Rejected: the weights would be invented, and a single number
  cannot say which measure triggered review.
- **Gate in `action.yml` (bash/jq).** Rejected as the default: the generated
  Alloy tests reach only Go, so the decision would be the one unverified part.
- **User-written rule expressions.** Deferred: Alloy can verify only rules we
  ship, not ones users write. Planned for a later release.
