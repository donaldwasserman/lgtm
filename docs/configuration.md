# Configuration

`lgtm` works with no configuration. To change how it decides, add a
`.lgtm.yml` at the root of your repository.

```yaml
preset: balanced          # or strict

thresholds:               # any measure: a number, or off
  lines: 500
  blast: off

exclude:                  # left out of the scan entirely
  - '**/*.pb.go'
  - '!build'              # remove a built-in entry

test-paths:               # not counted in files, modules or lines
  - 'e2e'

uncovered-ignore:         # may change without counting as uncovered
  - 'VERSION'

style-paths:              # stylesheets: never uncovered, but their size counts
  - '*.styl'

always-review:            # any change here needs review
  - 'src/auth'

overrides:                # stricter rules when a pull request touches a path
  - name: payments
    paths: ['services/payments/**']
    preset: strict
```

Every key is optional, and a misspelled key is an error.

**The file is read from the base commit** (where the branch split off), not from
the pull request. A pull request can't loosen the rules it's judged by, and a
change to `.lgtm.yml` itself needs review by default.

## Thresholds

| Key | Flag and action input | `balanced` | `strict` |
| --- | --- | --- | --- |
| `breadth` | `theta-breadth` | 25 | 8 |
| `modules` | `theta-modules` | 4 | 3 |
| `lines` | `theta-lines` | 7000 | 400 |
| `uncovered` | `theta-uncovered` | 1 | 1 |
| `cog` | `theta-cog` | 5 | 3 |
| `new-function` | `theta-new-function` | 25 | 15 |
| `significance` | `theta-significance` | `crucial` | `medium` |
| `blast` | `theta-blast` | 30 | 15 |
| `depth` | `theta-depth` | off | off |

The numbers are starting points, not measured optima. `strict` flags every
changed condition or error path, so expect it to be noisy on a busy codebase.

Settings apply in this order, later ones winning:

1. the preset
2. `thresholds` in `.lgtm.yml`
3. any `overrides` that match (these only tighten)
4. flags and action inputs

## Path lists

`exclude`, `test-paths`, `uncovered-ignore`, `style-paths` and `always-review` each start from
a built-in list. A pattern adds to the list; `!entry` removes that exact entry.
Removing an entry that isn't there is an error, so a typo can't go unnoticed.

Patterns are relative to the repository root. One without a slash (`*.pb.go`,
`testdata`) matches at any depth. One with a slash (`docs/gen`,
`src/**/*.min.js`) or a leading `/` is anchored at the root. `**` spans
directories, and a pattern matching a directory covers everything under it.

| List | Built-in entries |
| --- | --- |
| `exclude` | `.git` `node_modules` `vendor` `.venv` `venv` `__pycache__` `dist` `build` `target` `.tox` `.idea` `.vscode` |
| `test-paths` | `*_test.go` `test_*.py` `*_test.py` `conftest.py` `*_test.rb` `*_spec.rb` `*.test.*` `*.spec.*` `*Test.java` `*Tests.java` `__tests__` `test` `tests` `spec` `testdata` |
| `uncovered-ignore` | `*.md` `*.mdx` `*.rst` `*.txt` `docs` `LICENSE*` `NOTICE*` `AUTHORS*` `.gitignore` `.gitattributes` `.editorconfig` `*.png` `*.jpg` `*.jpeg` `*.gif` `*.svg` `*.ico` `*.webp` `*.css` `*.scss` `*.sass` `*.less` |
| `style-paths` | `*.css` `*.scss` `*.sass` `*.less` |
| `always-review` | `.github/workflows` `/.lgtm.yml` |

Excluded paths are invisible: they aren't parsed, counted, or matched by
`always-review`. They are still searched as text when deciding whether a deleted
private symbol is unreferenced, unless they come from a built-in entry.

## Overrides

An override applies when a pull request changes any of its `paths`, and it
applies to the whole pull request, because scores are per pull request. Each
threshold becomes the stricter of the current value and the override's, so an
override can never loosen the gate and can't be `off`. To relax rules for part
of a repository, `exclude` it or run a separate job.

