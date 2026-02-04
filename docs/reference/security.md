# Security

Security model and considerations for envsecrets.

## Encryption

envsecrets uses [age](https://age-encryption.org/) for encryption, a modern and audited encryption tool.

- **Algorithm**: ChaCha20-Poly1305 with scrypt key derivation
- **Key derivation**: scrypt with N=2^18, r=8, p=1
- **No metadata leakage**: File names are preserved but contents are fully encrypted

## Data Flow

```text
Local Files → Age Encryption → Local Git Cache → GCS
                  ↑
            Passphrase
```

1. Plaintext files exist only in your project directory
2. Files are encrypted with age before being written to the local cache
3. The local cache (`~/.envsecrets/cache/`) is a git repository containing only encrypted `.age` files
4. Encrypted files are synced to GCS (simple file storage, no git repos in GCS)

**Note:** Version history is maintained in the local git cache. GCS stores only the encrypted files and a `HEAD` pointer to the latest commit. This simpler architecture has fewer failure modes than storing full git repositories in cloud storage.

## Passphrase Security

The passphrase is the only secret needed to decrypt your files.

### Best Practices

- Use a strong, unique passphrase (20+ characters recommended)
- Store the passphrase in a password manager
- Use `passphrase_command_args` to retrieve from a secure source (see [Configuration](configuration.md))
- Never commit the passphrase to git

!!! note "Passphrase Recovery"
    If you lose the passphrase, the encrypted files cannot be recovered. Keep backups of:

    - The passphrase itself
    - Unencrypted copies of critical files

## GCS Security

### Bucket Access

- Restrict bucket access to authorized service accounts
- Use IAM roles, not ACLs
- Enable bucket versioning for additional protection
- Consider enabling object retention policies

### Service Account

The service account needs only `Storage Object Admin` on the specific bucket:

```bash
gcloud storage buckets add-iam-policy-binding gs://my-bucket \
  --member=serviceAccount:envsecrets@project.iam.gserviceaccount.com \
  --role=roles/storage.objectAdmin
```

### Credentials Storage

The `gcs_credentials` field in the config file is base64-encoded, not encrypted. Protect the config file:

```bash
chmod 600 ~/.envsecrets/config.yaml
```

## Threat Model

### Protected Against

- Unauthorized GCS access (files are encrypted)
- Accidental git commits (files are in .gitignore)
- Network interception (GCS uses TLS)
- Local cache exposure (cache contains only encrypted files)

!!! warning "Not Protected Against"
    - Passphrase compromise
    - Compromise of a machine with decrypted files
    - Malicious team members with passphrase access

## Audit

### Verify Encryption

Confirm files in the cache are encrypted:

```bash
file ~/.envsecrets/cache/owner/repo/.env.age
# Should show: data (not: ASCII text)
```

### Verify Connectivity

```bash
envsecrets doctor
```

### Test Decryption

```bash
envsecrets verify
```
