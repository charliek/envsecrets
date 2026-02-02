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

## Architecture

### Package Structure

- **cmd/envsecrets/**: Entry point, wires up CLI
- **internal/cli/**: Cobra-based commands (push, pull, status, etc.)
- **internal/config/**: YAML config parsing (`~/.envsecrets/config.yaml`)
- **internal/crypto/**: Age + scrypt encryption (`AgeEncrypter` interface)
- **internal/storage/**: Cloud storage abstraction with GCS implementation
- **internal/cache/**: Local git-based cache for encrypted files (`~/.envsecrets/cache/owner/repo/`)
- **internal/sync/**: Push/pull orchestration coordinating discovery, storage, and encryption
- **internal/project/**: Project discovery (finds git root, parses `.envsecrets` file)
- **internal/git/**: Git operations via go-git library
- **internal/domain/**: Domain types (`RepoInfo`, `Commit`, `FileStatus`, `SyncStatus`) and error handling
- **internal/ui/**: User output and prompts

### Key Data Flow

**Push**: Local .env files → Encrypt with age → Write to local git cache → Commit → Upload to GCS

**Pull**: Download from GCS → Checkout commit → Decrypt → Write to project directory

### Configuration

- Config file: `~/.envsecrets/config.yaml`
- Cache directory: `~/.envsecrets/cache/owner/repo/`
- Project tracking file: `.envsecrets` (lists files to track, one per line)

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
