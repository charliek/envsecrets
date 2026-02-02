# Development Setup

Set up your development environment for envsecrets.

## Prerequisites

- Go 1.24 or later
- Docker (for integration tests)
- golangci-lint

## Clone and Build

```bash
git clone https://github.com/charliek/envsecrets.git
cd envsecrets
make build
```

## Running Tests

### Unit Tests

```bash
make test
```

### Integration Tests

Integration tests use testcontainers with fake-gcs-server:

```bash
make test-integration
```

### All Checks

```bash
make check  # lint + test
```

## Code Quality

### Linting

```bash
make lint
```

### Formatting

```bash
make fmt
```

## Building Documentation

```bash
make docs-serve
```

Open http://127.0.0.1:7070 to preview.

## Project Structure

```
envsecrets/
├── cmd/envsecrets/       # Entry point
├── internal/
│   ├── cli/              # Cobra commands
│   ├── config/           # Configuration management
│   ├── constants/        # Defaults and exit codes
│   ├── crypto/           # Age encryption
│   ├── domain/           # Types and errors
│   ├── git/              # Git operations
│   ├── project/          # Project discovery
│   ├── storage/          # GCS abstraction
│   ├── cache/            # Local cache
│   ├── sync/             # Push/pull orchestration
│   ├── ui/               # Terminal UI
│   └── version/          # Build info
├── test/integration/     # Integration tests
└── docs/                 # Documentation
```

## Testing Strategy

- **Unit tests**: Interface mocking, table-driven tests
- **Integration tests**: testcontainers with fake-gcs-server
- **Git tests**: go-git with memory storage

## Making Changes

1. Create a feature branch
2. Make changes with tests
3. Run `make check`
4. Submit a pull request
