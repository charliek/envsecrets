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

## 6. Day-to-Day on Multiple Machines

When you've been working on more than one machine and aren't sure what state things are in, run:

```bash
envsecrets status
```

The output ends with a recommendation — one of:

- **In sync** — nothing to do
- **Run `envsecrets push`** — you have local edits to publish
- **Run `envsecrets pull`** — another machine pushed; catch up
- **Run `envsecrets pull` then `envsecrets push`** — both sides changed, but on different files
- **Reconcile** — the same file changed on two machines; resolve with `envsecrets diff <file>`, then `envsecrets pull` (interactive), then `envsecrets push`
- **Run `envsecrets pull` first** (`first_pull`) — fresh machine, post-reset, or upgraded from an older client; pull establishes the per-machine sync baseline that the recommendations rely on
- **Run `envsecrets push` to initialize** (`first_push_init`) — bucket has no entry for this repo yet; the first push creates it

To skip the manual step, run:

```bash
envsecrets sync
```

`sync` performs the recommended safe action automatically. It refuses (with an actionable message) when a manual reconcile is required.

## Next Steps

- [Configuration](configuration.md) - Detailed configuration options
- [CLI Reference](../reference/cli.md) - Full command documentation
