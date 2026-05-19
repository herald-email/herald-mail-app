# Preview Cache Follow-ups

This document captures the next fixes and product decisions after the preview telemetry and offline cache policy work. The real mailbox measurements showed the new preview cache is fast when populated, but first opens still depend on IMAP unless a background process warms the preview cache ahead of time.

## Current Measurements

These measurements came from the local Herald logs on 2026-05-15 after testing against the real Gmail mailbox. They matter because they separate cache-hit behavior from IMAP behavior instead of treating "preview load" as one undifferentiated problem.

- [x] Cache hits are effectively instant: 92 cached preview loads had p50 `1ms`, average `0.8ms`, and max `11ms`.
- [x] Uncached IMAP preview loads are still visible to humans: 13 IMAP preview loads had p50 `1057ms`, average `1541ms`, and max `6460ms`.
- [x] The current implementation stores durable preview rows in `email_preview_bodies`; the tested cache had 31 rows after the real mailbox runs.
- [x] No stale or failed preview results appeared in the measured run.

## Background Preview Prewarmer

Herald now has a dedicated background worker that fills `email_preview_bodies` before the user opens a message. The live default-account run on 2026-05-15 showed the worker makes recent previews land as cache hits while keeping IMAP work serialized.

- [x] Add a preview prewarmer that runs after the active folder finishes metadata sync and warms the newest or visible messages first.
- [x] Respect `cache.storage_policy` when warming previews: `lightweight` should fetch preview text and useful headers only, `no_attachments` should avoid downloadable attachment bytes, and `preserve_all` should fetch full message data only after explicit opt-in.
- [x] Keep IMAP pressure conservative by using one preview fetch at a time per account and by cancelling or deprioritizing work when the user changes folders.
- [x] Expose progress in logs and optionally the status bar, for example `Preview cache: 12/50 warmed`.
- [x] Add tests that prove prewarming skips already cached messages, stops on folder switch, and does not download attachment bytes under `lightweight` or `no_attachments`.
- [x] Verify against the live default account: the prewarmer processed 50 INBOX candidates, warmed 19 missing previews, skipped 31 cached previews, and a subsequent 12-message manual preview pass loaded every message from cache in 8-21ms.

## Policy Changes And Cache Pruning

The cache policy now controls future writes, but users can change their mind after richer data has already been stored. A privacy-sensitive setting needs a way to remove data that no longer matches the selected policy.

- [x] When changing from `preserve_all` to `no_attachments` or `lightweight`, remove stored attachment bytes from preview rows and any future full-message cache table.
- [x] When changing from `no_attachments` to `lightweight`, remove inline image bytes and keep only text, HTML, headers, and attachment metadata.
- [x] Add a settings action that explains and performs "reclaim offline cache storage" with a before/after byte estimate.
- [x] Add regression tests for policy downgrade cleanup so private attachment bytes cannot linger silently.

## Attachment Offline Behavior

Attachment save is now lazy unless the selected policy has the bytes locally. That is safer by default, but the app should make the attachment state clearer and make full offline attachment support explicit.

- [ ] Show whether an attachment is local or will be fetched on save, using a compact marker that does not widen the preview layout.
- [ ] Let `preserve_all` prewarm attachment bytes only for explicitly selected folders or bounded recent windows, because all-mail full attachment caching can grow quickly.
- [ ] Include `part_path` in attachment-facing APIs and tool descriptions so callers do not have to rely only on filenames, which can be duplicated within a message.
- [ ] Add telemetry for attachment saves: local hit versus IMAP fetch, byte size, duration, and error.

## Search And FTS Reliability

The real run logged `no such module: fts5`, which means this build could not create SQLite FTS tables. That is separate from preview latency, but it affects body search quality and makes the cache feel less capable than it should.

- [ ] Update build flags or dependency setup so local development and release builds include SQLite FTS5 support.
- [ ] Add a startup health check that records whether FTS is enabled and shows a clear warning in debug logs when Herald falls back to non-FTS search.
- [ ] Add a test or CI build job that verifies `CREATE VIRTUAL TABLE ... USING fts5` succeeds in the shipped binary configuration.
- [ ] Consider `sqlite-vec` or another SQLite-native vector extension before introducing a separate vector database.

## Telemetry Polish

The new telemetry is useful, but cache hits can show `duration=0s` in logs because sub-millisecond durations round down. That makes the cache look magical but less measurable.

- [ ] Log a numeric `duration_ms` field in addition to the display duration so scripts can parse timings without unit conversion.
- [ ] Format sub-millisecond cache hits as `<1ms` or `1ms` instead of `0s` in human-readable logs.
- [ ] Add source-specific summaries to debug logs when the app exits, such as cache hit count, IMAP miss count, p50, p95, and max.
- [ ] Save the mailbox measurement command as a small script under `scripts/` so future regressions can be compared consistently.

## Settings And First-run UX

The settings form now exposes the offline cache policy, but the first-run experience should explain the tradeoff in plain language. Users should understand that "cache" can mean metadata, preview text, or full attachment data.

- [x] In the install wizard, use compact policy labels for lightweight previews, message bodies without attachments, and full offline archives.
- [x] Default new configs to message bodies without attachments so background prewarming supports offline reading without caching downloadable attachment bytes.
- [ ] Show the current preview cache footprint and row count in settings.
- [ ] Add a manual "warm current folder previews" command for users who want control without enabling broad background work.

## Database Direction

SQLite remains the right primary store for Herald because this is a local-first, single-user mailbox cache. The current problem was not that SQLite was too slow; it was that preview bodies were not being stored durably before this branch.

- [ ] Keep SQLite as the system of record for metadata, preview bodies, settings, classifications, contacts, rules, and offline cache policy.
- [ ] Avoid CockroachDB for the local app because it adds distributed-system complexity without addressing preview latency.
- [ ] Avoid replacing SQLite with ChromaDB; consider a vector sidecar only if semantic search outgrows SQLite-native options.
- [ ] Revisit Postgres only if Herald becomes a multi-user server product rather than a local terminal client.
