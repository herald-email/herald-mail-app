---
name: finish-development
description: Herald-specific completion workflow for requests such as "finish-development", "nice" as a repo completion command, "wrap this branch", or "commit, merge to main, delete worktree". Use when a completed Herald task should be polished, verified, committed, merged into main, and its feature worktree cleaned up.
---

# Finish Development

Use this skill as the Herald shorthand for: nice, commit, merge to `main`, delete the completed worktree. Keep the workflow local unless the user explicitly asks to push.

## Contract

1. Preserve unrelated user changes. Do not stage, commit, merge, delete, or overwrite files that are outside the completed task.
2. Treat "nice" as the final polish pass: review the diff, run formatting, run appropriate verification, and fix anything small and clearly in scope before committing.
3. Ask before destructive or externally visible actions not already implied by this skill, especially pushing to a remote, force operations, deleting an unmerged branch, or removing a non-`.worktrees` checkout.
4. Prefer non-interactive git commands. Avoid interactive rebases, commit editors, and merge editors.
5. If a conflict, failing verification, or ambiguous dirty worktree appears, stop with the exact blocker and the safest next option.

## Preflight

Run these from the active Herald checkout:

```bash
git rev-parse --show-toplevel
git branch --show-current
git status --short
git worktree list --porcelain
```

Identify:

- current branch
- current worktree path
- default branch, normally `main`
- files that belong to the completed task
- unrelated dirty files that must be left alone

If the current checkout is already `main`, adapt the workflow: commit on `main` only if that is clearly what the user requested, skip merge, and never remove the repository root worktree.

## Nice Pass

Review the pending change before staging:

```bash
git diff --stat
git diff --check
```

Run the repo-standard polish and verification:

```bash
make fmt
make test
make vet
```

Route extra verification by impact:

- TUI or layout changes: follow `../tui-test/SKILL.md` and save the required report under `reports/`.
- SSH changes: build and smoke `./cmd/herald-ssh-server`.
- MCP changes: build and invoke the relevant `./cmd/herald-mcp-server` tool path.
- Release or packaging changes: include `make test`, `make vet`, and the affected release script or workflow tests.

If verification fixes are needed, make only scoped fixes, then rerun the failing command before proceeding.

## Commit

Stage only the intended files:

```bash
git add -- <intended-files>
git diff --cached --stat
git diff --cached --check
```

Create one clear commit unless the user requested multiple commits:

```bash
git commit -m "<concise imperative summary>"
```

Include issue references in the body when the completed task came from an issue. Do not include unrelated pending changes in the commit.

## Merge To Main

After the feature branch has a clean, verified commit:

```bash
feature_branch="$(git branch --show-current)"
git fetch origin main
git switch main
git pull --ff-only origin main
```

If `main` has moved since the feature branch started, rebase the feature branch onto `main` in the feature worktree, rerun affected verification, then return to `main`.

Prefer a fast-forward merge:

```bash
git merge --ff-only "$feature_branch"
```

If fast-forward is impossible, inspect why. Use a normal merge only when it is clearly safe and the user wanted a local merge; otherwise surface the branch divergence and ask.

Do not push `main` unless the user explicitly asks.

## Delete Worktree

Remove only the completed feature worktree, never the repository root checkout:

```bash
git worktree list --porcelain
git worktree remove <completed-worktree-path>
git branch -d "$feature_branch"
```

Use `git worktree prune` only for stale administrative entries, not as a substitute for understanding which checkout is being removed. If the branch is not fully merged, do not force delete it without explicit approval.

## Final Response

Report:

- commit hash and subject
- merge result on `main`
- verification commands and outcomes
- removed worktree path and deleted branch
- any skipped step and why
