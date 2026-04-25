---
title: Uninstall
description: Remove Herald binaries, config, cache files, and logs.
---

Herald does not install a background service unless you explicitly run or configure one. For a local checkout, uninstalling usually means deleting build outputs and local data.

## Remove build outputs

From the repository root:

```sh
make clean
```

Or remove binaries manually:

```sh
rm -rf bin/
```

## Remove config

Default config:

```sh
rm -f ~/.herald/conf.yaml
```

If you used custom config files, remove those paths too.

## Remove cache files

Check `cache.database_path` in your config before deleting it. Generated cache files usually live under `herald/cached/` relative to the directory where Herald was run.

```sh
rm -rf herald/cached/
```

If you used a custom cache path, remove that file directly.

## Remove logs

macOS:

```sh
rm -rf ~/Library/Logs/Herald
```

Linux/BSD:

```sh
rm -rf "${XDG_STATE_HOME:-$HOME/.local/state}/herald/logs"
```

Windows PowerShell:

```powershell
Remove-Item -Recurse -Force "$env:LOCALAPPDATA\Herald\Logs"
```

## Remove MCP client config

If you added Herald as an MCP server to Claude Code, Cursor, Windsurf, Codex, or another client, remove that client-side entry separately. Herald cannot remove tool registrations from external clients automatically.
