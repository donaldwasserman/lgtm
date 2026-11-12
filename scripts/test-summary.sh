#!/usr/bin/env bash
# Renders a saved report through the two jq programs in the "Publish check
# run" step of action.yml: the summary and the annotations. Both are extracted
# from action.yml rather than copied, so this test cannot drift from what the
# action runs.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
report="$root/scripts/testdata/report.json"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ruby -ryaml -e '
  d = YAML.load_file(ARGV[0])
  step = d["runs"]["steps"].find { |s| s["id"] == "check" }
  abort "no check step in action.yml" unless step
  run = step["run"]
  s = run.match(/jq -r '"'"'(\s+def th:.*?)'"'"' "\$report" >> "\$summary"/m)
  a = run.match(/annotations=\$\(jq -c '"'"'(.*?)'"'"' "\$report"\)/m)
  abort "could not extract the jq programs from action.yml" unless s && a
  File.write(File.join(ARGV[1], "summary.jq"), s[1])
  File.write(File.join(ARGV[1], "annotations.jq"), a[1])
' "$root/action.yml" "$work" || exit 1

summary="$(jq -r -f "$work/summary.jq" "$report")" || { echo "FAIL summary program errored"; exit 1; }
annotations="$(jq -c -f "$work/annotations.jq" "$report")" || { echo "FAIL annotation program errored"; exit 1; }

fail=0
contains() {
  if printf '%s' "$summary" | grep -qF -- "$1"; then
    printf 'ok   summary has %s\n' "$1"
  else
    printf 'FAIL summary lacks %s\n' "$1"
    fail=1
  fi
}
annotation() { # <label> <jq expression over the array> <want>
  local got
  got="$(jq -r "$2" <<< "$annotations")"
  if [ "$got" = "$3" ]; then
    printf 'ok   %-36s %s\n' "$1" "$got"
  else
    printf 'FAIL %-36s want: %s  got: %s\n' "$1" "$3" "$got"
    fail=1
  fi
}

contains '| Lines added | 40 | 1000 |  |'
contains '| Uncovered files | 2 | 1 | **yes** |'
contains '| Test lines removed | 12 | 100 |  |'
contains '| Edit depth | 9 | off |  |'
contains 'Path overrides applied: payments.'
contains '`lib F` `a.go:12` (6)'
contains '`deploy.sql`'
contains '`lib`'

annotation 'cog delta on its line' '[.[] | select(.title == "lgtm: cogDelta") | "\(.path):\(.start_line)"] | join(",")' 'a.go:12'
annotation 'deleted symbol not annotated' '[.[] | select(.start_line == 90)] | length' '0'
annotation 'deleted file not annotated' '[.[] | select(.path == "old.sql")] | length' '0'
annotation 'uncovered file annotated' '[.[] | select(.path == "deploy.sql")] | length' '1'
annotation 'measure that did not fire is quiet' '[.[] | select(.title == "lgtm: editDepth")] | length' '0'

exit "$fail"
