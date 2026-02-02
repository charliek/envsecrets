# envsecrets

A CLI tool for managing encrypted environment files across development environments using Google Cloud Storage and age encryption.

## Overview

### Problem Statement

When developing in remote containers (e.g., via the "shed" workflow), projects often require `.env` files containing secrets that are not committed to version control. These secrets need to be:

- Securely stored and versioned
- Easily synced to remote development containers
- Trackable with change history
- Simple to manage across multiple projects

### Goals

1. **Simple secret management**: Push and pull `.env` files with minimal friction
2. **Defense in depth**: Require both GCP credentials AND encryption passphrase to access secrets
3. **Version history**: Track changes over time using git, with ability to view diffs and revert
4. **Auto-discovery**: Automatically detect repo identity and env files to track
5. **Portable configuration**: Single config file that can be synced to containers
6. **Offline-capable encryption**: Use age passphrase encryption (no network required for encrypt/decrypt)

### Non-Goals

- Team/multi-user secret sharing (single passphrase model)
- Production secret management (this is for development workflows)
- Integration with external secret managers (Vault, AWS Secrets Manager, etc.)

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Developer Machine                           │
│                                                                     │
│  ~/.envsecrets/config.yaml                                          │
│    - GCP credentials (base64)                                       │
│    - Passphrase (optional)                                          │
│    - Bucket name                                                    │
│                                                                     │
│  ~/code/myproject/                                                  │
│    - .env              ←─── envsecrets pull                         │
│    - .env.local             envsecrets push ───→                    │
│    - .envsecrets (tracking config)                                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTPS (GCP authenticated)
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      Google Cloud Storage                           │
│                                                                     │
│  gs://bucket-name/                                                  │
│  ├── charliek/                                                      │
│  │   └── stbot/                                                     │
│  │       ├── .git/              (git repo for history)              │
│  │       ├── .env.age           (age encrypted)                     │
│  │       └── .env.local.age     (age encrypted)                     │
│  └── smartthings/                                                   │
│      └── platform/                                                  │
│          ├── .git/                                                  │
│          └── .env.age                                               │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Configuration

### Config File Location

```
~/.envsecrets/config.yaml
```

### Config File Format

```yaml
# GCP project ID
project: my-gcp-project

# GCS bucket name for storing encrypted secrets
bucket: my-secrets-bucket

# Base64-encoded GCP service account JSON
# Generate with: envsecrets encode /path/to/service-account.json
gcp_credentials: ewogICJ0eXBlIjogInNlcnZpY2VfYWNjb3VudCIsCiAgInByb2plY3RfaWQiOiAibXktcHJvamVjdCIs...

# Optional: age passphrase for encryption/decryption
# If omitted, will prompt at runtime
passphrase: my-secret-passphrase
```

### Environment Variable Overrides

| Variable | Purpose |
|----------|---------|
| `ENVSECRETS_PASSPHRASE` | Override passphrase (useful for CI/scripts) |
| `ENVSECRETS_CONFIG` | Override config file path |

### Passphrase Resolution Order

1. `--passphrase` flag (if implemented)
2. `ENVSECRETS_PASSPHRASE` environment variable
3. `passphrase` field in config.yaml
4. Interactive prompt

---

## Project Tracking

### `.envsecrets` File

Each project can have a `.envsecrets` file in its root to specify which files to track:

```
# .envsecrets
# Optional repo override (otherwise auto-detected from git remote)
# repo: charliek/custom-name

# Files to track (one per line)
.env
.env.local
.env.production
config/secrets.yaml
```

### `.gitignore` Marker (Alternative)

If no `.envsecrets` file exists, look for a marked section in `.gitignore`:

```gitignore
# Build output
/dist
/build

# envsecrets
.env
.env.local
.env.production

# Logs
*.log
```

Everything between `# envsecrets` and the next blank line or comment section is treated as tracked files.

### Discovery Precedence

1. `.envsecrets` file in repo root
2. `# envsecrets` marked section in `.gitignore`
3. Error: "No env files configured. Create .envsecrets or add # envsecrets section to .gitignore"

