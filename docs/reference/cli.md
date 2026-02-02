# CLI Reference

Complete reference for all envsecrets commands.

## Global Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to config file (default: `~/.envsecrets/config.yaml`) |
| `-v, --verbose` | Enable verbose output |
| `--json` | Output in JSON format |

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

### pull

Download and decrypt environment files.

```bash
envsecrets pull [flags]
```

| Flag | Description |
|------|-------------|
| `--ref` | Pull specific version (commit hash) |
| `--force` | Overwrite local files without confirmation |

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
envsecrets revert <ref>
```

### list

List repositories or files in the bucket.

```bash
envsecrets list [repo]
```

Without arguments, lists all repositories. With a repo name, lists files in that repo.

### rm

Remove a file from tracking.

```bash
envsecrets rm <file>
```

### delete

Delete an entire repository from GCS.

```bash
envsecrets delete <repo>
```

Requires confirmation.

### rotate-passphrase

Re-encrypt all repositories with a new passphrase.

```bash
envsecrets rotate-passphrase
```

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

### doctor

Verify configuration and connectivity.

```bash
envsecrets doctor
```

### completion

Generate shell completions.

```bash
envsecrets completion bash
envsecrets completion zsh
envsecrets completion fish
```
