#!/usr/bin/env bash
# Exercises the two jq programs behind the trusted-contributor exemption in
# action.yml: the contributor ranking and the CODEOWNERS ownership match.
#
# Both programs are extracted from action.yml rather than copied, so this test
# cannot drift from what the action actually runs. They are the only place the
# trust policy lives - the Go binary receives just the resulting bool and the
# Alloy model treats it as opaque - so this suite is the policy's whole safety
# net.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ruby -ryaml -e '
  d = YAML.load_file(ARGV[0])
  step = d["runs"]["steps"].find { |s| s["id"] == "trust" }
  abort "no trust step in action.yml" unless step
  %w[rank_filter codeowners_filter].each do |name|
    m = step["run"].match(/#{name}='"'"'\n(.*?)\n\s*'"'"'\n/m)
    abort "could not extract #{name} from action.yml" unless m
    File.write(File.join(ARGV[1], "#{name}.jq"), m[1])
  end
' "$root/action.yml" "$work" || exit 1

fail=0
check() {
  local label="$1" want="$2" got="$3"
  if [ "$got" = "$want" ]; then
    printf 'ok   %-42s %s\n' "$label" "$got"
  else
    printf 'FAIL %-42s want: %s  got: %s\n' "$label" "$want" "$got"
    fail=1
  fi
}

# ---- contributor ranking ----

# rank <contributors-json> <author> <mode> <value> <jq-expression>
rank() {
  printf '%s' "$1" | jq -r --arg author "$2" --arg mode "$3" --argjson value "$4" \
    -f "$work/rank_filter.jq" | jq -r "$5"
}

# n contributors with descending, distinct commit counts.
people() {
  local n="$1" i out=""
  for ((i = 0; i < n; i++)); do
    out="$out{\"login\":\"u$i\",\"contributions\":$((100 - i)),\"type\":\"User\"},"
  done
  printf '[%s]' "${out%,}"
}

echo "-- ranking: percentile rounds up --"
check "n=0  p=20 -> cutoff"        0 "$(rank "$(people 0)" u0 top-percent 20 .cutoff)"
check "n=1  p=20 -> cutoff"        1 "$(rank "$(people 1)" u0 top-percent 20 .cutoff)"
check "n=3  p=20 -> cutoff"        1 "$(rank "$(people 3)" u0 top-percent 20 .cutoff)"
check "n=5  p=20 -> cutoff"        1 "$(rank "$(people 5)" u0 top-percent 20 .cutoff)"
check "n=6  p=20 -> cutoff"        2 "$(rank "$(people 6)" u0 top-percent 20 .cutoff)"
check "n=7  p=50 -> cutoff"        4 "$(rank "$(people 7)" u0 top-percent 50 .cutoff)"
check "n=10 p=0   -> cutoff"       0 "$(rank "$(people 10)" u0 top-percent 0 .cutoff)"
check "n=10 p=100 -> cutoff"      10 "$(rank "$(people 10)" u0 top-percent 100 .cutoff)"

echo "-- ranking: membership --"
check "rank 1 inside top 20%"      true  "$(rank "$(people 5)" u0 top-percent 20 .trusted)"
check "rank 2 outside top 20%"     false "$(rank "$(people 5)" u1 top-percent 20 .trusted)"
check "author absent -> rank 0"    0     "$(rank "$(people 5)" nobody top-percent 20 .rank)"
check "author absent -> untrusted" false "$(rank "$(people 5)" nobody top-percent 20 .trusted)"
check "login case-insensitive"     true  "$(rank "$(people 5)" U0 top-percent 20 .trusted)"

echo "-- ranking: top-count --"
check "top-count 2 -> cutoff"      2     "$(rank "$(people 5)" u0 top-count 2 .cutoff)"
check "top-count 2 includes #2"    true  "$(rank "$(people 5)" u1 top-count 2 .trusted)"
check "top-count 2 excludes #3"    false "$(rank "$(people 5)" u2 top-count 2 .trusted)"
check "top-count over n clamps"    5     "$(rank "$(people 5)" u0 top-count 99 .cutoff)"

echo "-- ranking: ties at the cutoff expand --"
TIES='[{"login":"a","contributions":10,"type":"User"},
       {"login":"b","contributions":5,"type":"User"},
       {"login":"c","contributions":5,"type":"User"},
       {"login":"d","contributions":5,"type":"User"},
       {"login":"e","contributions":1,"type":"User"}]'
