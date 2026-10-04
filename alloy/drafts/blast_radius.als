-- DRAFT: affected set and blast radius as bounded reverse reachability over
-- the BASE call graph, k = 3 (fixed, not configurable). u -> v in calls means
-- "u calls (or references) v". Also the deletion exemption.
module blast_radius

abstract sig BOOL {}
one sig TRUE, FALSE extends BOOL {}

sig Symbol {
  exported: one BOOL,
  -- The symbol's name appears as plain text somewhere else in the repository.
  mentioned: one BOOL
}

sig Graph {
  nodes: set Symbol,
  calls: nodes -> nodes
}

-- Callers of S within one hop.
fun callers[g: Graph, S: set Symbol]: set Symbol { (g.calls).S }

-- Everything within k = 3 reverse hops of S, S included. Unrolled because the
-- Go side uses a bounded BFS, not full closure.
fun affected3[g: Graph, S: set Symbol]: set Symbol {
  let r1 = callers[g, S], r2 = callers[g, r1], r3 = callers[g, r2] |
    S + r1 + r2 + r3
}

-- The blast radius reported to users: affected symbols other than the changed
-- ones themselves.
fun radius3[g: Graph, S: set Symbol]: set Symbol { affected3[g, S] - S }

fun affectedAll[g: Graph, S: set Symbol]: set Symbol { S + (^(g.calls)).S }

-- The bounded radius never claims more than the unbounded one.
assert BoundedWithinClosure {
  all g: Graph, S: set g.nodes | affected3[g, S] in affectedAll[g, S]
}

-- A symbol nobody calls has an empty radius.
assert UncalledHasNoRadius {
  all g: Graph, s: g.nodes | no callers[g, s] implies no radius3[g, s]
}

-- Changing more never affects less.
assert AffectedMonotoneInChangeSet {
  all g: Graph, S1, S2: set g.nodes | S1 in S2 implies affected3[g, S1] in affected3[g, S2]
}

-- More edges never shrink the radius (base graph is an under-approximation,
-- so missing edges can only make us lenient, never strict).
assert MonotoneInEdges {
  all g1, g2: Graph, S: set Symbol |
    (g1.nodes = g2.nodes and g1.calls in g2.calls and S in g1.nodes)
    implies affected3[g1, S] in affected3[g2, S]
}

-- Symbols that exist only in head (new code) contribute nothing on base.
assert NewSymbolsHaveNoBaseRadius {
  all g: Graph, S: set Symbol | no (S & g.nodes) implies no radius3[g, S]
}

-- Expected to FAIL: the radius excluding S is not monotone in S, because
-- growing S can absorb a former caller. Kept to document why the Go code
-- reports |affected| - |S| per change rather than comparing radii.
assert RadiusMonotoneInChangeSet {
  all g: Graph, S1, S2: set g.nodes | S1 in S2 implies radius3[g, S1] in radius3[g, S2]
}

-- Deleting an existing symbol is exempt when it is not exported, the resolved
-- graph finds no caller, and its name is mentioned nowhere else.
pred deletionExempt[g: Graph, s: Symbol] {
  s.exported = FALSE and no callers[g, s] - s and s.mentioned = FALSE
}

-- The resolved graph is lenient: it may miss edges of the true graph. Every
-- true call names its callee in the caller's text, so a callee with a caller
-- other than itself is mentioned. Under that fact, the text check closes the
-- gap: an exempt deletion has no caller in the TRUE graph either.
assert ExemptionSoundUnderMissedCalls {
  all resolved, actual: Graph, s: resolved.nodes |
    (resolved.nodes = actual.nodes and resolved.calls in actual.calls and
     (all v: actual.nodes | some (callers[actual, v] - v) implies v.mentioned = TRUE) and
     deletionExempt[resolved, s])
    implies no callers[actual, s] - s
}

check ExemptionSoundUnderMissedCalls for 5 but 2 Graph
check BoundedWithinClosure for 5 but 2 Graph
check UncalledHasNoRadius for 5 but 2 Graph
check AffectedMonotoneInChangeSet for 5 but 2 Graph
check MonotoneInEdges for 5 but 2 Graph
check NewSymbolsHaveNoBaseRadius for 5 but 2 Graph
check RadiusMonotoneInChangeSet for 5 but 2 Graph
