# lgtm

lgtm measures how a pull request changes existing code, so a team can decide
which changes need a human review.

## Code under change

**Symbol**:
A named declaration — function, method, type, class, module — identified by
its language module, its enclosing declaration and its name. The file it
lives in is not part of its identity.
_Avoid_: definition, entity, node

**Existing symbol**:
A symbol present on the base side of the pull request.
_Avoid_: old code, prior code

**Unparsed file**:
A source file that could be read but not fully parsed on either side of the
pull request, syntax errors included.
_Avoid_: unreadable file, broken file

**Uncovered file**:
A changed file no measure can look at, because it is not in a supported
language, and that the team has not chosen to ignore.
_Avoid_: unsupported file, unknown file

**Substantive change**:
A change to a file that could alter behaviour: more than comments, formatting,
imports, renamed locals or code moved between files of one module.
_Avoid_: real change, meaningful change

## Measuring and gating

**Measure**:
One named aspect of a pull request that lgtm quantifies, such as blast radius
or edit depth.
_Avoid_: metric, signal, dimension

**Score**:
The value one measure takes for one pull request. lgtm reports one score per
measure and never combines them into a single number.
_Avoid_: composite score, risk index, metric

**Gate**:
The rule, configured by the team, that turns a pull request's scores into
"needs review" or not.
_Avoid_: verdict logic, policy

**Threshold**:
The score at or above which one measure makes the gate require review. A
threshold can be switched off, which is different from setting it to zero.
_Avoid_: limit, theta

**Unavailable score**:
A score lgtm does not produce because the measure is not implemented for a
file's language. Unlike an unparsed file, it does not require review; the gate
treats that measure's threshold as off for the file.
_Avoid_: missing score, unsupported, n/a

**Fixed rule**:
A part of the gate that no threshold can switch off, such as "an unparsed file
always requires review".
_Avoid_: hard gate

**Always-review path**:
A path the team has marked as needing review whenever a pull request changes it.
_Avoid_: protected path, sensitive path

**Preset**:
A named gate a team can start from.
_Avoid_: profile, policy

**Override**:
A set of stricter thresholds that applies to the whole pull request when it
changes a given path. An override can tighten the gate but never loosen it.
_Avoid_: exception, per-path gate

## Measures

**Edit depth**:
How deep in the syntax tree, plus how long the call chain below, the deepest
change to existing code reaches.
_Avoid_: depth, impact depth

**Files touched**:
The number of files that existed before the pull request, outside tests, with a
substantive change. New files and new symbols don't count: new code is judged by
lines added and new-function complexity.
_Avoid_: breadth, width

**Modules touched**:
The number of packages those files belong to.
_Avoid_: breadth, components

**Lines added**:
The number of non-blank lines a pull request adds to source files outside tests.
_Avoid_: size, churn, LOC

**Test lines removed**:
The number of non-blank lines in test files on the base side that no test file
on the head side still has, compared as trimmed text across all files. Moving a
test counts for nothing; deleting or rewriting one counts the lines that went.
_Avoid_: test churn, test coverage

**Nesting depth**:
How many syntactic levels a piece of code sits below the top of its file.
_Avoid_: depth, structural depth

**Call depth**:
The length of the longest chain of calls made from within a symbol.
_Avoid_: impact, call-chain depth

**Affected set**:
The changed existing symbols together with every symbol that calls them,
directly or through a bounded number of intermediate callers.
_Avoid_: impact set

**Blast radius**:
The affected set minus the changed symbols themselves, counting only symbols
whose change is at least medium significance (logic, not a statement or
constant edit). It can shrink when a pull request changes more, which the
affected set cannot.
_Avoid_: reach, ripple

**Significance level**:
How strongly a change to one existing symbol can affect code outside it:
none, low, medium, high or crucial.
_Avoid_: weight, risk weight

**Called significance**:
The highest significance level among changed existing symbols that something
calls. An exported signature change matters to the gate through it, so a
change nobody calls does not borrow the callers of another.
_Avoid_: compound significance

**Contributor**:
A symbol named in the report as one of those behind a score.
_Avoid_: offender, culprit
