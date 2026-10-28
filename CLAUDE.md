# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

envsecrets is a CLI tool for managing encrypted environment files using Google Cloud Storage (GCS) and age encryption. It provides a git-like push/pull workflow for secure team-wide access to environment configuration with full version history.

## Build & Development Commands

```bash
make build           # Compile binary to bin/envsecrets (includes version info via ldflags)
make install         # Build and install to ~/.local/bin
make test            # Run unit tests
make test-integration # Run integration tests (requires Docker)
make lint            # Run golangci-lint
make check           # Run lint + test together
make coverage        # Generate coverage.html report
make fmt             # Format code
make tidy            # Tidy go.mod
```

Run a single test:
```bash
go test -v ./internal/cache -run TestCacheName
```

### Docs

**Not part of `make check`.** The docs site has its own toolchain (uv/Python)
and its own CI workflows; `make lint` and `make test` do not cover it, and it
does not need to run for a change that touches neither. Run it for commits
touching `docs/`, `zensical.toml`, `pyproject.toml`, `uv.lock`, or either docs
workflow — both workflows trigger on those shared inputs (and each
additionally on its own file), because a dependency or lockfile change can
break the build just as easily as a content change:

```bash
make docs            # uv run --locked zensical build --strict
make docs-serve      # preview on http://127.0.0.1:7070
```

The site is [Zensical](https://zensical.org) (not MkDocs — migrated 2026-08),
configured in `zensical.toml`, built into `site-build/`. `--strict` fails on
broken links and anchors and is what both CI workflows run, so run it locally
before pushing docs changes. Note `zensical serve --strict` is unsupported;
verify strictness via `build`.

The look comes from the shared
[stridelabs-docs-theme](https://github.com/charliek/stridelabs-docs-theme)
package, pinned by tag in `pyproject.toml`. Palette, fonts and feature toggles
live there, not here — do not add `theme.palette`, `theme.features`, or a
`[project.theme.font]` table to `zensical.toml`. The last is the sharp edge: it
re-enables Zensical's Google Fonts `<link>` on every page while the theme's
self-hosted faces keep loading anyway.

Two gotchas worth knowing: Zensical **silently ignores unknown config keys**
even under `--strict`, so a green build does not prove a config edit did what
you meant; and the `pymdownx.emoji` callables live in the
`zensical.extensions.emoji` namespace — the Material for MkDocs
`material.extensions.emoji` namespace aborts the build.

## Architecture

### Package Structure

- **cmd/envsecrets/**: Entry point, wires up CLI; surfaces typed exit codes via `domain.GetExitCode`
- **internal/cli/**: Cobra-based commands (push, pull, status, sync, etc.)
- **internal/config/**: YAML config parsing (`~/.envsecrets/config.yaml`); includes optional `machine_id`
- **internal/crypto/**: Age + scrypt encryption (`AgeEncrypter` interface)
- **internal/storage/**: Cloud storage abstraction with GCS implementation
- **internal/cache/**: Local git-based cache for encrypted files (`~/.envsecrets/cache/owner/repo/`); reads and writes the `LAST_SYNCED` baseline, which lives per checkout (`UseCheckoutBaseline`); the legacy shared marker at `.git/.envsecrets-last-synced` is only read once, to migrate it
- **internal/sync/**: Push/pull/sync orchestration; `GetSyncStatus` does the 3-way classification (working tree vs `LAST_SYNCED` baseline vs remote HEAD) that drives `status` and `sync` recommendations and the push divergence safety check
- **internal/project/**: Project discovery (finds git root, parses `.envsecrets` file)
- **internal/git/**: Git operations via go-git library; commit author is stamped as `$USER@<machine_id>`, where `<machine_id>` defaults to the system `$hostname` and can be overridden via the `ENVSECRETS_MACHINE_ID` environment variable (or the `machine_id` config field, which the CLI exports into the env var on startup)
- **internal/domain/**: Domain types (`RepoInfo`, `Commit` with `AuthorEmail`/`AuthorDisplay`, `FileStatus`, `SyncStatus` with `Action` enum) and error handling
- **internal/ui/**: User output and prompts

### Key Data Flow

**Push**: Read `LAST_SYNCED` baseline → Sync remote into local cache → Divergence safety check (refuses with `ErrDivergedHistory` if remote moved AND files overlap, unless `--force`) → Encrypt local .env files → Commit (author = `$USER@<machine_id>`, where `<machine_id>` defaults to `$hostname` unless overridden) → Upload to GCS → Update `LAST_SYNCED` (or surface a `Warning` if marker write fails)

**Pull**: Download from GCS → Checkout commit → 3-way diff per file (overwrite stale tree, preserve local-only edits, conflict only when both sides changed the same file) → Decrypt → Write to project directory → Update `LAST_SYNCED` (only on full-HEAD pull, NOT for `pull --ref <hash>`)

**Status / Sync**: 3-way classify each tracked file → recommend one of `in_sync`/`push`/`pull`/`pull_then_push`/`reconcile`/`first_push_init`/`first_pull`/`restore` (`restore` when no tracked file exists locally but remote has them); `sync` executes the recommendation, never publishes remote deletions (push requires `AllowDeletes`), refusing with exit code 16 on reconcile

### Configuration

- Config file: `~/.envsecrets/config.yaml`
- Cache directory: `~/.envsecrets/cache/owner/repo/`
- Per-checkout baseline marker: `<project git dir>/envsecrets/owner/repo/last-synced` (never uploaded; worktrees get their own under `.git/worktrees/<name>/`). Legacy location `~/.envsecrets/cache/owner/repo/.git/.envsecrets-last-synced` is adopted by the first checkout that reads it
- Project tracking file: `.envsecrets` (lists files to track, one per line; supports `repo: owner/name` directive)

### Design Patterns

- **Interface-based**: `Storage`, `Repository`, `Encrypter` interfaces enable testing and swappable implementations
- **Dependency injection**: `NewProjectContext()` assembles all components
- **Local-first encryption**: Files encrypted before upload (server never sees plaintext)
- **Git-based versioning**: Cache is a git repo enabling offline work and version history

## Testing

- Unit tests: Standard `*_test.go` files
- Integration tests: Build-tagged with `integration`, require Docker
- Mocks: `internal/storage/mock.go`, `internal/git/mock.go`, `internal/crypto/mock.go`

## Linting

golangci-lint with: errcheck, staticcheck, unused, govet, ineffassign, gofmt, goimports, gosimple, misspell

Exemptions: `io.Closer.Close` errors ignored; errcheck/unused relaxed in test files.
