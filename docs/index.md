# envsecrets

CLI tool for managing encrypted environment files using GCS and age encryption.

## Overview

envsecrets provides a secure way to manage environment files across teams and machines. Files are encrypted using age encryption and stored in Google Cloud Storage with full version history via git.

## Features

- **Age encryption**: Industry-standard encryption using [age](https://age-encryption.org/)
- **GCS storage**: Reliable cloud storage with built-in redundancy
- **Version history**: Full git history for all environment files
- **Team sharing**: Share encrypted files via GCS bucket access
- **Multi-machine sync clarity**: a per-machine "last synced" baseline lets `status` give an unambiguous "do this next" recommendation (push / pull / reconcile / in sync) and powers a `sync` command that runs the safe action automatically. Push refuses to silently overwrite changes another machine made.
- **Per-commit attribution**: every push is stamped with the OS user and machine, so `log` and `status` show who pushed what from which machine.

## How it Works

1. Environment files listed in `.envsecrets` are tracked
2. On `push`, files are encrypted with age and committed to a local git cache
3. The encrypted cache is synced to GCS
4. On `pull`, the cache is synced from GCS and files are decrypted to your project
5. A local marker (`LAST_SYNCED`) records each machine's most recent sync point, used to drive 3-way diffs (working tree vs baseline vs remote) so `status` can tell you what to do next without having to remember which machine you last pushed from

## Installation

```bash
go install github.com/charliek/envsecrets/cmd/envsecrets@latest
```

## Requirements

- Go 1.24 or later
- Google Cloud Storage bucket
- GCS service account with Storage Object Admin permissions
