#!/usr/bin/env bash
# Runs lgtm over a checkout's recent first-parent commits and summarizes the
# scores: each measure's distribution, how often each rule fires, and any
# commit lgtm could not evaluate. This is how to get data for tuning defaults.
#
# Usage: scripts/replay.sh <repo> [count] [lgtm flags...]
set -uo pipefail

repo="${1:?usage: replay.sh <repo> [count] [lgtm flags...]}"
count="${2:-200}"
shift $(( $# < 2 ? $# : 2 ))

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
lgtm="${LGTM_BIN:-$root/bin/lgtm}"
[ -x "$lgtm" ] || { echo "no lgtm binary at $lgtm; run make build or set LGTM_BIN" >&2; exit 2; }

reports="$(mktemp)"
trap 'rm -f "$reports"' EXIT
failed=0
for commit in $(git -C "$repo" rev-list --first-parent --max-count="$count" HEAD); do
  git -C "$repo" rev-parse --verify --quiet "$commit^" >/dev/null || continue
  if report="$("$lgtm" --repo "$repo" --base-ref "$commit^" --head-ref "$commit" --exit-zero "$@" 2>/dev/null)"; then
    jq -c --arg c "$commit" '. + {commit: $c}' <<< "$report" >> "$reports"
  else
    echo "lgtm failed on $commit" >&2
    failed=$((failed + 1))
  fi
done

jq -rs --argjson failed "$failed" '
  def rank: {"none":0,"low":1,"medium":2,"high":3,"crucial":4}[.];
  def pct($xs; $p): $xs[(($xs | length) - 1) * $p | floor];
  [ .[] | select(.files | length > 0) ] as $all
  | ($all | length) as $n
  | "\($n) commits with changes; lgtm failed on \($failed).",
    "",
    "measure                   p50   p90   max",
    ( ["breadthFiles","breadthModules","linesAdded","uncoveredFiles","cogDelta",
       "newFunctionComplexity","blastRadius","editDepth"][] as $m
      | [ $all[] | .scores[$m] // 0 ] | sort as $xs
      | if ($xs | length) == 0 then empty else
          "\($m + ("                         " | .[($m | length):]))\(pct($xs; 0.5))\t\(pct($xs; 0.9))\t\($xs[-1])"
        end ),
    "",
    "significance levels: \([ $all[] | .scores.significance // "none" ] | group_by(.) | map("\(.[0])=\(length)") | join(" "))",
    "",
    "share of commits flagged, by rule:",
    ( [ $all[] | .reasons[] ] | group_by(.) | map({r: .[0], c: length}) | sort_by(-.c)[]
      | "  \(.r): \(.c) (\((.c * 100 / $n) | floor)%)" ),
    "  any: \([ $all[] | select(.requiresReview) ] | length) (\((([ $all[] | select(.requiresReview) ] | length) * 100 / (if $n == 0 then 1 else $n end)) | floor)%)"
' "$reports"
