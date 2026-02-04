# Architecture

Technical architecture and design decisions.

## Design Principles

- **Interface-based DI**: All external dependencies behind interfaces
- **Testability**: Mocks for all interfaces
- **Separation of concerns**: CLI, business logic, and infrastructure are separate

## Core Interfaces

### Storage

```go
type Storage interface {
    Upload(ctx context.Context, path string, r io.Reader) error
    Download(ctx context.Context, path string) (io.ReadCloser, error)
    List(ctx context.Context, prefix string) ([]string, error)
    Delete(ctx context.Context, path string) error
    Exists(ctx context.Context, path string) (bool, error)
}
```

### Encrypter

```go
type Encrypter interface {
    Encrypt(plaintext []byte) ([]byte, error)
    Decrypt(ciphertext []byte) ([]byte, error)
}
```

### Repository

```go
type Repository interface {
    Init() error
    Add(paths ...string) error
    Commit(message string) (string, error)
    Log(n int) ([]Commit, error)
    Checkout(ref string) error
    ListFiles() ([]string, error)
    ReadFile(path, ref string) ([]byte, error)
    WriteFile(path string, content []byte) error
    RemoveFile(path string) error
    Head() (string, error)
}
```

## Package Dependencies

```text
cmd/envsecrets
    └── internal/cli
            ├── internal/config
            ├── internal/sync
            │       ├── internal/storage
            │       ├── internal/crypto
            │       ├── internal/git
            │       └── internal/cache
            ├── internal/project
            └── internal/ui
```

## Data Flow

### Push

1. CLI parses flags and loads config
2. Project discovery finds repo identity and env files
3. For each file:
   - Read plaintext from project directory
   - Encrypt with age
   - Write encrypted file to cache
4. Commit changes to cache git repo
5. Sync cache to GCS

### Pull

1. CLI parses flags and loads config
2. Project discovery finds repo identity
3. Sync cache from GCS
4. Checkout requested ref (or HEAD)
5. For each file in cache:
   - Read encrypted file
   - Decrypt with age
   - Write plaintext to project directory

## Cache Structure

```text
~/.envsecrets/
├── config.yaml
└── cache/
    └── {owner}/
        └── {repo}/
            ├── .git/
            ├── .env.age
            └── .env.local.age
```

The cache is a git repository containing only encrypted files. This provides:

- Version history
- Efficient sync (only changed files)
- Atomic operations

## Error Handling

Sentinel errors in `internal/domain/errors.go` map to exit codes:

| Error | Exit Code | Description |
|-------|-----------|-------------|
| ErrNotConfigured | 1 | Missing configuration |
| ErrNotInRepo | 2 | Not in a git repository |
| ErrNoEnvFiles | 3 | No .envsecrets file found |
| ErrConflict | 4 | Local/remote conflict |
| ErrDecryptFailed | 5 | Decryption failed |
| ErrUploadFailed | 6 | GCS upload failed |
| ErrDownloadFailed | 7 | GCS download failed |

## Configuration Loading

1. Check `--config` flag
2. Check `ENVSECRETS_CONFIG` environment variable
3. Use default `~/.envsecrets/config.yaml`
4. Validate required fields
5. Resolve passphrase (env, command, or prompt)
