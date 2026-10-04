-- Every combination of changed parts and export status, each with the level
-- significance.als assigns. `make generate` enumerates all instances of this
-- run and cmd/genalloy turns them into internal/significance/levels_alloy_test.go,
-- so the Go classifier's level table cannot drift from the model.
module significance_fixtures

open util/integer
open significance

one sig Expected {
  change: one DeclChange,
  lvl: one Int
}

fact {
  Expected.lvl = level[Expected.change]
}

run levels {} for exactly 1 DeclChange, 4 Int
