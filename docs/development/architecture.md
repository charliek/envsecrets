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
3. Read this machine's `LAST_SYNCED` baseline (per-machine marker, never uploaded)
4. Sync from GCS: download packfile + refs, restore full git history locally
5. Fast-forward local branch to remote HEAD if behind
6. **Divergence safety check** — if `LAST_SYNCED != remote HEAD` AND any tracked file changed both locally (vs `LAST_SYNCED`) AND remotely (between `LAST_SYNCED` and HEAD), refuse with `ErrDivergedHistory` unless `--force` is set
7. For each file:
   - Read plaintext from project directory
   - Encrypt with age
   - Write encrypted file to cache
8. Commit changes to cache git repo (author = `$USER@<machine_id-or-hostname>`)
9. Optimistic locking check: verify remote HEAD hasn't changed since step 4
10. Sync to GCS: create packfile of all objects + refs, upload FORMAT version marker, upload HEAD last (HEAD is the existence marker)
11. Update `LAST_SYNCED` to the new commit. Failure here surfaces a `Warning` on the result but does NOT roll back the successful remote push

### Pull

1. CLI parses flags and loads config
2. Project discovery finds repo identity
3. Sync from GCS: download packfile + refs, validate FORMAT version, restore full git history locally
4. Checkout requested ref (or HEAD) to populate working tree
5. Read this machine's `LAST_SYNCED` baseline
6. For each tracked file, classify against (working tree, baseline, remote HEAD):
   - No local edits, remote moved → overwrite (catch-up case, no prompt)
   - Local edits, remote unchanged for this file → preserve local (push will publish)
   - Both sides changed → real conflict (resolver / `--force` / abort)
   - No baseline available → fall back to old pessimistic behavior
7. Decrypt and write the files chosen for overwrite
8. Update `LAST_SYNCED` to the new HEAD (only on full-HEAD pull; `--ref` checkouts do NOT update the marker)

### Status / Sync

1. Read `LAST_SYNCED` baseline + sync from GCS (so cache reflects remote)
2. Run the same 3-way classification as pull, producing per-file `LocalChanges` / `RemoteChanges` / `Conflicts` slices
3. Map to a `SyncAction`: `in_sync` / `push` / `pull` / `pull_then_push` / `reconcile` / `first_push_init` / `first_pull` / `nothing_tracked`
4. `status` renders the action plus provenance (remote HEAD's author/timestamp, this machine's last-synced commit + age) for the user
5. `sync` executes the action automatically — push, pull, or pull-then-push — and refuses with exit 16 (`ExitActionRequired`) on `reconcile` or `first_push_init` (initialization is intentionally manual)

## Cache Structure

```text
~/.envsecrets/
├── config.yaml
└── cache/
    └── {owner}/
        └── {repo}/
            ├── .git/                              # Full git history (restored from packfile)
            │   └── .envsecrets-last-synced        # Per-machine baseline marker; never uploaded
            ├── .env.age                           # Encrypted files (working tree, populated by checkout)
            └── .env.local.age
```

The cache is a git repository containing only encrypted files. Full git history
is synced between machines via packfiles stored in GCS.

The `LAST_SYNCED` marker lives **inside** `.git/` so go-git's force-checkout
during `pull --ref` cannot wipe it as an "untracked" file. It contains a
single 40-char hex commit hash recording the most recent `push` or full-HEAD
`pull` this machine successfully completed. `cache.Reset()` (which removes
the entire cache directory) intentionally clears it — Reset implies the
cache is no longer trusted, so the baseline must also be discarded.

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
| ErrDivergedHistory | 4 | Push refused: remote moved AND files overlap (use `pull` or `--force`) |
| ErrRemoteChanged | 4 | Push optimistic locking: remote moved during the push window |
| ErrDecryptFailed | 5 | Decryption failed |
| ErrUploadFailed | 6 | GCS upload failed |
| ErrDownloadFailed | 7 | GCS download failed |
| ErrVersionTooNew | 15 | Storage format version not supported by this client |
| ErrVersionUnknown | 15 | Storage format marker missing (legacy repository) |
| ErrActionRequired | 16 | `sync` reached a state requiring user action (`reconcile` or `first_push_init`) |

## Configuration Loading

1. Check `--config` flag
2. Check `ENVSECRETS_CONFIG` environment variable
3. Use default `~/.envsecrets/config.yaml`
4. Validate required fields
5. Resolve passphrase (env, command, or prompt)
