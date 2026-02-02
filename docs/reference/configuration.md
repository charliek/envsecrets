# Configuration Reference

Complete reference for envsecrets configuration.

## Config File Location

Default: `~/.envsecrets/config.yaml`

Override with `--config` flag or `ENVSECRETS_CONFIG` environment variable.

## Full Configuration

```yaml
# Required: GCS bucket name
bucket: my-envsecrets-bucket

# Passphrase: one of these is required
passphrase_env: ENVSECRETS_PASSPHRASE
passphrase_command: op read "op://Vault/envsecrets/password"

# Optional: Base64-encoded GCS service account JSON
# If not set, uses Application Default Credentials
gcs_credentials: eyJ0eXBlIjoic2VydmljZ...
```

## Field Reference

### bucket

**Required.** The GCS bucket name for storing encrypted files.

```yaml
bucket: my-company-envsecrets
```

### passphrase_env

Environment variable containing the encryption passphrase.

```yaml
passphrase_env: ENVSECRETS_PASSPHRASE
```

### passphrase_command

Command to execute to retrieve the passphrase. Stdout is used as the passphrase.

```yaml
passphrase_command: op read "op://Vault/envsecrets/password"
```

### gcs_credentials

Base64-encoded GCS service account JSON. Generate with `envsecrets encode`.

```yaml
gcs_credentials: eyJ0eXBlIjoic2VydmljZ...
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ENVSECRETS_CONFIG` | Override config file path |
| `ENVSECRETS_PASSPHRASE` | Default passphrase environment variable |

## Cache Directory

Encrypted files are cached at `~/.envsecrets/cache/{owner}/{repo}/`.

The cache contains:

- `.git/` - Git repository metadata
- `*.age` - Encrypted environment files
