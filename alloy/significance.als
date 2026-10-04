-- significance level of a change to one existing symbol, after the
-- ChangeDistiller taxonomy (Fluri & Gall 2006). The level is the maximum over
-- the parts of the symbol that changed. Symbols are module-scoped
-- (docs/adr/0001-module-scoped-symbols.md), so moving a symbol between files
-- of one module is not a change at all and never reaches this model.
module significance

open util/integer

abstract sig BOOL {}
one sig TRUE, FALSE extends BOOL {}

abstract sig Part {}
one sig Rename,          -- alpha-rename of a local; data flow unchanged
        Statement,       -- statement inserted/deleted/updated in a body
        Condition,       -- predicate of if/loop/switch changed
        ErrorFlow,       -- catch/rescue/except, panic/recover, error return paths
        Signature,       -- params, results, receiver, type params
        Visibility,      -- modifiers / export status
        Supertypes,      -- extends/implements/embedded types/trait bounds
        Unclassified     -- changed, but the classifier could not place it
  extends Part {}

sig DeclChange {
  parts: some Part,
  exported_before: one BOOL,
  exported_after: one BOOL
}

-- Per-part level. Visibility and Signature depend on export status.
fun partLevel[c: DeclChange, p: Part]: Int {
  p = Rename     => 0 else
  p = Statement  => 1 else
  p = Condition  => 2 else
  p = ErrorFlow  => 2 else
  p = Supertypes => 4 else
  -- Lenient on precision, strict on coverage: never below medium.
  p = Unclassified => 2 else
  p = Signature  => (c.exported_before = TRUE => 3 else 2) else
  -- Visibility: removing an export is crucial; any other change is medium.
  (c.exported_before = TRUE and c.exported_after = FALSE) => 4 else 2
}

fun level[c: DeclChange]: Int {
  max[{ i: Int | some p: c.parts | i = partLevel[c, p] }]
}

assert BodyOnlyNeverAboveMedium {
  all c: DeclChange | c.parts in (Rename + Statement + Condition + ErrorFlow)
    implies lte[level[c], 2]
}

assert ExportedSignatureAtLeastHigh {
  all c: DeclChange | (Signature in c.parts and c.exported_before = TRUE)
    implies gte[level[c], 3]
}

assert ExportRemovalIsCrucial {
  all c: DeclChange | (c.exported_before = TRUE and c.exported_after = FALSE and
                       Visibility in c.parts)
    implies level[c] = 4
}

assert UnclassifiedAtLeastMedium {
  all c: DeclChange | Unclassified in c.parts implies gte[level[c], 2]
}

assert RenameOnlyIsNone {
  all c: DeclChange | c.parts = Rename implies level[c] = 0
}

-- Changing more parts of the same declaration never lowers its level.
assert MonotoneInParts {
  all a, b: DeclChange |
    (a.exported_before = b.exported_before and a.exported_after = b.exported_after and
     b.parts in a.parts)
    implies gte[level[a], level[b]]
}

check BodyOnlyNeverAboveMedium for 3 but 2 DeclChange
check ExportedSignatureAtLeastHigh for 3 but 2 DeclChange
check ExportRemovalIsCrucial for 3 but 2 DeclChange
check UnclassifiedAtLeastMedium for 3 but 2 DeclChange
check RenameOnlyIsNone for 3 but 2 DeclChange
check MonotoneInParts for 3 but 2 DeclChange