---

## Repository Identification

### Auto-Detection

Extract `owner/repo` format from git remote:

| Remote URL | Extracted Identifier |
|------------|---------------------|
| `git@github.com:charliek/stbot.git` | `charliek/stbot` |
| `https://github.com/charliek/stbot.git` | `charliek/stbot` |
| `git@gitlab.com:smartthings/platform.git` | `smartthings/platform` |
| `https://gitlab.com/smartthings/platform` | `smartthings/platform` |

### Override

Can be overridden via:

1. `repo:` directive in `.envsecrets` file
2. `--repo` CLI flag

### Algorithm

```
1. If --repo flag provided, use it
2. If .envsecrets has repo: directive, use it
3. Try to read git remote "origin" URL
4. Extract owner/repo from URL
5. If no git remote, error with suggestion to use --repo or add repo: to .envsecrets
```

---

## Bucket Structure

```
gs://bucket-name/
├── owner1/
│   ├── repo1/
│   │   ├── .git/
│   │   │   ├── HEAD
│   │   │   ├── config
│   │   │   ├── objects/
│   │   │   └── refs/
│   │   ├── .env.age
│   │   └── .env.local.age
│   └── repo2/
│       ├── .git/
│       └── .env.age
└── owner2/
    └── repo3/
        ├── .git/
        ├── .env.age
        └── config/
            └── secrets.yaml.age
```

### File Naming

- Original: `.env` → Stored as: `.env.age`
- Original: `config/secrets.yaml` → Stored as: `config/secrets.yaml.age`

### Git Repository

Each project directory in the bucket is a bare-ish git repository:
- Tracks history of all encrypted files
- Commit messages capture change descriptions
- Standard git operations (log, diff, revert) work on encrypted content

---

## Encryption

### Algorithm

**age** with scrypt passphrase-based encryption.

### Why age?

- Simple, modern, audited
- Passphrase-based encryption (no key files to manage)
- Excellent Go library (`filippo.io/age`)
- Small encrypted output overhead
- Stream-based (handles large files efficiently)

### Encryption Flow

```
┌──────────────┐     age encrypt      ┌──────────────┐
│   .env       │  ────────────────►   │  .env.age    │
│  (plaintext) │    (passphrase)      │ (encrypted)  │
└──────────────┘                      └──────────────┘
```

### Decryption Flow

```
┌──────────────┐     age decrypt      ┌──────────────┐
│  .env.age    │  ────────────────►   │    .env      │
│ (encrypted)  │    (passphrase)      │  (plaintext) │
└──────────────┘                      └──────────────┘
```

### Security Model

Access requires **both**:
1. GCP credentials with bucket access
2. Encryption passphrase

Compromise of either alone is insufficient to read secrets.

---

## CLI Interface

### Command Overview

| Command | Description |
|---------|-------------|
| `envsecrets status` | Show discovered repo info and file status |
| `envsecrets push` | Encrypt and push env files to remote |
| `envsecrets pull` | Pull and decrypt env files to local |
| `envsecrets log` | Show history of changes |
| `envsecrets diff` | Show changes between versions |
| `envsecrets revert` | Restore files from a previous version |
| `envsecrets list` | List all repos or files in bucket |
| `envsecrets rm` | Remove a file from tracking |
| `envsecrets delete` | Delete entire repo from bucket |
| `envsecrets init` | Initialize config file |
| `envsecrets encode` | Encode service account JSON to base64 |
| `envsecrets doctor` | Verify configuration and connectivity |
| `envsecrets rotate-passphrase` | Re-encrypt all repos with new passphrase |
| `envsecrets verify` | Test decryption across all repos |

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--repo` | `-r` | Override repo identifier |
| `--config` | `-c` | Path to config file |
| `--verbose` | `-v` | Verbose output |
| `--quiet` | `-q` | Minimal output |
| `--dry-run` | | Show what would happen without doing it |
| `--non-interactive` | | Fail instead of prompting (for scripts) |

---

### Command Details

#### `envsecrets status`

Show current state without making changes.

```bash
$ envsecrets status

