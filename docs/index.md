# envsecrets

CLI tool for managing encrypted environment files using GCS and age encryption.

## Overview

envsecrets provides a secure way to manage environment files across teams and machines. Files are encrypted using age encryption and stored in Google Cloud Storage with full version history via git.

## Features

- **Age encryption**: Industry-standard encryption using [age](https://age-encryption.org/)
- **GCS storage**: Reliable cloud storage with built-in redundancy
- **Version history**: Full git history for all environment files
- **Team sharing**: Share encrypted files via GCS bucket access
- **Conflict detection**: Warns when local and remote versions diverge

## How it Works

1. Environment files listed in `.envsecrets` are tracked
2. On `push`, files are encrypted with age and committed to a local git cache
3. The encrypted cache is synced to GCS
4. On `pull`, the cache is synced from GCS and files are decrypted to your project

## Installation

```bash
go install github.com/charliek/envsecrets/cmd/envsecrets@latest
```

## Requirements

- Go 1.24 or later
- Google Cloud Storage bucket
- GCS service account with Storage Object Admin permissions
