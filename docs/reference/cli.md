# CLI Reference

Complete reference for all envsecrets commands.

## Global Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to config file (default: `~/.envsecrets/config.yaml`) |
| `-r, --repo` | Override repository identifier (format: `owner/name`) |
| `-v, --verbose` | Enable verbose output |
| `--json` | Output in JSON format (for scripting) |
| `--non-interactive` | Disable interactive prompts (for CI/CD) |

## Commands

### init

Create or update configuration interactively.

```bash
envsecrets init
```

### status

Show repository info and file status.

```bash
envsecrets status
```

### push

Encrypt and upload environment files.

```bash
envsecrets push [flags]
```

| Flag | Description |
|------|-------------|
| `-m, --message` | Commit message |
| `--dry-run` | Show what would be pushed without pushing |
| `--force` | Force push even with conflicts |
| `--allow-missing` | Allow push with missing tracked files (for non-interactive mode) |

### pull

Download and decrypt environment files.

```bash
envsecrets pull [flags]
```

| Flag | Description |
|------|-------------|
| `--ref` | Pull specific version (commit hash) |
| `--force` | Overwrite local files without confirmation |
| `--dry-run` | Show what would be pulled without pulling |
| `--skip-conflicts` | Skip conflicting files instead of aborting |

When conflicts exist (local files differ from remote), pull will prompt per-file:
- `[o]verwrite` - Replace local file with remote version
- `[s]kip` - Keep local file unchanged
- `[a]bort` - Cancel the entire pull operation

Use `--force` to overwrite all conflicts, or `--skip-conflicts` to skip all conflicts without prompting.

### log

Show commit history.

```bash
envsecrets log [flags]
```

| Flag | Description |
|------|-------------|
| `-n` | Number of commits to show (default: 10) |
| `-v, --verbose` | Show file changes in each commit |

### diff

Show changes between versions.

```bash
envsecrets diff [ref1] [ref2]
```

If no refs provided, shows diff between local and latest remote.

### revert

Restore files from a previous version.

```bash
envsecrets revert [ref]
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be reverted without reverting |
| `-p, --push` | Push reverted state as new commit |
| `-m, --message` | Commit message (used with --push) |
| `-y, --yes` | Skip confirmation prompt |

If no ref is provided in interactive mode, you can pick from recent commits. In non-interactive mode, a ref is required.

### list

List repositories or files in the bucket.

```bash
envsecrets list [repo]
```

| Flag | Description |
|------|-------------|
| `--current` | List files in auto-detected current repository |

Without arguments, lists all repositories. With a repo name, lists files in that repo.
With `--current`, auto-detects the current repository from git remote.

### rm

Remove a file from tracking.

```bash
envsecrets rm <file>
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be removed without removing |
| `-y, --yes` | Skip confirmation prompt |

### delete

Delete an entire repository from GCS.

```bash
envsecrets delete <repo>
```

| Flag | Description |
|------|-------------|
| `--yes-delete-permanently` | Confirm deletion in non-interactive mode |
| `--dry-run` | Show what would be deleted without deleting |

Requires confirmation in interactive mode.

### rotate-passphrase

Re-encrypt all repositories with a new passphrase.

```bash
envsecrets rotate-passphrase
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be rotated without rotating |

### verify

Test decryption across all repositories.

```bash
envsecrets verify
```

### encode

Base64 encode a service account JSON file.

```bash
envsecrets encode <path>
```

| Flag | Description |
|------|-------------|
| `--copy` | Copy to clipboard (macOS/Linux) |

### doctor

Verify configuration and connectivity.

```bash
envsecrets doctor [flags]
```

| Flag | Description |
|------|-------------|
| `--fix` | Attempt to repair corrupted cache |

The `--fix` flag will:
- Remove corrupted cache directories
- Re-initialize git repositories in the cache
- Clear orphaned lock files

### completion

Generate shell completions.

```bash
envsecrets completion bash
envsecrets completion zsh
envsecrets completion fish
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Not configured (config file missing or invalid) |
| 2 | Not in a git repository |
| 3 | No .envsecrets file found or no files tracked |
| 4 | Conflict between local and remote |
| 5 | Decryption failed (wrong passphrase or corrupted data) |
| 6 | Upload failed |
| 7 | Download failed |
| 8 | Invalid configuration |
| 9 | GCS error |
| 10 | Git error |
| 11 | User cancelled operation |
| 12 | Invalid arguments |
| 13 | File or repository not found |
| 14 | Permission denied |
| 99 | Unknown error |
