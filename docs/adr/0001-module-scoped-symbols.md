# Symbols are identified by module, not by file

A symbol's identity is its language module (Go package, Java fully qualified
class, Python module, Rust module path, ...) plus its enclosing declaration and
name — not the file that contains it. Callers bind to the module-level name, so
moving `func Parse` from `parse/a.go` to `parse/b.go` must not register as
deleting one exported symbol and adding another. Under file-scoped identity
that move would read as an export removal (crucial significance) and force
review of a change no caller can observe.

## Consequences

- The differ pairs declarations across files within a module before it diffs
  them; pairing base and head files by path alone is no longer enough.
- The same module scope is the first resolution tier when building the call
  graph for the affected set.
- Every supported language gets real module identity at launch, including
  the two where it is not obvious from one file: in Ruby a class or module
  reopened across files is one module, keyed by its constant path; in
  JavaScript the module is the file, and its exported names come from both
  ESM `export` and CommonJS `module.exports` / `exports.x`.
