# Herald GEPA Phase Impact

This document measures the current effect of the first four Herald GEPA improvements using the durable run corpus. It is meant to answer whether retries, skipped gates, and clarification load are trending down before we add more autonomous workflow behavior.

## Summary

- Generated at: 2026-05-28T17:36:34+00:00
- Baseline runs before Phase 1: 68
- Runs after Phase 1 started: 111
- Real bug/feature runs after Phase 1 started: 106
- Pending approval items: 0

## Window Metrics

### Baseline before Phase 1

- Runs: 68
- Average retries: 0.47
- Average skipped gates: 0.18
- Human follow-up rate: 0.04
- Average questions asked: 0.00
- Average clarification touches: 0.04

### Phase 1 window

- Runs: 2
- Average retries: 0.00
- Average skipped gates: 0.00
- Human follow-up rate: 0.00
- Average questions asked: 0.00
- Average clarification touches: 0.00

### Phase 2 window

- Runs: 2
- Average retries: 0.00
- Average skipped gates: 0.00
- Human follow-up rate: 0.00
- Average questions asked: 0.00
- Average clarification touches: 0.00

### Phase 3 window

- Runs: 0
- Average retries: 0.00
- Average skipped gates: 0.00
- Human follow-up rate: 0.00
- Average questions asked: 0.00
- Average clarification touches: 0.00

### Phase 4 window

- Runs: 107
- Average retries: 0.13
- Average skipped gates: 0.09
- Human follow-up rate: 0.01
- Average questions asked: 0.00
- Average clarification touches: 0.01

## Current Vs Baseline

- Baseline average retries: 0.47
- Current average retries: 0.13
- Retry delta: -0.34
- Baseline average skipped gates: 0.18
- Current average skipped gates: 0.09
- Skipped gate delta: -0.09
- Baseline clarification touches: 0.04
- Current clarification touches: 0.01
- Clarification delta: -0.04

## Real Task Evidence

- Baseline real-task runs: 64
- Post-Phase 1 real-task runs: 106
- Baseline real-task average retries: 0.50
- Post-Phase 1 real-task average retries: 0.13

## Pending Approval Queue

- Total items: 10
- Pending: 0
- Approved: 0
- Implemented: 10
- Published runs analyzed: 44

## Findings

- Average retries dropped from 0.47 before Phase 1 to 0.13 across post-Phase 1 runs.
- Skipped verification gates fell from 0.18 per run to 0.09.
- Clarification load dropped from 0.04 touches per run to 0.01.
- Real-task evidence includes 106 post-Phase 1 bug/feature run(s) compared with 64 baseline run(s).
- Phase 4 has 107 measured post-implementation run(s), so queue visibility can now be compared alongside run-level metrics.

## Caveats

- This measurement is observational, not causal proof. The phase windows are small and some windows may contain synthetic validation runs alongside real task runs.
- Phase 4 visibility can be measured immediately through the queue, but its effect on retries or clarification load depends on future published runs.
