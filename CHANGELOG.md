# Changelog

All notable changes to this project will be documented in this file.

## v0.0.7

- **Fix Homebrew formula test block**: the v0.0.6 formula called a non-existent `envsecrets version` subcommand (envsecrets uses the `--version` flag instead), causing `brew test` and `brew audit --strict` to fail. End-user `brew install` was unaffected. Updated `.goreleaser.yaml` so the regenerated formula uses `--version`.

## v0.0.6

- **Homebrew distribution**: `envsecrets` is now installable via `brew install charliek/tap/envsecrets` on macOS and Linux. Each tagged release auto-publishes a formula to [charliek/homebrew-tap](https://github.com/charliek/homebrew-tap) via GoReleaser, so `brew upgrade` will track new releases automatically.

## v0.0.5

- **Multi-machine sync clarity**: `envsecrets status` now ends with an unambiguous "do this next" recommendation (in_sync / push / pull / pull_then_push / reconcile / first_push_init / first_pull). Computed from a 3-way diff of working tree vs new `LAST_SYNCED` baseline vs remote HEAD.
- **New `envsecrets sync` command**: runs the recommended safe action automatically; refuses with exit code 16 on `reconcile` (does not auto-resolve overlapping conflicts).
- **Push divergence safety**: `push` refuses with `ErrDivergedHistory` (exit 4) when the remote moved since this machine's last sync AND files overlap. `--force` bypasses, preserving today's "publish working tree as-is" semantics. Closes a latent silent-overwrite path where a stale working tree could revert another machine's changes.
- **Pull is smarter about conflicts**: catch-up pulls (no local edits, just stale tree) no longer require `--force`. Local-only edits are preserved through `pull_then_push`. Real conflicts (same file, different content on both sides) still require resolution.
- **Per-commit machine attribution**: commits are now authored as `$USER@$hostname` (overridable via the `machine_id` config field or `ENVSECRETS_MACHINE_ID` env var) instead of the hardcoded `envsecrets@local`. `status` and `log` show "pushed by user@machine" in human output.
- **Typed exit codes now actually surface**: previously `main.go` collapsed all errors to exit 1, throwing away the typed exit code map. Fixed; CI scripts can now distinguish reconcile (16) from network failure (9) from auth issues, etc.
- **JSON status output enriched**: `--json` includes the `Sync status` recommendation, the three file-level lists (`local_changes`, `remote_changes`, `conflicts`), `last_synced`, `last_synced_at`, `remote_author`, and `remote_committed_at`.

Storage format unchanged — the `LAST_SYNCED` marker is local-only, never uploaded. Old clients on other machines keep working seamlessly; new clients gain the recommendations and safety on their own pushes.

- **Fix docs workflow**: pin `astral-sh/setup-uv` to `@v7` (the repo doesn't publish a floating `v8` tag, so the `Documentation` workflow had been failing since April 1).

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
