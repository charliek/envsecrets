# Quick Start

Get up and running with envsecrets in a few minutes.

## Prerequisites

- Google Cloud Storage bucket
- Service account with Storage Object Admin permissions
- Service account JSON key file

## 1. Install envsecrets

```bash
go install github.com/charliek/envsecrets/cmd/envsecrets@latest
```

## 2. Initialize Configuration

Run the interactive setup:

```bash
envsecrets init
```

This creates `~/.envsecrets/config.yaml` with your settings.

## 3. Set Up Your Project

Create a `.envsecrets` file in your project root listing files to track:

```text
.env
.env.local
config/secrets.yaml
```

Add these files to your `.gitignore`:

```gitignore
.env
.env.local
config/secrets.yaml
```

## 4. Push Environment Files

```bash
envsecrets push -m "Initial environment setup"
```

## 5. Pull on Another Machine

After setting up envsecrets on another machine:

```bash
cd your-project
envsecrets pull
```

## Next Steps

- [Configuration](configuration.md) - Detailed configuration options
- [CLI Reference](../reference/cli.md) - Full command documentation
