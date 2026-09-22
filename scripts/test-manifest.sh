#!/usr/bin/env bash
# Checks action.yml for mistakes that only surface when the runner loads it.
#
# The runner evaluates ${{ }} everywhere in the manifest, description text
# included, and rejects the entire action when an expression uses a context
# that is unavailable there. A plain YAML parse accepts such a file, so this
# failure is otherwise invisible until CI.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

ruby -ryaml -e '
  d = YAML.load_file(ARGV[0])
  bad = (d["inputs"] || {}).merge(d["outputs"] || {})
          .select { |_, v| v["description"].to_s.include?("${{") }.keys
  if bad.empty?
    puts "ok   no expression syntax in input or output descriptions"
  else
    puts "FAIL expression syntax in descriptions of: #{bad.join(", ")}"
    exit 1
  end
' "$root/action.yml"
