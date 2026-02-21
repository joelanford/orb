#!/usr/bin/env bash
set -euo pipefail

# Check for uncommitted changes using whichever VCS is active.
if [ -d .jj ]; then
    test -z "$(jj diff)"
else
    git diff --exit-code
fi
