#!/usr/bin/env bash
# Extract metadata.version from a SKILL.md YAML frontmatter block.
#
# Usage: scripts/skill-version.sh [path/to/SKILL.md]
#   Default path: skills/airbyte-agent/SKILL.md
#   Reads from stdin when the path is "-" or "/dev/stdin".
#
# Prints the version string (e.g. "v0.1.1") to stdout. Exits non-zero if
# the file or version field is missing — callers should treat that as a
# fatal build error rather than silently producing an empty ldflag.
set -euo pipefail

path="${1:-skills/airbyte-agent/SKILL.md}"

version=$(awk '
  /^---[[:space:]]*$/ {
    fm++
    if (fm == 2) exit
    next
  }
  fm != 1 { next }
  /^metadata:[[:space:]]*$/ { in_meta = 1; next }
  in_meta && /^[^[:space:]]/ { in_meta = 0 }
  in_meta && /^[[:space:]]+version:/ {
    sub(/^[[:space:]]+version:[[:space:]]*/, "")
    gsub(/^"|"$/, "")
    gsub(/^'\''|'\''$/, "")
    print
    exit
  }
' "$path")

if [ -z "$version" ]; then
  echo "skill-version: no metadata.version found in $path" >&2
  exit 1
fi

printf '%s\n' "$version"
