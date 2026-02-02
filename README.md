# envsecrets

[![CI](https://github.com/charliek/envsecrets/actions/workflows/ci.yml/badge.svg)](https://github.com/charliek/envsecrets/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/charliek/envsecrets)](https://goreportcard.com/report/github.com/charliek/envsecrets)

CLI tool for managing encrypted environment files using GCS and age encryption.

## Features

- **Secure storage**: Environment files encrypted with age and stored in Google Cloud Storage
- **Version history**: Git-based versioning for all environment files
- **Team sharing**: Share encrypted environment files across your team via GCS
- **Simple workflow**: Push/pull workflow similar to git

## Installation

```bash
go install github.com/charliek/envsecrets/cmd/envsecrets@latest
```

Or build from source:

```bash
git clone https://github.com/charliek/envsecrets.git
cd envsecrets
make install
```

## Quick Start

1. Initialize configuration:

```bash
envsecrets init
```

2. Create a `.envsecrets` file in your project listing files to track:

```
.env
.env.local
```

3. Push your environment files:

```bash
envsecrets push -m "Initial commit"
```

4. Pull environment files on another machine:

```bash
envsecrets pull
```

## Documentation

Full documentation is available at [https://charliek.github.io/envsecrets](https://charliek.github.io/envsecrets)

## Configuration

Configuration is stored in `~/.envsecrets/config.yaml`:

```yaml
bucket: your-gcs-bucket
passphrase_env: ENVSECRETS_PASSPHRASE  # or use passphrase_command
```

## License

MIT
