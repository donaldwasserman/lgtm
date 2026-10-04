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
A part of the gate that no configuration can switch off, such as "an unparsed
file always requires review".
_Avoid_: hard gate, override

## Measures

**Edit depth**:
How deep in the syntax tree, plus how long the call chain below, the deepest
change to existing code reaches.
_Avoid_: depth, impact depth

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
The affected set minus the changed symbols themselves. It can shrink when a
pull request changes more, which the affected set cannot.
_Avoid_: reach, ripple

**Significance level**:
How strongly a change to one existing symbol can affect code outside it:
none, low, medium, high or crucial.
_Avoid_: weight, risk weight
