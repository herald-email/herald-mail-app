# Config-Specific Cache Paths

This spec defines how Herald chooses the local SQLite cache database for each config file. It prevents separate accounts or local test configs from accidentally sharing one `email_cache.db`.

## Intended Behavior

These rules describe the observable cache-path behavior shared by all local Herald entrypoints. They keep cache ownership explicit while preserving compatibility for configs that already name a database.

- [x] A YAML-provided cache database path is authoritative and is used by all local cache readers and writers.
- [x] When the YAML has no cache database path, Herald generates one under `herald/cached/` using the config filename stem.
- [x] If the generated default path already exists, Herald appends a date plus short random suffix so a second config with the same filename does not reuse the first database.
- [x] Herald writes the generated database path back to the YAML config before opening the SQLite cache.
- [x] TUI, SSH app mode, daemon mode, and MCP cache reads all resolve the cache path through the same config behavior.