check "k=2 expands over ties"      4     "$(rank "$TIES" a top-count 2 .cutoff)"
check "tied member is inside"      true  "$(rank "$TIES" d top-count 2 .trusted)"
check "below the ties is outside"  false "$(rank "$TIES" e top-count 2 .trusted)"

echo "-- ranking: bots are not contributors --"
BOTS='[{"login":"dependabot[bot]","contributions":999,"type":"Bot"},
       {"login":"a","contributions":10,"type":"User"},
       {"login":"b","contributions":5,"type":"User"}]'
check "bot excluded from n"        2     "$(rank "$BOTS" a top-count 1 .contributors)"
check "bot does not take the slot" true  "$(rank "$BOTS" a top-count 1 .trusted)"

# ---- CODEOWNERS ----

# owners <codeowners-text> <author> <paths-json> [teams-json] -> jq expression on the result
own() {
  local text="$1" author="$2" paths="$3" teams="${4:-}" expr="${5:-.trusted}"
  [ -z "$teams" ] && teams='{}'
  printf '%s\n' "$text" > "$work/CODEOWNERS"
  jq -n -r --arg author "$author" --argjson teams "$teams" \
    --rawfile text "$work/CODEOWNERS" --argjson paths "$paths" \
    -f "$work/codeowners_filter.jq" | jq -r "$expr"
}

echo "-- codeowners: last matching rule wins --"
check "later rule overrides earlier" false \
  "$(own '* @alice
*.go @bob' alice '["x.go"]')"
check "later rule owner is trusted"  true \
  "$(own '* @alice
*.go @bob' bob '["x.go"]')"
check "reversed order flips it"      true \
  "$(own '*.go @bob
* @alice' alice '["x.go"]')"
check "non-matching later rule kept" true \
  "$(own '* @alice
*.md @bob' alice '["x.go"]')"

echo "-- codeowners: glob semantics --"
check "* does not cross /"           false "$(own 'docs/* @alice' alice '["docs/a/b.md"]')"
check "docs/* matches direct child"  true  "$(own 'docs/* @alice' alice '["docs/a.md"]')"
check "basename at any depth"        true  "$(own '*.go @alice' alice '["internal/parse/scan.go"]')"
check "extension is not a prefix"    false "$(own '*.go @alice' alice '["scan.gox"]')"
check "trailing slash is a dir"      true  "$(own '/build/logs/ @alice' alice '["build/logs/x.go"]')"
check "dir pattern is root-anchored" false "$(own '/build/logs/ @alice' alice '["x/build/logs/x.go"]')"
check "** spans zero segments"       true  "$(own 'apps/**/test/ @alice' alice '["apps/test/x.go"]')"
check "** spans many segments"       true  "$(own 'apps/**/test/ @alice' alice '["apps/a/b/test/x.go"]')"
check "bare * owns everything"       true  "$(own '* @alice' alice '["any/deep/path.rb"]')"
check "anchored exact path"          true  "$(own '/eval/eval.go @alice' alice '["eval/eval.go"]')"

echo "-- codeowners: ownership of at least one changed file --"
check "owns one of several"          true \
  "$(own '/internal/ @alice' alice '["internal/a.go","eval/b.go","cmd/c.go"]')"
check "owns none"                    false \
  "$(own '/internal/ @alice' carol '["internal/a.go","eval/b.go"]')"
check "owned file list"              '["internal/a.go"]' \
  "$(own '/internal/ @alice' alice '["internal/a.go","eval/b.go"]' '{}' '.ownedFiles|tojson')"

echo "-- codeowners: parsing --"
check "comments and blanks ignored"  true \
  "$(own '# owners

*.go @alice   # trailing comment' alice '["x.go"]')"
check "multiple owners on a rule"    true  "$(own '*.go @bob @alice' alice '["x.go"]')"
check "owner-less rule owns nobody"  false "$(own '*.go' alice '["x.go"]')"
check "section headers skipped"      true  "$(own '[Docs]
*.go @alice' alice '["x.go"]')"
check "negation rule skipped"        false "$(own '!*.go @alice' alice '["x.go"]')"
check "author login case-insensitive" true "$(own '*.go @Alice' alice '["x.go"]')"

echo "-- codeowners: teams --"
check "team owner alone never matches" false \
  "$(own '*.go @acme/platform' bob '["x.go"]')"
check "expanded team member matches"   true \
  "$(own '*.go @acme/platform' bob '["x.go"]' '{"@acme/platform":["alice","bob"]}')"
check "non-member of expanded team"    false \
  "$(own '*.go @acme/platform' carol '["x.go"]' '{"@acme/platform":["alice","bob"]}')"

exit "$fail"
