# Configuration Reference

Complete reference for envsecrets configuration.

## Config File Location

Default: `~/.envsecrets/config.yaml`

Override with `--config` flag or `ENVSECRETS_CONFIG` environment variable.

## Full Configuration

```yaml
# Required: GCS bucket name
bucket: my-envsecrets-bucket

# Passphrase: configure one of these methods
passphrase_env: ENVSECRETS_PASSPHRASE
passphrase_command_args: ["op", "read", "op://Vault/envsecrets/password"]

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

### passphrase_command_args

**Preferred method.** Command and arguments to execute to retrieve the passphrase. Stdout is used as the passphrase.

```yaml
passphrase_command_args: ["op", "read", "op://Vault/envsecrets/password"]
```

This method executes the command directly without shell interpolation, which is more secure.

Examples:

```yaml
# 1Password CLI
passphrase_command_args: ["op", "read", "op://Vault/envsecrets/password"]

# AWS Secrets Manager
passphrase_command_args: ["aws", "secretsmanager", "get-secret-value", "--secret-id", "envsecrets", "--query", "SecretString", "--output", "text"]

# HashiCorp Vault
passphrase_command_args: ["vault", "kv", "get", "-field=password", "secret/envsecrets"]

# macOS Keychain
passphrase_command_args: ["security", "find-generic-password", "-s", "envsecrets", "-w"]
```

### gcs_credentials

Base64-encoded GCS service account JSON. Generate with `envsecrets encode`.

```yaml
gcs_credentials: eyJ0eXBlIjoic2VydmljZ...
```

If not set, envsecrets uses Application Default Credentials (ADC).

## Passphrase Resolution Order

When envsecrets needs the passphrase, it tries these sources in order:

1. **Environment variable** - If `passphrase_env` is set, read from that environment variable
2. **Command args** - If `passphrase_command_args` is set, execute the command
3. **Interactive prompt** - If running in a terminal, prompt the user

The first successful method is used. If all methods fail, the operation fails with an error.

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
