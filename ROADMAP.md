# envsecrets Roadmap

## Future Enhancements

### Multi-Passphrase Support
Allow different repositories in the same bucket to use different passphrases. The `rotate-passphrase` command would need to handle this by prompting per-repo or accepting a mapping.

### Improved Diff Algorithm
Replace the simple set-based diff with a proper line-by-line diff that preserves order and shows context (similar to `git diff`).

### Progress Indicators
Add progress bars or spinners for long-running operations like push/pull with many files.

### Remove Legacy passphrase_command
After sufficient deprecation period (v2.0), remove shell-based `passphrase_command` in favor of the safer `passphrase_command_args`.
