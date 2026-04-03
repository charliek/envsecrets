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
    Log(n int, includeFiles bool) ([]Commit, error)
    Checkout(ref string) error
    ListFiles() ([]string, error)
    ReadFile(path, ref string) ([]byte, error)
    WriteFile(path string, content []byte) error
    RemoveFile(path string) error
    Head() (string, error)
    PackAll(w io.Writer) error
    UnpackAll(r io.Reader) error
    GetAllRefs() (map[string]string, error)
    SetRef(name, hash string) error
    DeleteRef(name string) error
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
3. Sync from GCS: download packfile + refs, restore full git history locally
4. Fast-forward local branch to remote HEAD if behind
5. For each file:
   - Read plaintext from project directory
   - Encrypt with age
   - Write encrypted file to cache
6. Commit changes to cache git repo
7. Conflict check: verify remote HEAD hasn't changed since step 3
8. Sync to GCS: create packfile of all objects + refs, upload FORMAT version marker, upload HEAD last (HEAD is the existence marker)

### Pull

1. CLI parses flags and loads config
2. Project discovery finds repo identity
3. Sync from GCS: download packfile + refs, validate FORMAT version, restore full git history locally
4. Checkout requested ref (or HEAD) to populate working tree
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
            ├── .git/          # Full git history (restored from packfile)
            ├── .env.age       # Encrypted files (working tree, populated by checkout)
            └── .env.local.age
```

The cache is a git repository containing only encrypted files. Full git history
is synced between machines via packfiles stored in GCS.

## GCS Storage Layout

```text
{owner}/{repo}/FORMAT         # Storage format version marker (contains "1")
{owner}/{repo}/objects.pack   # Packfile containing all git objects
{owner}/{repo}/refs           # Text file: refname SP hash LF
{owner}/{repo}/HEAD           # Current HEAD commit hash (written last; existence marker)
```

Every sync downloads the packfile and restores full git history locally. This
enables `log`, `diff`, and `revert` to work correctly across machines with
shared commit history.

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
| ErrVersionTooNew | 15 | Storage format version not supported by this client |
| ErrVersionUnknown | 15 | Storage format marker missing (legacy repository) |

## Configuration Loading

1. Check `--config` flag
2. Check `ENVSECRETS_CONFIG` environment variable
3. Use default `~/.envsecrets/config.yaml`
4. Validate required fields
5. Resolve passphrase (env, command, or prompt)