Repo:       charliek/stbot
Source:     git remote (origin)

Env files:  .envsecrets file
  .env              (exists locally, modified)
  .env.local        (exists locally, unchanged)
  .env.production   (missing locally)

Remote:
  Last push:  2024-01-15 10:32:04
  Commit:     abc1234
  Files:      3
```

**Exit codes:**
- 0: Success
- 1: Error (no repo detected, config issues, etc.)

---

#### `envsecrets push`

Encrypt and upload env files.

```bash
# Push all tracked files
$ envsecrets push

# Push with commit message
$ envsecrets push -m "Added Redis credentials"

# Push specific files only
$ envsecrets push .env .env.local

# Push to different repo
$ envsecrets push --repo charliek/other-name

# Preview without pushing
$ envsecrets push --dry-run

# Force push even if no changes detected
$ envsecrets push --force
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--message` | `-m` | Commit message (default: timestamp) |
| `--force` | `-f` | Push even if no changes |
| `--dry-run` | | Show what would be pushed |

**Behavior:**
1. Resolve repo identifier
2. Discover tracked files from `.envsecrets` or `.gitignore`
3. For each file that exists locally:
   - Read file content
   - Encrypt with age passphrase
   - Stage in local git working copy
4. Commit with message (or timestamp if none provided)
5. Sync git repo to GCS bucket

**Output:**
```
Repo: charliek/stbot
Pushing 2 files...
  .env              ✓
  .env.local        ✓

✓ Pushed with message: "Added Redis credentials"
  Commit: def5678
```

**Warnings:**
```
Warning: .env.production listed in .envsecrets but not found locally
Push remaining 2 files? [Y/n]
```

---

#### `envsecrets pull`

Download and decrypt env files.

```bash
# Pull all tracked files
$ envsecrets pull

# Pull specific file
$ envsecrets pull .env

# Pull specific version
$ envsecrets pull --ref abc1234

# Pull to different directory
$ envsecrets pull --out ./secrets/

# Overwrite without prompting
$ envsecrets pull --force

# Preview without writing
$ envsecrets pull --dry-run
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--ref` | | Pull specific commit |
| `--out` | `-o` | Output directory (default: current) |
| `--force` | `-f` | Overwrite existing files without prompting |
| `--dry-run` | | Show what would be pulled |

**Behavior:**
1. Resolve repo identifier
2. Sync git repo from GCS bucket to local cache
3. Checkout specified ref (or HEAD)
4. For each tracked file in remote:
   - Read encrypted content
   - Decrypt with age passphrase
   - Check if local file exists and differs
   - Write to local filesystem (with conflict handling)

**Conflict Handling (interactive mode):**
```
.env exists locally and differs from remote.
  [o]verwrite / [s]kip / [d]iff / [a]bort? d

Local has:
  + DEBUG=true
  + LOCAL_ONLY=yes

Remote has:
  + REDIS_URL=redis://prod:6379

  [o]verwrite / [s]kip / [a]bort?
```

**Output:**
```
Repo: charliek/stbot
Pulling 3 files...
  .env              ✓
  .env.local        ✓ (overwritten)
  .env.production   ✓ (new)
```

---

#### `envsecrets log`

Show commit history.

```bash
# Show recent history
$ envsecrets log

# Show more entries
$ envsecrets log -n 20

# Show history for specific file
$ envsecrets log .env

# Verbose output (show files changed)
$ envsecrets log -v
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--number` | `-n` | Number of entries to show (default: 10) |
| `--verbose` | `-v` | Show files changed in each commit |

**Output:**
```
Repo: charliek/stbot

abc1234  2024-01-15 10:32:04  Added Redis credentials
def5678  2024-01-10 14:20:11  Updated API keys
ghi9012  2024-01-05 09:15:33  Initial commit
```

**Verbose output:**
```
Repo: charliek/stbot

