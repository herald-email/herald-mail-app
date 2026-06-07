# Two-Candidate Worktree Trial

Use this reference only for an explicit GEPA improvement pass that asks to explore `two-candidate-worktree-trial`. Normal Herald bug and feature runs remain single-candidate unless the user requests this trial or a later workflow version makes it default.

## Purpose

The trial tests whether a small challenger worktree improves handoff quality without disrupting the stable branch-plus-report contract. It is meant to compare two implementation approaches on the same bounded task, not to run a broad autonomous search.

## Entry Conditions

- The user has explicitly approved or requested `two-candidate-worktree-trial`.
- The task is small enough that two focused implementations fit the verification budget.
- Product truth and degradation-review inputs are shared before either candidate edits files.
- No other active agent depends on changing the same files in the same checkout.
- The run can stop at one selected branch/worktree/report without pushing or merging.

## Candidate Layout

- Create one parent run folder under `.superpowers/autopilot/runs/<run-id>/`.
- Create two sibling worktrees under `.worktrees/<run-id>-candidate-a-<slug>` and `.worktrees/<run-id>-candidate-b-<slug>`.
- Use branches named `codex/autopilot-<slug>-<timestamp>-a` and `codex/autopilot-<slug>-<timestamp>-b`.
- Keep shared intake, product-truth, degradation-review, and verification budget in the parent run folder.
- Record candidate-specific notes and evidence under `candidates.a` and `candidates.b` metadata, leaving existing top-level run fields unchanged.

## Trial Procedure

1. Write one shared acceptance contract before candidate work begins.
2. Define a clear difference between candidates, such as minimal patch versus helper extraction, or docs-only guidance versus helper-backed guidance.
3. Implement candidate A and candidate B independently without copying unreviewed diffs between them.
4. Run the same focused verification commands for both candidates, then any candidate-specific checks required by the approach.
5. Compare on user-visible correctness, regression protection, implementation simplicity, retry count, verification cost, and report clarity.
6. Select exactly one candidate for handoff and mark the other as archived or discarded in the report.
7. Preserve both evidence sets long enough for review, but do not merge both branches.

## Selection Rules

- Prefer the simpler candidate when both pass the same acceptance contract.
- Prefer the candidate with clearer evidence when implementation quality is otherwise close.
- Do not select a candidate that skipped a required gate merely because it is faster.
- If both candidates fail, record the trial as failed and continue with the narrower diagnostic rather than inventing a third candidate inside the same trial.
- If the candidates expose a product ambiguity, stop and ask the user instead of choosing by implementation convenience.

## Report Requirements

The final report must include:

- the two candidate branch and worktree paths
- the shared acceptance contract
- the commands run for each candidate
- the selected candidate and why
- the discarded candidate and what was learned from it
- the exact commands for reviewing or cleaning up both worktrees

## Out Of Scope

- automatic prompt mutation
- automatic merge, push, or PR creation
- more than two candidates
- changing existing `run.json` field meanings
- using candidate comparison for large, underspecified, or high-risk product work without user confirmation
