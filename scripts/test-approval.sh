#!/usr/bin/env bash
# Exercises the approval reduction used by the "Check for a qualifying
# approval" step in action.yml against saved review payloads.
#
# The jq program is extracted from action.yml rather than copied, so this test
# cannot drift from what the action actually runs. The approve-flips-the-check
# transition itself is not reachable locally (GitHub blocks approving your own
# pull request), so this is the coverage that stands in for it.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
data="$root/scripts/testdata"

filter="$(ruby -ryaml -e '
  d = YAML.load_file(ARGV[0])
  step = d["runs"]["steps"].find { |s| s["id"] == "approval" }
  abort "no approval step in action.yml" unless step
  m = step["run"].match(/states=\$\(jq -r '"'"'(.*?)'"'"' "\$reviews"\)/m)
  abort "could not extract the jq program from action.yml" unless m
  print m[1]
' "$root/action.yml")" || exit 1

# Mirrors the action's interpretation of the reduced per-author states.
verdict() {
  local approved=false changes_requested=false state
  for state in $(jq -r "$filter" "$1"); do
    case "$state" in
      APPROVED) approved=true ;;
      CHANGES_REQUESTED) changes_requested=true ;;
    esac
  done
  echo "approved=$approved changes_requested=$changes_requested"
}

fail=0
expect() {
  local fixture="$1" want="$2" got
  got="$(verdict "$data/$fixture")"
  if [ "$got" = "$want" ]; then
    printf 'ok   %-32s %s\n' "$fixture" "$got"
  else
    printf 'FAIL %-32s want: %s  got: %s\n' "$fixture" "$want" "$got"
    fail=1
  fi
}

expect empty.json                  'approved=false changes_requested=false'
expect single-approval.json        'approved=true changes_requested=false'
expect approved-then-changes.json  'approved=false changes_requested=true'
expect changes-then-approved.json  'approved=true changes_requested=false'
expect approval-dismissed.json     'approved=false changes_requested=false'
expect comments-only.json          'approved=false changes_requested=false'
# Commenting after approving must not revoke the approval.
expect approved-then-commented.json 'approved=true changes_requested=false'
# One outstanding block outranks an approval from someone else.
expect split-verdict.json          'approved=true changes_requested=true'
expect pending.json                'approved=false changes_requested=false'

exit "$fail"