abc1234  2024-01-15 10:32:04  Added Redis credentials
         .env, .env.local

def5678  2024-01-10 14:20:11  Updated API keys
         .env

ghi9012  2024-01-05 09:15:33  Initial commit
         .env, .env.local, .env.production
```

---

#### `envsecrets diff`

Show differences between versions.

```bash
# Diff local vs remote HEAD (what would change on push)
$ envsecrets diff

# Diff between two commits
$ envsecrets diff abc1234 def5678

# Diff local vs specific commit
$ envsecrets diff --ref abc1234

# Diff specific file only
$ envsecrets diff .env

# Diff specific file at specific commits
$ envsecrets diff .env abc1234 def5678
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--ref` | | Compare local against specific commit |

**Output:**
```
Repo: charliek/stbot

.env:
  + NEW_VAR=value
  - OLD_VAR=removed
  ~ DATABASE_URL: "postgres://old" -> "postgres://new"

.env.local:
  (no changes)
```

**Note:** Diff is performed on decrypted content. Requires passphrase.

---

#### `envsecrets revert`

Restore files from a previous version.

```bash
# Interactive: show log and pick
$ envsecrets revert

# Revert to specific commit
$ envsecrets revert abc1234

# Revert specific file only
$ envsecrets revert abc1234 .env

# Revert and push as new commit
$ envsecrets revert abc1234 --push -m "Reverted bad change"

# Preview without writing
$ envsecrets revert abc1234 --dry-run
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--push` | `-p` | Push reverted state as new commit |
| `--message` | `-m` | Commit message (used with --push) |
| `--dry-run` | | Show what would be reverted |

**Interactive mode:**
```
Repo: charliek/stbot

  1. abc1234  2024-01-15  Added Redis credentials
  2. def5678  2024-01-10  Updated API keys
  3. ghi9012  2024-01-05  Initial commit

Revert to [1-3]: 2

Reverting to def5678...
  .env              ✓
  .env.local        ✓
```

---

#### `envsecrets list`

List repos or files.

```bash
# List all repos in bucket
$ envsecrets list

# List files in specific repo
$ envsecrets list charliek/stbot

# List files in current repo
$ envsecrets list --current
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--current` | | List files in auto-detected current repo |

**Output (repos):**
```
charliek/stbot           3 files   2024-01-15 10:32:04
charliek/envsecrets      1 file    2024-01-12 08:15:22
smartthings/platform     2 files   2024-01-10 14:20:11
```

**Output (files):**
```
charliek/stbot:

.env               245 bytes   2024-01-15 10:32:04
.env.local         128 bytes   2024-01-15 10:32:04
.env.production    312 bytes   2024-01-05 09:15:33
```

---

#### `envsecrets rm`

Remove a file from tracking.

```bash
# Remove file (keeps history)
$ envsecrets rm .env.old

# Remove from specific repo
$ envsecrets rm --repo charliek/stbot .env.old
```

**Behavior:**
1. Remove file from git repo
2. Commit removal
3. Sync to bucket
4. History is preserved (can still revert to commits that had the file)

**Output:**
```
Removing .env.old from charliek/stbot...
✓ Removed

Note: File history is preserved. Use 'envsecrets revert' to restore.
```

---

#### `envsecrets delete`

Delete entire repo from bucket.

```bash
$ envsecrets delete charliek/stbot
```

**Behavior:**
1. Require explicit confirmation (type repo name)
2. Delete entire directory from bucket
3. This is destructive and cannot be undone

**Output:**
```
This will permanently delete all secrets and history for charliek/stbot.
Type the full repo name to confirm: charliek/stbot

Deleting charliek/stbot...
✓ Deleted
```

---

#### `envsecrets init`

Initialize configuration file.

```bash
$ envsecrets init
```

**Interactive flow:**
```
Creating ~/.envsecrets/config.yaml

GCP Project ID: my-project
Bucket name: my-secrets-bucket
Path to service account JSON: ~/Downloads/sa-key.json
Passphrase (empty to prompt each time): ****
Confirm passphrase: ****

