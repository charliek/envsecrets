# Releasing envsecrets

The general release framework is `cc-plugins:release-workflows`; this file
documents what's specific to this repo.

## TL;DR

    /release-workflows:release v0.0.9

That's it. Everything else is automatic.

## What happens

1. **`release-workflows:release`** (LLM, local):
   - Verifies branch (`main`) + clean tree + CI green on HEAD
     (envsecrets has no `ci-success` aggregator; the skill reports
     "missing" for that check, which is expected — treat as non-blocking)
   - Asks/confirms version
   - Drafts a CHANGELOG entry from `git log v<previous>..HEAD`, commits as
     `docs(changelog): vX.Y.Z entry`
   - Runs `scripts/release/update-version.sh X.Y.Z` — **a no-op for
     envsecrets** because Go's version comes from a build-time ldflag, not
     a source-tree manifest. The script exists so the convention's
     contract holds.
   - Commits as `chore(version): bump to X.Y.Z` (the commit will be
     content-empty but tags reliably on a known commit)
   - Tags `vX.Y.Z` (annotated) on the version commit
   - `git push --follow-tags` (admin bypasses the ruleset)

2. **`release.yaml`** (CI, on tag push `v*`):
   - **`release`** (single job):
     - Checks out, sets up Go, runs `go test` + golangci-lint
     - Mints a release-bot App token scoped to `charliek/homebrew-tap`
     - Runs `goreleaser release --clean`:
       - Builds 4 targets (`linux/darwin` × `amd64/arm64`) with version,
         GitCommit, BuildDate injected via ldflags
       - Tarballs as `envsecrets_<os>_<arch>.tar.gz`
       - Builds .debs (amd64 + arm64) via `nfpms:`
       - Uploads tarballs + .debs + `checksums.txt` to the GitHub Release
       - Pushes `Formula/envsecrets.rb` to `charliek/homebrew-tap` using
         the App-minted token (replaces the legacy `HOMEBREW_TAP_TOKEN`)
     - Mints a release-bot App token scoped to `charliek/apt-charliek`
     - Dispatches `repository_dispatch` (`event_type=publish`) at
       `charliek/apt-charliek` (replaces the legacy `APT_DISPATCH_TOKEN`)

The maintainer runs step 1; everything else is automated.

## Version files this repo owns

`scripts/release/update-version.sh` bumps nothing. Envsecrets's canonical
version is the git tag, injected at build time via:

```
-X github.com/charliek/envsecrets/internal/version.Version={{.Version}}
-X github.com/charliek/envsecrets/internal/version.GitCommit={{.ShortCommit}}
-X github.com/charliek/envsecrets/internal/version.BuildDate={{.Date}}
```

`CHANGELOG.md` is maintained by the release skill for human-readable
in-repo history; GoReleaser's auto-generated release notes go on the
GitHub Release body separately.

`pyproject.toml` is for mkdocs only; has its own version cadence.

## Snapshot / dev versioning

Not used. `envsecrets --version` between releases shows the last
released version.

## Secrets

| Secret | Purpose | Required? |
|---|---|---|
| `RELEASE_BOT_APP_ID` | `charliek-release-bot` GitHub App ID | required — for homebrew-tap push + apt-charliek dispatch |
| `RELEASE_BOT_APP_KEY` | App private key (.pem) | required — same |

Retired (deleted from `gh secret list -R charliek/envsecrets` during the
convention adoption — confirm `gh secret list -R charliek/envsecrets`
returns only the `RELEASE_BOT_APP_*` pair, not just removed from the
workflow):

- `APT_DISPATCH_TOKEN` — replaced by the App-minted apt-charliek token
- `HOMEBREW_TAP_TOKEN` — replaced by the App-minted homebrew-tap token

GoReleaser still reads the env var named `HOMEBREW_TAP_TOKEN`; the
workflow sets it from `steps.tap.outputs.token` instead of `secrets`.

## Branch protection

`main` is protected by a ruleset (created during the convention
adoption) with rules `deletion` + `non_fast_forward` (no
`required_status_checks` — envsecrets's `ci.yml` runs separate jobs
with no aggregator). Bypass actors:

- `charliek-release-bot` (App, type `Integration`)
- Admin role (id `5`, type `RepositoryRole`)

Inspect or edit at https://github.com/charliek/envsecrets/rules.

## App installation

The release-bot App must be installed on three repos:

- `charliek/envsecrets`
- `charliek/homebrew-tap`
- `charliek/apt-charliek`

Verify all three via `sanity-check-app.yml` (Actions → Run workflow).

## When things break

| Symptom | Cause | Fix |
|---|---|---|
| `git push` rejected | Pusher not in ruleset bypass | Add the App or admin to `bypass_actors` |
| GoReleaser fails at `brews` with `Bad credentials` | App not installed on `homebrew-tap` | Confirm via sanity-check; install the App on the tap |
| `Trigger apt-charliek publish` fails | App not installed on `apt-charliek` | Confirm via sanity-check; install if missing. Step retries 3× with stderr captured. |
| `release` job's `go test` fails on the tag | Real test failure | Fix on branch, merge, cut a fresh patch tag |
| `brew install` finds old version | Tap cache | `brew untap charliek/tap && brew tap charliek/tap` |

## Break-glass recovery

### GoReleaser failed after some artifacts uploaded

```bash
RUN_ID=$(gh run list -R charliek/envsecrets --workflow release.yaml \
                     --limit 1 --json databaseId --jq '.[0].databaseId')
gh run rerun "${RUN_ID}" -R charliek/envsecrets --failed
```

GoReleaser's `mode: replace` reuses the existing Release.

## Adopting the convention (for new contributors)

If you're new to this repo and need to understand the release pipeline,
read [`cc-plugins/plugins/release-workflows/references/convention.md`](https://github.com/charliek/cc-plugins/blob/main/plugins/release-workflows/references/convention.md)
in the framework repo.

## Notes for this repo

- **No `version-check` step**: envsecrets has no source-tree version
  manifest (Go binary's version is ldflag-injected at build time), so
  there's nothing for a tag-vs-manifest check to assert.
- **No `ci-gate` job**: envsecrets's `ci.yml` has 4 separate jobs with
  no aggregator. The `release` job's inline `go test` + lint serve as
  the inline gate at tag time.
- **No `sync-version` job**: never existed for envsecrets (no source-
  tree version manifest to sync), so the local-bump-trumps-CI migration
  is a no-op for this repo. Simpler than prox/codelens.
