#!/usr/bin/env bash
# Fail when gosec produced a report without actually analysing the tree.
#
# WHY THIS EXISTS. `gosec -no-fail` exits 0 unconditionally — that is the
# point of the flag, and it is what keeps this repo's ~19 genuine findings
# advisory instead of blocking. But it also swallows the case where gosec
# never analysed anything. Measured on a deliberately broken package:
#
#   $ gosec -no-fail -fmt sarif -out out.sarif ./...
#   [gosec] Error building the SSA representation of the package main:
#           package main has type errors, skipping SSA analysis, no ssa result
#   $ echo $?
#   0
#   $ jq '.runs[0].results' out.sarif
#   []
#
# A structurally valid SARIF *file* is written, so every downstream control
# in ci.yml's audit job is satisfied — including `if-no-files-found: error`,
# the one deliberately-unsoftened link, which only ever asked whether a file
# exists. Task 5 closed "no file at all"; this closes "a file from an
# analysis that did not run".
#
# WHAT IT MUST NOT DO. Genuine findings must stay non-blocking. This script
# therefore never looks at how many results the SARIF holds: a tree with 19
# findings and a tree with 0 findings are both fine, and the empty SARIF
# above is rejected on the LOG's evidence, not on its own emptiness. Getting
# that backwards would either make every gosec finding fail the build or
# make a genuinely clean tree impossible to achieve.
#
# The log and report paths are arguments rather than constants so
# gosecguard_test.go can drive this against fixture files.
#
# Usage: check-gosec-analysis.sh <gosec-log> <sarif-file>
set -euo pipefail

log="${1:?usage: check-gosec-analysis.sh <gosec-log> <sarif-file>}"
sarif="${2:?usage: check-gosec-analysis.sh <gosec-log> <sarif-file>}"

status=0

if [[ ! -s "$log" ]]; then
  echo "refusing: gosec wrote no log output to '$log' — it cannot have run" >&2
  status=1
fi

# The signature of an analysis that bailed out. gosec logs this and keeps
# going, so it is the only trace left by the time the SARIF is written.
failure_re='no ssa result|has type errors|Error building the SSA representation|Golang errors in file|failed to load package'
if [[ -s "$log" ]] && grep -qiE "$failure_re" "$log"; then
  echo "refusing: gosec reported a failed analysis — the SARIF below it is not evidence of anything." >&2
  echo "  This is a compile/type error in the tree, not a security finding. Fix the build, then re-run." >&2
  grep -iE "$failure_re" "$log" | sed 's/^/  /' >&2
  status=1
fi

# gosec logs exactly one "Checking file:" line per file it reads. Zero of
# them means it walked no packages at all — the literal "went green having
# analysed nothing" case.
#
# This does key on gosec's log wording, and GOSEC_VERSION floats at `latest`,
# so a future rewording would fail this closed (red CI on a sound tree) with
# the message right here to explain it. That is the correct direction to
# fail; do not "fix" it by deleting the check.
if [[ -s "$log" ]] && ! grep -q 'Checking file:' "$log"; then
  echo "refusing: gosec logged no 'Checking file:' lines — it analysed zero files." >&2
  echo "  Either no packages matched, or gosec's log format changed (see this script's comment)." >&2
  status=1
fi

if [[ ! -s "$sarif" ]]; then
  echo "refusing: no SARIF report at '$sarif'" >&2
  status=1
fi

if [[ "$status" -eq 0 ]]; then
  echo "ok: gosec analysed $(grep -c 'Checking file:' "$log") files and wrote '$sarif'"
fi

exit "$status"
