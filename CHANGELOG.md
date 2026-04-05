# Changelog

All notable changes to this project will be documented in this file.

## v0.0.4

- Fix stale local cache in multi-machine scenarios by fetching latest remote state before push/pull operations

## v0.0.3

- Add storage format versioning with FORMAT marker file in GCS to prevent future incompatibilities
- Every push writes FORMAT (currently "1"); pull/push reject missing or unsupported versions with clear errors
- Display storage format version in status, doctor, and verify commands
- Filter internal storage files (FORMAT, HEAD, objects.pack, refs) from list output
- Fix status display bug: files missing both locally and in cache now show "not synced" instead of "unchanged"
- Add exit code 15 for version incompatibility errors
- Upgrade GitHub Actions to Node.js 24 compatible versions

## v0.0.2

- Rework storage to use git packfiles and fix diff algorithm
- Add -v shorthand to log command and populate commit file changes
- Upgrade golangci-lint to v2 and manage via mise

## v0.0.1

- Initial release of envsecrets CLI tool
- Push/pull workflow for encrypted environment files using GCS and age encryption
- Git-based local cache with full version history
- Multi-file tracking via `.envsecrets` project file
- Status and diff commands for inspecting changes
- Release workflow and GoReleaser config for multi-platform builds