✓ Created ~/.envsecrets/config.yaml

Test connection? [Y/n] y
✓ Connected to gs://my-secrets-bucket
```

**If config exists:**
```
Config file already exists at ~/.envsecrets/config.yaml
Overwrite? [y/N]
```

---

#### `envsecrets encode`

Encode service account JSON to base64.

```bash
$ envsecrets encode ~/path/to/service-account.json

ewogICJ0eXBlIjogInNlcnZpY2VfYWNjb3VudCIs...

Add this to your ~/.envsecrets/config.yaml as 'gcp_credentials'
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--copy` | | Copy to clipboard (macOS/Linux) |

---

#### `envsecrets doctor`

Verify configuration and connectivity.

```bash
$ envsecrets doctor

Config:      ~/.envsecrets/config.yaml ✓
GCP Project: my-project ✓
Bucket:      my-secrets-bucket ✓
GCP Auth:    ✓ (sa@project.iam.gserviceaccount.com)
Passphrase:  configured in file ✓

All checks passed!
```

**Failure example:**
```
Config:      ~/.envsecrets/config.yaml ✓
GCP Project: my-project ✓
Bucket:      my-secrets-bucket ✗
  Error: bucket not found or access denied

GCP Auth:    ✗
  Error: invalid credentials

1 check passed, 2 checks failed
```

---

#### `envsecrets rotate-passphrase`

Re-encrypt all repos with a new passphrase.

```bash
$ envsecrets rotate-passphrase

Current passphrase: ****
New passphrase: ****
Confirm new passphrase: ****

Re-encrypting 3 repos...
  charliek/stbot         ✓ (3 files)
  charliek/envsecrets    ✓ (1 file)
  smartthings/platform   ✓ (2 files)

✓ Passphrase rotated

Update your config? [Y/n] y
✓ Updated ~/.envsecrets/config.yaml
```

**Behavior:**
1. Prompt for current and new passphrase
2. For each repo:
   - Pull and decrypt all files with old passphrase
   - Re-encrypt with new passphrase
   - Push with commit message "Passphrase rotation"
3. Optionally update config file

---

#### `envsecrets verify`

Test decryption across all repos.

```bash
$ envsecrets verify

Testing decryption for 3 repos...
  charliek/stbot         ✓ (3 files)
  charliek/envsecrets    ✓ (1 file)
  smartthings/platform   ✓ (2 files)

All repos verified successfully!
```

**Failure example:**
```
Testing decryption for 3 repos...
  charliek/stbot         ✓ (3 files)
  charliek/envsecrets    ✗
    Error: decryption failed - incorrect passphrase?
  smartthings/platform   ✓ (2 files)

1 repo failed verification
```

---

## Implementation

### Language

**Go** - chosen for:
- Single binary distribution (no runtime dependencies)
- Excellent libraries for GCS and git
- Fast startup time
- Easy cross-compilation for Linux containers

### Dependencies

| Library | Purpose | URL |
|---------|---------|-----|
| `cloud.google.com/go/storage` | GCS client | https://pkg.go.dev/cloud.google.com/go/storage |
| `google.golang.org/api/option` | GCP client options | https://pkg.go.dev/google.golang.org/api/option |
| `github.com/go-git/go-git/v5` | Pure Go git implementation | https://github.com/go-git/go-git |
| `filippo.io/age` | age encryption | https://github.com/FiloSottile/age |
| `github.com/spf13/cobra` | CLI framework | https://github.com/spf13/cobra |
| `gopkg.in/yaml.v3` | YAML parsing | https://github.com/go-yaml/yaml |
| `golang.org/x/term` | Terminal password input | https://pkg.go.dev/golang.org/x/term |

### Project Structure

```
envsecrets/
├── cmd/
│   └── envsecrets/
│       └── main.go
├── internal/
│   ├── cli/
│   │   ├── root.go
│   │   ├── push.go
│   │   ├── pull.go
│   │   ├── status.go
│   │   ├── log.go
│   │   ├── diff.go
│   │   ├── revert.go
│   │   ├── list.go
│   │   ├── rm.go
│   │   ├── delete.go
│   │   ├── init.go
│   │   ├── encode.go
│   │   ├── doctor.go
│   │   ├── rotate.go
│   │   └── verify.go
│   ├── config/
│   │   └── config.go
│   ├── crypto/
│   │   └── age.go
│   ├── repo/
│   │   ├── discover.go
│   │   ├── envfiles.go
│   │   └── identity.go
│   ├── storage/
│   │   ├── gcs.go
│   │   └── git.go
│   └── ui/
│       ├── prompt.go
│       └── output.go
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Core Interfaces