The report's `overrides` field names the ones that applied.

## Recipes

### Start without blocking anyone

Add the Quick start workflow and don't make the check required. Watch it on real
pull requests for a week or two, adjust, then require it.

### Strict everywhere

```yaml
preset: strict
```

### Strict where mistakes are expensive

```yaml
overrides:
  - name: money
    paths: ['services/payments/**', 'services/billing/**']
    preset: strict
  - name: migrations
    paths: ['db/migrate']
    thresholds:
      lines: 100
```

### Certain paths always need a person

Use this for changes the measures can't judge, such as removing an authorization
check, which reads as a small condition edit.

```yaml
always-review:
  - 'src/auth'
  - 'db/migrate'
  - 'CODEOWNERS'
```

### Generated code

```yaml
exclude:
  - '*.pb.go'
  - '**/*.generated.ts'
  - 'src/**/*.min.js'
```

### Source code in a directory lgtm skips by default

Remove the entry that matches at any depth. Add it back at the root if the
top-level directory really is build output.

```yaml
exclude:
  - '!build'
  - '/build'
```

### Repositories with many config files

By default any changed file `lgtm` can't read needs review. List what's safe, or
switch the rule off and name what still matters.

```yaml
uncovered-ignore:
  - 'VERSION'
  - '*.lock'
  - 'package-lock.json'
  - 'go.sum'
```

```yaml
thresholds:
  uncovered: off
always-review:
  - 'terraform'
  - '*.sql'
  - 'Dockerfile'
```

### Stylesheets

`lgtm` can't read CSS, Sass or Less as code, but a stylesheet change rarely
carries logic risk. By default `*.css`, `*.scss`, `*.sass` and `*.less` are
**style paths**: they are never uncovered, and they count toward files and lines
added (non-blank lines the change adds; reordering and whitespace count for
nothing). They add nothing to modules, complexity, significance or blast radius.

Treat another format the same way:

```yaml
style-paths:
  - '*.styl'
```

To ignore stylesheets entirely, so they count for nothing, leave them out of
style paths (they stay on the `uncovered-ignore` list):

```yaml
style-paths:
  - '!*.css'
  - '!*.scss'
  - '!*.sass'
  - '!*.less'
```

To make them need review again, remove them from both lists:

```yaml
style-paths: ['!*.css']
uncovered-ignore: ['!*.css']
```

### Monorepo with separate teams

Run one job per area, excluding the others, each with its own check. Exclude
patterns are additive, so the built-in list still applies.

```yaml
- uses: donaldwasserman/lgtm@v0
  with:
    base-ref: origin/${{ github.event.pull_request.base.ref }}
    check-name: 'LGTM / api'
    exclude: |
      web
      mobile
```

### A rule the thresholds can't express

Turn off the check and decide from the report. This one requires review when a
condition changes in code that at least ten symbols depend on.

```yaml
- uses: donaldwasserman/lgtm@v0
  id: lgtm
  with:
    base-ref: origin/${{ github.event.pull_request.base.ref }}
    check-name: ''
- name: Risky logic changes need review
  env:
    REPORT: ${{ steps.lgtm.outputs.report }}
  run: |
    jq -e '(.scores.calledSignificance | IN("medium", "high", "crucial"))
           and .scores.blastRadius >= 10 | not' <<< "$REPORT"
```

The fields are documented in the [README](../README.md#result).

## Keeping the gate honest

On `pull_request`, GitHub runs the workflow from the pull request, so an author
can edit it. `lgtm` makes that visible: `.github/workflows` is an always-review
path, read from the base commit. It can't stop a workflow that removes the
action and posts a green check of the same name. To close that gap:

- Run the workflow as a [required workflow](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets#require-workflows-to-pass-before-merging)
  in a ruleset, so pull requests can't change it.
- Or require a code owner's review for `.github/workflows`.

`lgtm` looks only at code and reviews. It doesn't know who wrote a change, so it
has no exemption for particular authors.
