# Herald GEPA Improvement Log

This document is the durable history of changes to the Herald autopilot workflow. It is designed to answer two questions quickly:

- Are we getting better?
- Do we have enough structured evidence to write about the approach later?

## Snapshot Table

| Logged At | Title | Status | Runs | Avg Score | Failed Runs | Frontier |
|---|---|---:|---:|---:|---:|---:|
| 2026-04-23T18:42:12 | Improvement-history logging for GEPA | applied | 4 | 85.66666666666667 | 1 | 2 |
| 2026-04-23T18:42:12 | Herald Autopilot foundation | reconstructed | 4 | 85.66666666666667 | 1 | 2 |

## Entries

### Improvement-history logging for GEPA

- Logged at: 2026-04-23T18:42:12+00:00
- Status: applied
- Kind: workflow-improvement
- Bottleneck: The workflow had a ledger and optimizer state, but no durable narrative of how the process itself improved over time.
- Summary: Added a dedicated improvement-history log that records workflow changes, metrics snapshots, deltas, article notes, and follow-ups.

Metrics at log time:
- Recent runs: 4
- Average score: 85.66666666666667
- Average retries: 0.25
- Failed runs: 1
- Frontier members: 2
Delta from previous entry:
- recent_run_count: +0
- average_score: +0.0
- average_retry_count: +0.0
- failed_run_count: +0
- frontier_count: +0
Changes:
- Added machine-readable improvement-log state under .superpowers/autopilot/state/.
- Added a rendered markdown timeline at docs/superpowers/gepa-improvement-log.md.
- Updated the GEPA-improvement workflow so every future improvement can log itself.
Recommended experiment at log time:
- `auto-ledger-and-state-sync` (high value, low risk)
Article notes:
- A good research/dev workflow needs both raw traces and a story of how the method changed.
- This log is the bridge from internal optimization to publishable methodology.
Follow-ups:
- Log each future improve-GEPA change as a separate entry with evidence and deltas.

### Herald Autopilot foundation

- Logged at: 2026-04-23T18:42:12+00:00
- Status: reconstructed
- Kind: workflow-improvement
- Bottleneck: The workflow needed durable structure before it could learn from its own runs.
- Summary: Established the initial repo-local autopilot workflow with run artifacts, scoring, reflection, reports, and the first GEPA optimizer state.

Metrics at log time:
- Recent runs: 4
- Average score: 85.66666666666667
- Average retries: 0.25
- Failed runs: 1
- Frontier members: 2
Changes:
- Created the Herald autopilot skill and run-artifact schema.
- Added scoring, reflection, report rendering, and the first optimizer state layer.
Recommended experiment at log time:
- `auto-ledger-and-state-sync` (high value, low risk)
Article notes:
- The first milestone was not autonomy for its own sake; it was making every run legible enough to learn from.
Follow-ups:
- Measure real-task behavior now that the workflow has durable state.

