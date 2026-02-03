# Configuration

envsecrets uses a YAML configuration file at `~/.envsecrets/config.yaml`.

## Creating Configuration

Run `envsecrets init` to create configuration interactively, or create the file manually.

## Configuration File

```yaml
# GCS bucket for storing encrypted files
bucket: my-envsecrets-bucket

# Passphrase configuration (choose one method)
passphrase_env: ENVSECRETS_PASSPHRASE
# OR
passphrase_command_args: ["op", "read", "op://Vault/envsecrets/password"]

# Base64-encoded GCS service account JSON
gcs_credentials: eyJ0eXBlIjoic2VydmljZ...
```

## Configuration Fields

| Field | Required | Description |
|-------|----------|-------------|
| `bucket` | Yes | GCS bucket name for storage |
| `passphrase_env` | One of passphrase options | Environment variable containing passphrase |
| `passphrase_command_args` | One of passphrase options | Command and arguments to retrieve passphrase |
| `gcs_credentials` | No* | Base64-encoded service account JSON |

*If `gcs_credentials` is not set, Application Default Credentials are used.

## Passphrase Options

### Environment Variable

```yaml
passphrase_env: ENVSECRETS_PASSPHRASE
```

Set the passphrase in your shell:

```bash
export ENVSECRETS_PASSPHRASE="your-secure-passphrase"
```

### Command

```yaml
passphrase_command_args: ["op", "read", "op://Vault/envsecrets/password"]
```

The command is executed and its stdout is used as the passphrase. Works with:

- 1Password CLI (`op`)
- Bitwarden CLI (`bw`)
- AWS Secrets Manager
- Any command that outputs a passphrase

Example configurations:

```yaml
# 1Password CLI
passphrase_command_args: ["op", "read", "op://Vault/envsecrets/password"]

# pass (password-store)
passphrase_command_args: ["pass", "show", "envsecrets"]

# macOS Keychain
passphrase_command_args: ["security", "find-generic-password", "-s", "envsecrets", "-w"]
```

### Interactive

If neither option is configured, you'll be prompted for the passphrase.

## GCS Credentials

### Encoding Service Account JSON

Use `envsecrets encode` to base64-encode your service account JSON:

```bash
envsecrets encode path/to/service-account.json
```

Copy the output to your config file.

### Application Default Credentials

If `gcs_credentials` is not set, envsecrets uses Application Default Credentials:

```bash
gcloud auth application-default login
```
