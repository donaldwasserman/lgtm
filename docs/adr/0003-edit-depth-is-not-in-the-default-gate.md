# Edit depth is reported but not in the default gate

Edit depth counts levels of a language's syntax tree, so it measures the grammar more than the change. The same one-argument edit scored 6 in Ruby and 9 in Java, a one-token edit reached the old threshold of 7 in 69–97% of samples from this repository's Go code, and removing an authorization check scored lower than changing a log message. Cognitive complexity, significance and blast radius say what depth was meant to say, in units that mean the same thing in every language, so the threshold is off by default. The score is still reported and the threshold can be switched back on.

This reverses the earlier decision to freeze edit depth and file breadth at their first definitions. With depth out of the gate, the rule that only counted a wide change if it was also deep had nothing left to read, so breadth now counts only files with a substantive change, outside tests.

## Considered options

- **Lower or per-language thresholds.** Rejected: they would rescale a number that still ranks a log message above an authorization check.
- **Counting only control-flow nesting around the change.** Not adopted yet: the other measures cover the signal, and it would be one more number to tune without data.
