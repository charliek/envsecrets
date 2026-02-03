# Project Setup

Configure envsecrets for your project.

## The .envsecrets File

Create a `.envsecrets` file in your project root listing files to track:

```
.env
.env.local
config/secrets.yaml
```

### File Format

- One file path per line
- Paths are relative to project root
- Empty lines and lines starting with `#` are ignored
- Glob patterns are not supported

### Example

```
# Environment files
.env
.env.local

# Configuration secrets
config/database.yaml
config/api-keys.yaml
```

### Repository Override

You can override the auto-detected repository identity by adding a `repo:` directive:

```
repo: myorg/custom-name

.env
.env.local
```

This is useful when:
- Your git remote URL doesn't match the desired storage path
- You're working in a fork but want to share secrets with the upstream repo
- You want a custom organization/repo structure in GCS

Priority order for repo identification:
1. `--repo` command-line flag (highest)
2. `repo:` directive in `.envsecrets`
3. Git remote URL detection (lowest)

## Alternative: .gitignore Marker

If you don't want a separate `.envsecrets` file, you can mark tracked files directly in your `.gitignore`:

```gitignore
node_modules/
dist/

# envsecrets
.env
.env.local
.env.production

# Logs
*.log
```

Everything between `# envsecrets` and the next blank line or comment section is treated as tracked files.

This approach keeps your tracked files and gitignore entries in one place.

## Project Identification

envsecrets identifies your project using the git remote URL. The remote is parsed to extract the owner and repository name:

| Remote URL | Owner | Repo |
|------------|-------|------|
| `git@github.com:acme/myapp.git` | acme | myapp |
| `https://github.com/acme/myapp.git` | acme | myapp |
| `git@gitlab.com:team/project.git` | team | project |

## .gitignore

Add tracked files to your `.gitignore` to prevent committing plaintext secrets:

```gitignore
# Environment files (managed by envsecrets)
.env
.env.local
config/secrets.yaml
```

## Workflow

1. Create `.envsecrets` listing files to track
2. Add those files to `.gitignore`
3. Create the actual environment files with your secrets
4. Run `envsecrets push` to encrypt and upload
5. Team members run `envsecrets pull` to get the files

## Multiple Environments

For projects with multiple environments, use separate files:

```
.env.development
.env.staging
.env.production
```

Or use subdirectories:

```
envs/development/.env
envs/staging/.env
envs/production/.env
```
