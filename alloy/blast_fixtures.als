-- Small reference graphs with a changed set, each with the affected set and
-- the exempt deletions blast_radius.als assigns. `make generate` enumerates
-- instances of this run and cmd/genalloy turns them into
-- internal/blast/affected_alloy_test.go, so the Go traversal and exemption
-- cannot drift from the model.
module blast_fixtures

open blast_radius

one sig Case {
  graph: one Graph,
  changed: set Symbol,
  affected: set Symbol,
  exempt: set Symbol
}

fact {
  Case.graph.nodes = Symbol
  Case.changed in Case.graph.nodes
  Case.affected = affected3[Case.graph, Case.changed]
  Case.exempt = { s: Case.graph.nodes | deletionExempt[Case.graph, s] }
}

run fixtures {} for exactly 1 Graph, exactly 5 Symbol

-- Sparse graphs, where the affected set is a proper subset.
run sparse { #Case.graph.calls <= 4 } for exactly 1 Graph, exactly 5 Symbol

-- Some caller is reachable, but more than three hops away: the bound matters.
run beyondThreeHops {
  some s: Symbol | s in (^(Case.graph.calls)).(Case.changed) and s not in Case.affected
} for exactly 1 Graph, exactly 5 Symbol
