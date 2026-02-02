# Spec vs Implementation Analysis

This document tracks differences between the original specification (`docs/spec.md`) and the current implementation. Use this to understand what features remain to be implemented in future PRs.

---

## Unimplemented Features

### Global Flags

The spec (lines 294-302) defines these global flags that are not yet implemented:

| Flag | Spec Description | Status |
|------|------------------|--------|
| `--repo, -r` | Override repo identifier (bypass git remote detection) | Not implemented |
| `--quiet, -q` | Minimal output | Not implemented |
| `--dry-run` | Show what would happen without doing it | Only on `push` command |
| `--non-interactive` | Fail instead of prompting (for scripts) | Not implemented |

**Implementation notes:**
- `--repo` would need to be threaded through `NewProjectContext()` and `project.NewDiscovery()`
- `--quiet` could suppress all non-error output
- `--dry-run` global would affect `pull`, `rm`, `delete`, `revert`, `rotate-passphrase`
- `--non-interactive` should cause any prompt to return an error instead

### Command-Specific Flags

#### `pull` command (spec lines 395-426)

| Flag | Spec Description | Status |
|------|------------------|--------|
| `--out, -o` | Output directory (default: current) | Not implemented |

**Implementation notes:**
- Would write decrypted files to specified directory instead of project root
- Useful for extracting secrets to a different location

#### `revert` command (spec lines 555-597)

| Flag | Spec Description | Status |
|------|------------------|--------|
| `--push, -p` | Push reverted state as new commit | Not implemented |
| `--message, -m` | Commit message (used with --push) | Not implemented |

**Current behavior:** Revert writes files locally and tells user to run `push` manually.

**Spec behavior:** With `--push`, would automatically push the reverted state.

#### `list` command (spec lines 600-635)

| Flag | Spec Description | Status |
|------|------------------|--------|
| `--current` | List files in auto-detected current repo | Not implemented |

**Current behavior:** Must specify repo name explicitly: `envsecrets list owner/repo`

**Spec behavior:** `envsecrets list --current` would auto-detect repo from git remote.

#### `encode` command (spec lines 722-738)

| Flag | Spec Description | Status |
|------|------------------|--------|
| `--copy` | Copy to clipboard (macOS/Linux) | Not implemented |

**Implementation notes:**
- macOS: pipe to `pbcopy`
- Linux: pipe to `xclip -selection clipboard` or `xsel --clipboard`

### Discovery Features

#### `.gitignore` Marker (spec lines 133-157)

**Spec describes:** If no `.envsecrets` file exists, parse `.gitignore` for a marked section:

```gitignore
# envsecrets
.env
.env.local
.env.production

# Logs
*.log
```

Everything between `# envsecrets` and the next blank line or comment section is treated as tracked files.

**Current behavior:** Only `.envsecrets` file is supported. No `.gitignore` fallback.

**Files affected:** `internal/project/envfiles.go`, `internal/project/discovery.go`

#### `repo:` Directive in `.envsecrets` (spec lines 121-130)

**Spec describes:** The `.envsecrets` file can contain a `repo:` directive to override auto-detection:

```
# .envsecrets
repo: charliek/custom-name

.env
.env.local
```

**Current behavior:** Not implemented. The `.envsecrets` file only contains file paths.

**Files affected:** `internal/project/envfiles.go`

### Interactive Features

#### Pull Conflict Handling (spec lines 437-450)

**Spec describes:** When pulling and local file differs from remote:

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

**Current behavior:** With `--force`, overwrites. Without `--force`, fails on conflict.

**Implementation notes:**
- Would require interactive prompt in `internal/sync/pull.go`
- The `[d]iff` option would show a diff and re-prompt

#### Revert Interactive Mode (spec lines 583-596)

**Spec describes:** Running `envsecrets revert` without a ref shows recent commits and lets user pick:

```
Repo: charliek/stbot

  1. abc1234  2024-01-15  Added Redis credentials
  2. def5678  2024-01-10  Updated API keys
  3. ghi9012  2024-01-05  Initial commit

Revert to [1-3]: 2
```

**Current behavior:** Requires ref as argument: `envsecrets revert <ref>`

### Output Format Enhancements

#### `status` Command (spec lines 308-332)

**Spec output:**
```
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

**Current output:** Similar but less detailed per-file status indicators.

#### `list` Command (spec lines 620-634)

**Spec output (repos):**
```
charliek/stbot           3 files   2024-01-15 10:32:04
charliek/envsecrets      1 file    2024-01-12 08:15:22
smartthings/platform     2 files   2024-01-10 14:20:11
```

**Spec output (files):**
```
charliek/stbot:

