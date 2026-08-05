# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

- **Docs site migrated from Material for MkDocs to [Zensical](https://zensical.org)**, the successor from the same team. Material entered maintenance mode in November 2025 and now warns on every build that MkDocs 2.0 will remove the plugin and theming systems with no migration path. `mkdocs.yml` is replaced by a native `zensical.toml`; documentation content is unchanged. The look now comes from the shared [stridelabs-docs-theme](https://github.com/charliek/stridelabs-docs-theme) package rather than per-repo config, so restyling every site is a version bump instead of an edit in each repo; fonts are self-hosted by the theme, so the site no longer requests anything from `fonts.googleapis.com` or `fonts.gstatic.com`. Verified against the pre-migration build: identical 10-page set and all 115 heading anchors preserved, so existing deep links still resolve. The site also gains a project icon, an author meta tag, "edit this page" links, and a trailing slash on `site_url` (correct canonical form) — all previously absent.
- **`Docs PR Build` workflow added**: docs previously built only on push to `main`, and without `--strict`, so a broken link or heading anchor could land on `main` and be caught at deploy time or not at all. Both workflows now build `--strict` and watch `uv.lock`.

## v0.0.9

- **Release pipeline on the `cc-plugins:release-workflows` convention**: envsecrets is the fifth consumer of the framework (after strix, roost, prox, codelens). Both legacy PATs (`HOMEBREW_TAP_TOKEN`, `APT_DISPATCH_TOKEN`) are retired in favor of `charliek-release-bot` App tokens minted at workflow time, scoped to each target repo via `actions/create-github-app-token`'s `owner`+`repositories` inputs with `permission-contents: write` defense-in-depth. GoReleaser still uses `HOMEBREW_TAP_TOKEN` as its env-var name; the workflow now sets it from the App-minted token instead of from `secrets`. The dispatch retry loop now captures stderr so a real install-state failure (App not installed on a target, scope mismatch, etc.) is diagnosable from the workflow log. Same release flow as before — `/release-workflows:release vX.Y.Z` — but no per-pipeline PATs to rotate. (#11)
- **README .deb install snippet auto-resolves the latest tag**: the previous example hardcoded `VERSION=0.0.8`, which silently drifted on every release. Now uses `curl -fsS -o /dev/null -w '%{redirect_url}'` on the GitHub `releases/latest` redirect, so the snippet always installs the current version with no doc maintenance. (#11)
- **Branch protection ruleset on `main`** (created during the migration): `deletion` + `non_fast_forward` rules with the release-bot App (id `3902108`, `Integration`) and admin role (id `5`, `RepositoryRole`) in `bypass_actors`. Previously envsecrets had no branch protection at all. (#11)
- **`sanity-check-app.yml`**: new manual workflow that verifies the release-bot App can reach `homebrew-tap` and `apt-charliek` before a release tries to push to them. Catches App-install-on-target-repo mistakes early.
- **`RELEASING.md`**: per-repo release policy, break-glass recovery runbooks, secrets table, failure-mode table.

## v0.0.8

- **Linux apt distribution**: `envsecrets` is now installable via `sudo apt install envsecrets` from [charliek/apt-charliek](https://github.com/charliek/apt-charliek) (`apt.stridelabs.ai`) on Pop!_OS 24.04 / Ubuntu 24.04+ for `amd64` and `arm64`. Each tagged release builds `envsecrets_<version>_<arch>.deb` via GoReleaser's `nfpms:` block, attaches the artifacts to the GitHub Release, and fires `repository_dispatch` at apt-charliek so `apt update` picks up the new version within minutes. A `.deb` download path is also documented for one-off installs without configuring the apt repo.
- **CI release-snapshot job**: PRs now run `goreleaser release --snapshot` and validate both deb arches with `dpkg-deb`, catching broken nfpm config before it reaches a real tag.

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
