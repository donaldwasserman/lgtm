#!/usr/bin/env bash
# Prints the body of the "## <version>" section of CHANGELOG.md.
# Used by the release workflow for release notes.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
awk -v v="$1" '
  index($0, "## " v) == 1 && (length($0) == length(v) + 3 || substr($0, length(v) + 4, 1) == " ") { on = 1; next }
  on && /^## / { exit }
  on { print }
' "$root/CHANGELOG.md" | sed -e '/./,$!d'
