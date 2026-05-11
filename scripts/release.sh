#!/usr/bin/env bash
# Cuts a new release tag and pushes it. The release workflow at
# .github/workflows/release.yml fires on `v*` tags and runs goreleaser,
# which creates a draft GitHub release and commits the brew formula to
# airbytehq/homebrew-tap.
#
# Usage: ./scripts/release.sh <major|minor|fix>
#
# "Current version" is derived from the latest semver tag in this repo.
# If no tags exist, it defaults to v0.0.0, so the first `fix` bump yields
# v0.0.1, the first `minor` yields v0.1.0, and the first `major` yields v1.0.0.
set -euo pipefail

usage() {
  echo "Usage: $0 <major|minor|fix>" >&2
  exit 1
}

[[ $# -eq 1 ]] || usage
bump=$1
case "$bump" in
  major|minor|fix) ;;
  *) usage ;;
esac

current=$(git tag --list 'v*' --sort=-v:refname | head -n1)
current=${current:-v0.0.0}

IFS=. read -r major minor patch <<<"${current#v}"
case "$bump" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  fix)   patch=$((patch + 1)) ;;
esac
next="v${major}.${minor}.${patch}"

branch=$(git rev-parse --abbrev-ref HEAD)
if [[ "$branch" != "main" ]]; then
  echo "Refusing to release from '$branch'. Switch to main first." >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Working tree is dirty. Commit or stash before releasing." >&2
  exit 1
fi

git fetch origin --tags --quiet
local_sha=$(git rev-parse HEAD)
remote_sha=$(git rev-parse origin/main)
if [[ "$local_sha" != "$remote_sha" ]]; then
  echo "main is out of sync with origin/main. Pull or push before releasing." >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/$next" >/dev/null; then
  echo "Tag $next already exists locally." >&2
  exit 1
fi

echo "Current: $current"
echo "Next:    $next"
read -r -p "Cut release $next? [y/N] " confirm
[[ "$confirm" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 0; }

git tag -a "$next" -m "Release $next"
git push origin "$next"

echo
echo "Tag pushed. The release workflow will:"
echo "  - build cross-platform archives"
echo "  - create a DRAFT release at https://github.com/airbytehq/airbyte-agents-cli/releases"
echo "  - commit Formula/airbyte-agents.rb to airbytehq/homebrew-tap"
echo
echo "Watch the run:    gh run watch --repo airbytehq/airbyte-agents-cli"
echo "Publish the draft once you've eyeballed the artifacts and the formula commit."