```go
// internal/config/config.go
package config

type Config struct {
    Project        string `yaml:"project"`
    Bucket         string `yaml:"bucket"`
    Passphrase     string `yaml:"passphrase,omitempty"`
    GCPCredentials string `yaml:"gcp_credentials"`
}

func Load() (*Config, error)
func (c *Config) GetPassphrase() (string, error)
func (c *Config) GetGCPCredentialsJSON() ([]byte, error)
func (c *Config) NewStorageClient(ctx context.Context) (*storage.Client, error)
```

```go
// internal/crypto/age.go
package crypto

func Encrypt(data []byte, passphrase string) ([]byte, error)
func Decrypt(encrypted []byte, passphrase string) ([]byte, error)
```

```go
// internal/repo/discover.go
package repo

type RepoInfo struct {
    Identifier string   // "owner/repo"
    Source     string   // "git remote", ".envsecrets", "--repo flag"
    EnvFiles   []string // tracked files
    FileSource string   // ".envsecrets file", ".gitignore marker"
}

func Discover(repoOverride string) (*RepoInfo, error)
```

```go
// internal/storage/gcs.go
package storage

type GCSStore struct {
    client *storage.Client
    bucket string
}

func NewGCSStore(ctx context.Context, cfg *config.Config) (*GCSStore, error)
func (s *GCSStore) Upload(ctx context.Context, path string, data []byte) error
func (s *GCSStore) Download(ctx context.Context, path string) ([]byte, error)
func (s *GCSStore) List(ctx context.Context, prefix string) ([]string, error)
func (s *GCSStore) Delete(ctx context.Context, path string) error
func (s *GCSStore) SyncToLocal(ctx context.Context, prefix, localPath string) error
func (s *GCSStore) SyncFromLocal(ctx context.Context, localPath, prefix string) error
```

```go
// internal/storage/git.go
package storage

type GitRepo struct {
    repo *git.Repository
    path string
}

func OpenOrInit(path string) (*GitRepo, error)
func (r *GitRepo) Add(files ...string) error
func (r *GitRepo) Commit(message string) (string, error)
func (r *GitRepo) Log(n int) ([]Commit, error)
func (r *GitRepo) Diff(ref1, ref2 string) ([]FileDiff, error)
func (r *GitRepo) Checkout(ref string) error
func (r *GitRepo) ListFiles() ([]string, error)
func (r *GitRepo) ReadFile(path string) ([]byte, error)
func (r *GitRepo) RemoveFile(path string) error
```

### Workflow: Push

```
1. Load config
2. Get passphrase (from config, env, or prompt)
3. Discover repo (identity + tracked files)
4. Create GCS client
5. Sync remote repo to local temp directory
6. For each tracked file that exists locally:
   a. Read local file
   b. Encrypt with age
   c. Write to temp repo as {filename}.age
7. Git add all .age files
8. Git commit with message
9. Sync temp repo back to GCS
10. Clean up temp directory
```

### Workflow: Pull

```
1. Load config
2. Get passphrase (from config, env, or prompt)
3. Discover repo identity
4. Create GCS client
5. Sync remote repo to local temp directory
6. Git checkout specified ref (or HEAD)
7. List all .age files in temp repo
8. For each .age file:
   a. Read encrypted content
   b. Decrypt with age
   c. Determine output path (remove .age suffix)
   d. Check for local conflicts
   e. Write to local filesystem
9. Clean up temp directory
```

