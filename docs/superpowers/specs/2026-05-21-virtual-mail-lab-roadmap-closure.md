# Virtual Mail Lab Roadmap Closure

This spec closes the roadmap started by `reports/PROCESS_REPORT_2026-05-20_codex-archive-and-virtual-mail-lab.md`. It records what shipped in the v1 virtual-lab work, what remains deferred, and which intentionally omitted items should not be rediscovered as accidental gaps.

## Source Artifacts

The original process report remains in `reports/` because it is a private, gitignored evidence area. This tracked closure spec is the durable index for future agents and maintainers, while `engineering/testplans/VIRTUAL_MAIL_LAB_COVERAGE.md` is the coverage matrix for scenarios and verification lanes.

- [x] `reports/PROCESS_REPORT_2026-05-20_codex-archive-and-virtual-mail-lab.md` remains the source audit and recommendation report.
- [x] `engineering/testplans/VIRTUAL_MAIL_LAB_COVERAGE.md` records scenario-to-surface coverage and focused commands.
- [x] `internal/testmail/README.md` remains the local developer guide for using and adding sanitized scenarios.

## Recommendation Checklist

The report recommendations are considered closed when they are implemented, explicitly deferred, or intentionally out of scope for v1. Status words are lowercase by design so repo assertions can check that this closure artifact keeps all three states visible: implemented, deferred, and intentionally out of scope.

- [x] Verification budget guidance - implemented in `AGENTS.md`, `CLAUDE.md`, and Herald workflow skills.
- [x] Second-failure rule - implemented in agent guidance, TUI skill guidance, and the report template.
- [x] Degradation check - implemented in Herald autopilot guidance and report template fields.
- [x] No silent scope substitution - implemented in agent guidance.
- [x] Superpowers micro-mode/throttle - implemented in agent guidance and Herald autopilot guidance.
- [x] Verification surface field - implemented in `engineering/testplans/REPORT_TEMPLATE.md`.
- [x] Tracked report template - implemented under `engineering/testplans/`.
- [x] Two-account virtual IMAP/SMTP lab - implemented in `internal/testmail` with Alice and Bob default accounts.
- [x] Compatibility wrapper for old IMAP tests - implemented through the existing test utility compatibility path.
- [x] Sanitized corpus workflow - implemented with quarantine guidance, committed `.eml` fixtures, and `tools/testmail-sanitize`.
- [x] Scenario helpers - implemented with named `ScenarioName` constants, `LoadScenario`, and `StartScenario`.
- [x] Realistic scenario catalog - implemented for plain thread, calendar invite, newsletter table, receipt HTML, malformed charset, inline CID image, remote HTML image, long-link HTML, and unsubscribe-header variants.
- [x] Backend coverage - implemented through virtual-lab `LocalBackend` send, draft, reply, and body-fetch tests.
- [x] Rendering and TUI coverage - implemented through in-process Bubble Tea preview tests at realistic sizes.
- [x] SSH coverage - implemented through a real loopback Wish SSH server backed by virtual-lab scenarios.
- [x] MCP read coverage - implemented with cache-backed virtual-lab MCP tests.
- [x] Daemon and MCP mutation coverage - implemented for send, drafts, replies, forwards, attachments, mailbox mutations, bulk mutations, unsubscribe, and sender cleanup.
- [x] Terminal raster evidence hardening - implemented with inline-CID Go assertions and the ttyd image harness contract tests.
- [x] Key-hint contract hardening - implemented in existing app keyboard and custom keymap tests that compare handlers, bottom hints, help, and remapped keys.
- [x] Public docs for boundaries - implemented across demo, MCP, daemon, SSH, troubleshooting, and screenshot guidance.
- [ ] Live provider checks - deferred because virtual lab intentionally proves deterministic behavior before provider-specific manual checks.
- [ ] Developer-facing virtual-lab runner or `herald --testmail-*` CLI - intentionally out of scope for v1.
- [ ] Demo-mode merge for realistic fixtures - intentionally out of scope for v1 because demo remains curated and presentation-friendly.
- [ ] Long-running virtual-lab process runner for ttyd or native terminal pixels - deferred; v1 keeps browser pixel evidence demo-backed and MIME behavior Go-backed.

## V1 Boundary

The v1 virtual lab is test-only by design. That keeps realistic regression fixtures private-safe, deterministic, and isolated from user-facing demo or live-mail behavior.

- [x] No public Go API, CLI flag, daemon route, MCP wire shape, or demo-mode behavior was added for virtual-lab scenarios.
- [x] `herald --testmail-*` is intentionally absent; adding it would require a separate v2 design for lifecycle, fixture selection, safety, and user-facing support.
- [x] Demo mode remains the polished explanatory lane; virtual lab remains the ugly realistic regression lane; live config remains the provider-specific lane.

## Future Work

Future work should start only after checking the coverage matrix so the next slice does not repeat already-closed work. The next likely v2 candidates are developer ergonomics rather than missing core confidence.

- [ ] Decide whether a developer-only virtual-lab runner is worth exposing outside Go tests.
- [ ] Add provider-specific live checks only when a bug depends on OAuth, bridge behavior, folder naming, throttling, or production IMAP/SMTP quirks.
- [ ] Add new sanitized scenarios only when a real bug shape cannot be represented by the current corpus.
