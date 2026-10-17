---
title: What's New in v0.9
description: External mailbox password commands and reliable account-title updates in v0.9.0-beta.1.
---

`v0.9.0-beta.1` lets you retrieve mailbox passwords from a password manager or another local command instead of keeping the password in `conf.yaml`. It also fixes account-title updates while editing Settings.

## Release Delta

These changes are new since `v0.8.0-beta.1`. Existing literal passwords continue to work without changing your configuration.

- [x] Configure `credentials.password.command` as a shell string or an argument array for IMAP and SMTP authentication. Argument arrays execute directly without a shell.
- [x] Resolve passwords only when authenticating a connection. Loading, validating, or saving configuration, demo mode, and cache-only operations do not run password commands.
- [x] Keep command definitions intact when saving configuration or editing an account with the password field left blank. Entering a new literal password replaces the command.
- [x] Bound command execution with a 30-second timeout and a 64 KiB output limit. Command text, arguments, and output are omitted from command failure messages.
- [x] Update account-edit and disconnect titles immediately as the display name changes, without asynchronous reads of the input field.

## Beta Notes

This is the first beta on the v0.9 line. Password commands run locally with Herald's environment and permissions; configure only commands you trust, and make sure your password manager is ready before connecting.

- [x] `v0.9.0-beta.1` includes external mailbox password commands requested in [#77](https://github.com/herald-email/herald-mail-app/issues/77), implemented in [#80](https://github.com/herald-email/herald-mail-app/pull/80), and the Settings title fix in [#79](https://github.com/herald-email/herald-mail-app/pull/79).
- [x] Literal passwords remain supported. Commands must print the password to standard output; trailing line endings are removed while other whitespace is preserved.
- [x] Release archives include `herald`, `herald-mcp-server`, and `herald-ssh-server` for macOS and Linux on arm64 and amd64. Linux packages require glibc 2.39 or newer.
- [x] The release workflow updates the `beta-latest` channel and Homebrew tap from the immutable versioned release.

## How To Try It

Upgrade an existing Homebrew installation, then replace the password value in your account's credentials with a command. For example, this retrieves a password from an existing `pass` entry:

```sh
brew update
brew upgrade herald
```

```yaml
credentials:
  username: "you@example.com"
  password:
    command: ["pass", "show", "mail/example"]
```

Use an entry containing only the password. For a shell command, use `command: "pass show mail/example | head -n 1"`. Start Herald normally to connect and test authentication; `herald --demo` does not run the command. See [External mailbox passwords](/reference/config/#external-mailbox-passwords) for command behavior and Settings details.

## Previous Release

The v0.8 line introduced sending identities, selected-message folder moves, native Linux packages, and connection recovery. See [What's New in v0.8](/whats-new-in-v0-8/) for that release history.