### Local Cache Strategy

For v1, use temporary directories for each operation:
- `os.MkdirTemp()` for working directory
- Clean up after operation completes

Future optimization: persistent cache at `~/.envsecrets/cache/` to avoid repeated downloads.

---

## Error Handling

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Config error (missing, invalid) |
| 3 | Auth error (GCP credentials) |
| 4 | Encryption error (wrong passphrase) |
| 5 | Network error (GCS unreachable) |
| 6 | Git error |
| 10 | User cancelled |

### Common Error Messages

```
Error: config file not found at ~/.envsecrets/config.yaml
  Run 'envsecrets init' to create one

Error: could not detect repository
  Add 'repo: owner/name' to .envsecrets or use --repo flag

Error: no env files configured
  Create .envsecrets file or add '# envsecrets' section to .gitignore

Error: decryption failed
  Check your passphrase is correct

Error: bucket access denied
  Verify GCP credentials have storage.objects.* permissions on bucket
```

---

## Security Considerations

### Threat Model

**Protected against:**
- Bucket access without passphrase (files are encrypted)
- Passphrase without bucket access (can't retrieve files)
- Accidental commit of secrets (files tracked via .envsecrets, not git)

**Not protected against:**
- Compromise of ~/.envsecrets/config.yaml (contains both credentials)
- Memory scraping on machine where decryption occurs
- Malicious modification of encrypted files (no authentication tag verification beyond age)

### Recommendations

1. Use a strong passphrase (16+ characters)
2. Don't reuse the passphrase elsewhere
3. Restrict bucket IAM to specific service account
4. Rotate passphrase periodically
5. For higher security, omit passphrase from config and enter at runtime

### Bucket IAM

Minimum required permissions for service account:

```
storage.objects.create
storage.objects.delete
storage.objects.get
storage.objects.list
```

Recommended: Create a dedicated bucket with restricted access rather than using a shared bucket.

---

## Future Enhancements

### v1.1
- `rotate-passphrase` command
- `verify` command
- Shell completions (bash, zsh, fish)
- Homebrew formula

### v2.0 (Maybe)
- Multiple passphrases/recipients for team sharing
- age identity files (in addition to passphrase)
- Hooks (post-pull scripts)
- Export/import for backup/migration
- Web UI for browsing history

---

## Appendix: Example Session

```bash
# First time setup
$ envsecrets init
Creating ~/.envsecrets/config.yaml
GCP Project ID: charliek-dev
Bucket name: charliek-envsecrets
Path to service account JSON: ~/Downloads/envsecrets-sa.json
Passphrase (empty to prompt each time): ********
Confirm passphrase: ********
✓ Created ~/.envsecrets/config.yaml

# In a project directory
$ cd ~/code/stbot
$ cat .envsecrets
.env
.env.local

# Check status
$ envsecrets status
Repo:       charliek/stbot
Source:     git remote (origin)
Env files:  .envsecrets file
  .env              (exists locally)
  .env.local        (exists locally)
Remote:     not yet initialized

# Initial push
$ envsecrets push -m "Initial secrets"
Pushing 2 files...
  .env              ✓
  .env.local        ✓
✓ Pushed with message: "Initial secrets"

# Later, on a remote container
$ envsecrets pull
Repo: charliek/stbot
Pulling 2 files...
  .env              ✓
  .env.local        ✓

# Make changes, push with message
$ echo "NEW_KEY=value" >> .env
$ envsecrets push -m "Added NEW_KEY"
Pushing 1 file...
  .env              ✓
✓ Pushed with message: "Added NEW_KEY"

# View history
$ envsecrets log
abc1234  2024-01-15 10:35:22  Added NEW_KEY
def5678  2024-01-15 10:30:00  Initial secrets

# Oops, revert
$ envsecrets revert def5678
Reverting to def5678...
  .env              ✓
  .env.local        ✓
```
