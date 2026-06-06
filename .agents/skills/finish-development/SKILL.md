---
name: finish-development
description: Herald-specific completion workflow for requests such as "finish-development", "nice" as a repo completion command, "wrap this branch", "fast merge", "fastpass", "quick merge", "fast mode", or "commit, merge to main, delete worktree". Use when a completed Herald task should be polished or fast-passed, committed, merged into main, and its feature worktree cleaned up.
---

# Finish Development

Use this skill as the Herald shorthand for: nice, commit, merge to `main`, delete the completed worktree. Keep the workflow local unless the user explicitly asks to push.

## Contract

1. Preserve unrelated user changes. Do not stage, commit, merge, delete, or overwrite files that are outside the completed task.
2. Treat "nice" as the final polish pass: review the diff, run formatting, run appropriate verification, and fix anything small and clearly in scope before committing.
3. Treat "fast merge", "fastpass", "quick merge", "quickpass", "fast mode", or similar no-tests merge phrasing as Fast Mode: skip tests only if the main-branch sync and merge are mechanically clean and the change-interference review is boring.
4. Ask before destructive or externally visible actions not already implied by this skill, especially pushing to a remote, force operations, deleting an unmerged branch, or removing a non-`.worktrees` checkout. Closing a GitHub issue is implied only when the thread was clearly about that issue and the Issue Closure section below applies.
5. Prefer non-interactive git commands. Avoid interactive rebases, commit editors, and merge editors.
6. If a conflict, failing verification, or ambiguous dirty worktree appears, stop with the exact blocker and the safest next option.

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
- issue numbers that clearly define the completed task, from the user request, current thread title/context, a GitHub issue URL, or an unambiguous branch name such as `issue-123` or `product-polish-123`

If the current checkout is already `main`, adapt the workflow: commit on `main` only if that is clearly what the user requested, skip merge, and never remove the repository root worktree.

For any candidate issue number, verify it belongs to the current remote before relying on it:

```bash
gh issue view <number> --json number,title,state,url
```

Do not infer issue intent from arbitrary numbers in filenames, dates, versions, test names, or commit hashes.

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

## Fast Mode

Use this mode only when the user explicitly asks for a faster no-tests merge path with wording like "fast merge", "fastpass", "quick merge", "quickpass", "fast finish", "fast mode", or "merge without testing".

Fast Mode replaces the Nice Pass verification commands, but not the safety review. Before committing, still run:

```bash
git diff --stat
git diff --name-only
git diff --check
```

Commit the intended files using the normal Commit section. If a commit hook would run tests and the user explicitly asked for no tests, commit with `--no-verify` and report that in the final response.

Then make sure local `main` is current and sync it into the feature branch before merging back:

```bash
git -C <main-checkout> fetch origin main
git -C <main-checkout> pull --ff-only origin main
feature_branch="$(git branch --show-current)"
base="$(git merge-base HEAD main)"
git diff --name-only "$base"..main
git merge --no-edit main
```

Inspect the changed-file list from the task and from the main sync. Proceed without tests only when all of these are true:

- the local `main` update was a clean fast-forward or already up to date
- the merge from `main` completed with no conflicts
- any merge commit created by syncing `main` is expected and contains only the clean main sync
- the main sync did not change the same files, package boundaries, test fixtures, build scripts, generated assets, schemas, or docs sections that the task changed
- there are no unrelated dirty files that would be staged, committed, overwritten, or removed
- `git diff --check` and `git diff --cached --check` pass
- the later merge to `main` can be fast-forwarded cleanly

If any item is uncertain, leave Fast Mode and stop with the reason or run the normal Nice Pass verification before continuing. In Fast Mode, skip `make fmt`, `make test`, `make vet`, TUI captures, SSH smoke, and MCP smoke.

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

For issue-backed work that fully completes the requested issue, prefer a closing keyword in the task commit body:

```text
Closes #<issue-number>
```

Use `Refs #<issue-number>` only when the branch is a partial slice, the issue is related but not completed, or the workflow is intentionally stopping at branch handoff without merging the completed work to the default branch. If a task commit was already created with `Refs` but the work is now being merged as complete, amend the commit or create the final merge/default-branch commit with `Closes #<issue-number>` before any push.

## Merge To Main

After the feature branch has a clean, verified commit, or a Fast Mode commit that passed the no-interference review:

```bash
feature_branch="$(git branch --show-current)"
git fetch origin main
git switch main
git pull --ff-only origin main
```

If `main` has moved since the feature branch started, rebase the feature branch onto `main` in the feature worktree, rerun affected verification, then return to `main`. In Fast Mode, use the Fast Mode sync and interference rules instead; do not rerun tests unless the review is uncertain or the user asks.

Prefer a fast-forward merge:

```bash
git merge --ff-only "$feature_branch"
```

If fast-forward is impossible, inspect why. Use a normal merge only when it is clearly safe and the user wanted a local merge; otherwise surface the branch divergence and ask.

Do not push `main` unless the user explicitly asks.

## Issue Closure

If the completed task clearly came from one or more GitHub issues:

1. Confirm the completed issue has a closing reference (`Closes #<number>`, `Fixes #<number>`, or `Resolves #<number>`) in the final task/default-branch history. Use `Refs` only for partial or related work.
2. After the final commit exists and any requested local merge to `main` has succeeded, collect the final commit hash and subject:

   ```bash
   commit_hash="$(git rev-parse HEAD)"
   commit_subject="$(git log -1 --pretty=%s)"
   ```

3. Close each verified issue with a comment that references the commit whenever the issue is still open after local completion, especially when the workflow did not push `main`:

   ```bash
   gh issue close <number> --comment "Completed in commit ${commit_hash} (${commit_subject})."
   ```

If the issue is already closed, add no duplicate closure comment unless the user explicitly asks for a handoff note. If `gh issue close` fails because of authentication, network, or permissions, leave the issue open and report the exact command and error in the final response.

Do not close issues for broad roadmap work, partially completed issue slices, speculative references, or issue numbers mentioned only as related context. When multiple issues are present, close only the issues whose requested acceptance criteria were actually completed by the final commit.

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
- when Fast Mode was used, the merge-from-main result, the no-interference review result, and the verification commands intentionally skipped
- any issue numbers closed, with the closing commit hash, or why issue closure was skipped
- removed worktree path and deleted branch
- a copy-paste "How To Test This Change" block with `cd <main-checkout>`, the build command, and either `./bin/herald <parameters>` or the full path to the built test binary
- any skipped step and why
