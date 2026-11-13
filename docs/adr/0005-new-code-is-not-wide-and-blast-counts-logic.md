# New code is not wide, blast radius counts logic, and test deletions are gated

The gate exists so that most pull requests need no person. Two measures were
working against that, and one gap let a cheap, risky change through.

**Breadth counted new files.** A feature made of a form, an API client, a
handler, business logic and a store method adds files in four or five
packages, so it tripped files or modules touched even though nothing that
existed changed. Breadth now counts only files whose existing code changed
substantively. New code is still judged, by lines added and new-function
complexity, and the small edits that wire it in still count as touching those
files. The `NewCodeExemption` property in `alloy/gate_properties.als` now
reads "pure additions pass under the line limit", not "narrow pure additions".

**Blast radius counted every change.** A new constant value or an extra
argument to a log call in a function with forty callers cost as much as
removing a check from it. That made blast radius noisy, so the configuration
that prompted this switched it off, which also stopped it catching the risky
case. Blast radius now counts only callers of changes of at least medium
significance: conditions, error handling, a signature change or a removed
export. `calledSignificance` still reads every change, so the compound rule
for exported signatures is unchanged. `alloy/gate.als` records the
consequence: a positive blast radius implies a called change of at least
medium.

**Deleting tests was free.** Tests are left out of files, modules and lines
added, so a pull request could delete a failing test, or a whole suite, and
pass. The new score, test lines removed, counts the non-blank lines that leave
test files and fixtures and appear in no test file on the head side, compared
as trimmed text across all files. Moving or renaming a test costs nothing, a
rewrite costs the lines that went, and deleting tests en masse reaches the
threshold (100 lines in `balanced`, 50 in `strict`). It reads raw text, so it
works the same in every language, including fixtures lgtm doesn't parse.

## Consequences

- A statement-level edit in a widely called function, such as removing a call
  to an audit or authorization helper, has no blast radius. Paths where that
  matters belong on `always-review` or under an override with
  `significance: low`.
- A pull request made only of new code can span any number of files and
  packages. Lines added (1000) is the bound.
- Replacing a test file wholesale with different tests counts the old lines as
  removed. That is the intended trade: lgtm can't tell a better test from a
  weaker one.

## Considered options

- **A configurable significance floor for blast radius.** Not adopted: one
  more knob to tune without data, and medium is the line the significance
  model already draws between data and logic.
- **Counting deleted test functions instead of lines.** Rejected: test cases
  in JavaScript, TypeScript and RSpec are calls (`it(...)`), not declarations,
  so a symbol count would miss them.
- **Net lines removed per file.** Rejected: moving a test between files would
  count as a deletion.
