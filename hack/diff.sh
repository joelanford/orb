#!/usr/bin/env bash
set -euo pipefail

# Runs the given make targets and verifies they produce no file changes.
# Usage: ./hack/diff.sh target1 target2 ...
# Works transparently with both git and jj.
if [ -d .jj ]; then
    orig=$(jj log --no-graph -r @ -T 'change_id' --quiet)
    jj new --quiet
    trap 'jj abandon --quiet; jj edit "$orig" --quiet' EXIT
    make "$@"
    diff=$(jj diff --summary)
    if [ -n "$diff" ]; then
        jj diff --git
        exit 1
    fi
else
    make "$@"
    git diff --exit-code
fi