.env               245 bytes   2024-01-15 10:32:04
.env.local         128 bytes   2024-01-15 10:32:04
.env.production    312 bytes   2024-01-05 09:15:33
```

**Current output:** Simple list without counts, sizes, or timestamps.

#### `push` Warning (spec lines 388-391)

**Spec behavior:** Warn when tracked files are missing locally:

```
Warning: .env.production listed in .envsecrets but not found locally
Push remaining 2 files? [Y/n]
```

**Current behavior:** Silently skips missing files.

---

## Intentional Deviations

These are changes from the spec that improve the implementation:

### Configuration Format

| Spec Field | Implementation | Rationale |
|------------|----------------|-----------|
| `project` | Removed | GCS bucket name is sufficient; GCP project not needed |
| `passphrase` | `passphrase_env`, `passphrase_command`, `passphrase_command_args` | More secure than storing passphrase directly in config |
| `gcp_credentials` | `gcs_credentials` | Clearer naming (GCS-specific, not general GCP) |

### Added Features (not in spec)

| Feature | Description |
|---------|-------------|
| `--json` global flag | Output in JSON format for scripting |
| `doctor --fix` flag | Attempt to repair corrupted cache |
| `passphrase_command_args` | Array format for command (no shell interpolation, more secure) |

### Exit Codes

The spec defines 7 exit codes (lines 1026-1037). The implementation has 15 more granular codes:

| Code | Implementation Meaning |
|------|------------------------|
| 0 | Success |
| 1 | Not configured |
| 2 | Not in repo |
| 3 | No env files |
| 4 | Conflict |
| 5 | Decrypt failed |
| 6 | Upload failed |
| 7 | Download failed |
| 8 | Invalid config |
| 9 | GCS error |
| 10 | Git error |
| 11 | User cancelled |
| 12 | Invalid args |
| 13 | File not found |
| 14 | Permission denied |
| 99 | Unknown error |

### Storage Architecture

**Spec:** Each project in GCS is a full git repository with `.git/` subdirectory.

**Implementation:** Simpler file-based storage with encrypted `.age` files and a `HEAD` pointer file. No embedded git repos in GCS.

**Rationale:** Simpler implementation, fewer failure modes, easier to debug.

---

## Documentation Gaps

These items need to be added or corrected in documentation:

### docs/reference/configuration.md

- [ ] Add `passphrase_command_args` field documentation (preferred over `passphrase_command`)
- [ ] Mark `passphrase_command` as deprecated (uses shell execution, security risk)
- [ ] Add passphrase resolution order section:
  1. `ENVSECRETS_PASSPHRASE` environment variable (if `passphrase_env` not set)
  2. Environment variable specified by `passphrase_env`
  3. Command specified by `passphrase_command_args` (preferred)
  4. Command specified by `passphrase_command` (deprecated, shell execution)
  5. Interactive prompt (if terminal available)

### docs/reference/cli.md

- [ ] Add `doctor --fix` flag documentation
- [ ] Add `--json` global flag to global flags table (currently missing)
- [ ] Add exit codes section with all 15 codes from implementation
- [ ] Note that `--config` has no short form (spec shows `-c` but not implemented)

### docs/getting-started/configuration.md

- [ ] Add `passphrase_command_args` example:
  ```yaml
  passphrase_command_args: ["op", "read", "op://Vault/envsecrets/password"]
  ```
- [ ] Note that `passphrase_command` is deprecated in favor of `passphrase_command_args`
- [ ] Clarify that both passphrase methods cannot be used simultaneously

### docs/spec.md

- [ ] Update config format section (lines 82-96):
  - Remove `project` field
  - Remove direct `passphrase` field
  - Add `passphrase_env`, `passphrase_command`, `passphrase_command_args`
  - Rename `gcp_credentials` to `gcs_credentials`
- [ ] Update exit codes table (lines 1026-1037) to match implementation's 15 codes
- [ ] Update bucket structure diagram (lines 194-214) - no `.git/` subdirectories
- [ ] Update architecture diagram (lines 35-68) to reflect simpler storage model
- [ ] Mark all unimplemented features with "Future:" or "Not yet implemented" prefix
- [ ] Add `--json` to global flags table (line 300)
- [ ] Remove `-c` short form from `--config` flag (line 298)

### docs/index.md

- [ ] Verify installation instructions are accurate
- [ ] Verify quick example commands work as shown

### docs/reference/project-setup.md

- [ ] Remove or mark `.gitignore` marker discovery as "not yet implemented"
- [ ] Remove or mark `repo:` directive as "not yet implemented"

### docs/reference/security.md

- [ ] Update to reflect simplified storage architecture (no embedded git repos)
- [ ] Document the scrypt work factor (18) for passphrase-based encryption

### General Documentation Issues

- [ ] Ensure all code examples are tested and work
- [ ] Add troubleshooting section or FAQ
- [ ] Consider adding migration/upgrade guide for future versions

---

## Implementation Priority Suggestions

### Low Effort, High Value
1. `--quiet` flag - simple output suppression
2. `--non-interactive` flag - return error instead of prompting
3. `list --current` - reuse existing repo discovery
4. `revert --push` and `--message` - extend existing revert
5. `encode --copy` - simple clipboard integration

### Medium Effort
1. `--repo` global flag - thread through discovery
2. `pull --out` - output directory option
3. `repo:` directive in `.envsecrets` - extend parser
4. Push warning for missing files - add confirmation prompt

### Higher Effort
1. `.gitignore` marker discovery - new parser, fallback logic
2. Pull interactive conflict handling - new UI flow
3. Revert interactive mode - new UI flow
4. Enhanced `list` output with timestamps/sizes - requires GCS metadata queries
