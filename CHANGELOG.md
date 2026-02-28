# Changelog

All notable changes to this project will be documented in this file.

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
