---
title: SSH Mode
description: Serve the Herald TUI over SSH for remote terminal access.
---

SSH mode wraps the full Herald TUI in a Charm Wish SSH server. Each SSH session can either open its own local backend connection or connect to a running Herald daemon. New setups should use the primary `herald ssh` subcommand; `herald-ssh-server` is retained only as a compatibility wrapper for older scripts.

## Install or Build

```sh
go install github.com/herald-email/herald-mail-app/cmd/herald@latest
herald ssh -config ~/.herald/conf.yaml -addr :2222
```

From a local checkout, build the same primary CLI and substitute `./bin/herald`
for `herald` in the examples:

```sh
go build -o bin/herald ./cmd/herald
./bin/herald ssh -config ~/.herald/conf.yaml -addr :2222
```

Connect from another terminal:

```sh
ssh -p 2222 localhost
```

Use a specific host key path:

```sh
herald ssh -host-key .ssh/host_ed25519
```

Use the daemon backend instead of opening IMAP per SSH session:

```sh
herald serve -config ~/.herald/conf.yaml
herald ssh -config ~/.herald/conf.yaml -daemon http://127.0.0.1:7272
```

<!-- HERALD_SCREENSHOT id="ssh-mode-session" page="ssh-mode" alt="Herald TUI inside SSH session" state="local SSH, 120x40 client terminal" desc="Shows the full Herald TUI rendered through an SSH client with normal tab bar, panels, status bar, and key hints." capture="terminal; build herald; run ./bin/herald ssh; connect with ssh -p 2222 localhost" deferred="true" reason="requires local SSH server session" -->

## User-Facing Behavior

The SSH TUI uses the same tabs, keybindings, and overlays as local Herald. The difference is process ownership: without `-daemon`, each SSH connection creates its own backend and IMAP connection; with `-daemon`, the session uses the shared remote backend.

## Security Notes

SSH mode creates or uses an SSH host key path. Bind to a local address or protect network exposure appropriately. Mail credentials still come from the Herald config file on the machine running the SSH server.

## Troubleshooting

If connection fails, verify the server is running and listening on the expected address. If the TUI opens but mail fails, check config path and provider connectivity on the server machine. If daemon mode fails, run `herald status`.

## Related Pages

- [Global UI](/using-herald/global-ui/)
- [Daemon Commands](/advanced/daemon/)
- [Privacy and Security](/security-privacy/)
- [All Keybindings](/reference/keybindings/)
