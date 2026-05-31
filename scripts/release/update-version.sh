#!/usr/bin/env bash
# Bump envsecrets's release version.
#
# Envsecrets is a Go binary. Its version comes from a build-time ldflag
# injected by GoReleaser at tag time:
#
#   -X github.com/charliek/envsecrets/internal/version.Version={{.Version}}
#   -X github.com/charliek/envsecrets/internal/version.GitCommit={{.ShortCommit}}
#   -X github.com/charliek/envsecrets/internal/version.BuildDate={{.Date}}
#
# There's no source-tree manifest to bump — this script is intentionally
# a no-op, present only so the cc-plugins:release-workflows convention's
# contract holds (the release skill checks for and runs it, and the
# version-derivation expectation is documented for the next maintainer).
#
# Adapted from the cc-plugins:release-workflows convention's "Special
# case — Go modules" guidance in setup.md Phase 4.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <X.Y.Z>   e.g. $0 0.0.9" >&2
  exit 2
fi
V="$1"

if [[ ! "$V" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then
  echo "error: '$V' is not semver (X.Y.Z or X.Y.Z-suffix)" >&2
  exit 2
fi

echo "Go module: version (${V}), GitCommit, and BuildDate are injected" \
     "at build time from the tag via GoReleaser ldflags. " \
     "Nothing to bump in the source tree."
