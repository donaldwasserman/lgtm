#!/usr/bin/env bash
# Exercises the image choice in the "Pull lgtm image" step of action.yml:
# a release pulls its own image, a pinned commit pulls that commit's image,
# anything else pulls "main", and an explicit image input always wins.
#
# The step's script is extracted from action.yml rather than copied, and runs
# against a stand-in docker that records what it was asked to pull.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

ruby -ryaml -e '
  d = YAML.load_file(ARGV[0])
  step = d["runs"]["steps"].find { |s| s["id"] == "image" }
  abort "no image step in action.yml" unless step
  print step["run"]
' "$root/action.yml" > "$work/step.sh" || exit 1

# docker: "image inspect" finds nothing locally; "pull" records the image.
mkdir -p "$work/tools"
cat > "$work/tools/docker" <<'SH'
#!/usr/bin/env bash
[ "$1" = "pull" ] && echo "$2" >> "$PULLED"
[ "$1" = "pull" ]
SH
chmod +x "$work/tools/docker"

# pick <ref> <VERSION contents> [image input] -> the image the step pulled
pick() {
  local ref="$1" version="$2" input="${3:-}"
  local dir="$work/_actions/donaldwasserman/lgtm/$ref"
  rm -rf "$work/_actions" && mkdir -p "$dir"
  echo "$version" > "$dir/VERSION"
  : > "$work/pulled"; : > "$work/output"
  env -i PATH="$work/tools:/usr/bin:/bin" PULLED="$work/pulled" \
      GITHUB_OUTPUT="$work/output" GITHUB_ACTION_PATH="$dir" IMAGE="$input" \
      bash "$work/step.sh" >/dev/null 2>&1 || { echo "step-failed"; return; }
  # The output the evaluate step reads must name the image that was pulled.
  if [ "$(sed -n 's/^ref=//p' "$work/output")" != "$(cat "$work/pulled")" ]; then
    echo "output-mismatch"; return
  fi
  cat "$work/pulled"
}

fail=0
check() {
  local name="$1" want="$2" got="$3"
  if [ "$got" = "$want" ]; then
    printf 'ok   %-36s %s\n' "$name" "$got"
  else
    printf 'FAIL %-36s want %s, got %s\n' "$name" "$want" "$got"
    fail=1
  fi
}

img=ghcr.io/donaldwasserman/lgtm
sha=0123456789abcdef0123456789abcdef01234567
check "exact release tag"             "$img:1.2.3"      "$(pick v1.2.3 1.2.3)"
check "major tag"                     "$img:1.2.3"      "$(pick v1 1.2.3)"
check "commit SHA"                    "$img:sha-0123456" "$(pick "$sha" 1.2.3)"
check "branch"                        "$img:main"       "$(pick main 1.2.3)"
check "tag that isn't this VERSION"   "$img:main"       "$(pick v2 1.2.3)"
check "uses: ./ (workspace path)"     "$img:main"       "$(pick lgtm 1.2.3)"
check "explicit image input wins"     "lgtm:pr"         "$(pick v1 1.2.3 lgtm:pr)"

exit "$fail"
