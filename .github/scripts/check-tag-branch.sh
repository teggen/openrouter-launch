#!/usr/bin/env bash
# Enforce the branch model: a stable tag (vX.Y.Z) must be reachable from the
# stable branch, a prerelease tag (vX.Y.Z-beta.N) from the prerelease branch.
#
# Git records no "branch this tag was pushed from" — reachability is the only
# thing that actually exists, so it is what this checks.
#
# The branch names are arguments rather than constants so tagguard_test.go can
# drive this against a throwaway repo with local branches.
#
# Usage: check-tag-branch.sh <tag> [stable-branch] [prerelease-branch]
set -euo pipefail

tag="${1:?usage: check-tag-branch.sh <tag> [stable-branch] [prerelease-branch]}"
stable_branch="${2:-origin/main}"
pre_branch="${3:-origin/develop}"

if [[ "$tag" != v* ]]; then
  echo "refusing: tag '$tag' does not start with 'v'" >&2
  exit 1
fi

# Any hyphen after the version core is a semver prerelease. This matches the
# same tags GoReleaser's `prerelease: auto` treats as prereleases, so the
# guard and the publisher cannot disagree.
if [[ "$tag" == *-* ]]; then
  want="$pre_branch"
  kind="prerelease"
else
  want="$stable_branch"
  kind="stable"
fi

if ! git rev-parse --verify --quiet "$want" >/dev/null; then
  echo "refusing: $kind tag '$tag' requires branch '$want', which does not exist" >&2
  exit 1
fi

if git merge-base --is-ancestor "$tag^{commit}" "$want"; then
  echo "ok: $kind tag '$tag' is reachable from '$want'"
  exit 0
fi

echo "refusing: $kind tag '$tag' is not reachable from '$want'." >&2
echo "  stable tags (vX.Y.Z) must be cut on ${stable_branch}." >&2
echo "  prerelease tags (vX.Y.Z-beta.N) must be cut on ${pre_branch}." >&2
exit 1
