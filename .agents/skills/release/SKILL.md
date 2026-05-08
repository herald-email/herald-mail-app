---
name: release
description: Herald beta release workflow for requests to cut, tag, publish, or repair a Herald release. Use when Codex should read RELEASE.md, review changes since the last tag, recommend the next v0.X.Y-beta.Z version, create and push an annotated tag, monitor CI/CD, fix release blockers, move or supersede the tag when appropriate, and write release notes.
---

# Release

Use this skill to publish Herald beta releases. The authoritative runbook is `RELEASE.md`; read it at the start of every release and let it override stale instructions here.

## Required Reads

Read these before acting:

- `RELEASE.md`
- `.github/workflows/release.yml`
- `.github/scripts/release-channel.sh`

Also inspect `README.md` or product docs when the change summary needs user-facing release notes.

## Release Contract

1. Release only from a clean `main` checkout unless the user explicitly gives a different tag source.
2. Fetch tags before deciding the next version.
3. Review every meaningful change between the last version tag and `HEAD` before recommending a version.
4. Present the recommended version and rationale before creating or pushing a tag, unless the user has already supplied an exact tag and asked to proceed.
5. Use annotated tags in the exact shape `v0.X.Y-beta.Z` with message `Herald v0.X.Y-beta.Z`.
6. Treat immutable beta tags as public once release assets are published or testers may have downloaded them. In that case, publish a new beta tag instead of moving the old one.
7. Do not manually create, edit, or push `beta-latest`; the GitHub Actions release workflow owns it.

## Review Changes

Sync the local view:

```bash
git fetch origin main --tags
git switch main
git pull --ff-only origin main
git status --short
```

Find the previous version tag and review the delta:

```bash
git tag --list 'v[0-9]*' --sort=-v:refname | head -20
last_tag="$(git tag --list 'v[0-9]*' --sort=-v:refname | head -1)"
git log --first-parent --oneline "${last_tag}..HEAD"
git log --oneline "${last_tag}..HEAD"
git diff --stat "${last_tag}..HEAD"
```

Summarize the changes by user impact:

- user-visible features or workflow changes
- bug fixes and regressions repaired
- TUI, SSH, MCP, release, packaging, docs, and test changes
- breaking or risky behavior changes
- release-only fixes after a failed tag

If there is no previous tag, use the first-release baseline from `RELEASE.md`.

## Recommend Version

For `v0.X.Y-beta.Z`:

- Bump `Z` when publishing another beta for the same intended version line, especially after release-pipeline fixes or validation fixes for a beta that should stay on the same `0.X.Y` scope.
- Bump `Y` and reset `Z` to `1` for patch-level app fixes on an existing minor line, for example `v0.2.0-beta.4` to `v0.2.1-beta.1`.
- Bump `X`, reset `Y` to `0`, and reset `Z` to `1` for new user-facing capability, major TUI workflow changes, broad architecture changes, or a release train that deserves a new minor line.
- Keep the major version at `0` until the user explicitly starts stable-major release planning.

Show the recommendation as:

```text
Recommended tag: v0.X.Y-beta.Z
Why: <short rationale tied to the change review>
Alternative: <next safest alternative if the choice is debatable>
```

## Pre-Tag Verification

Run the checks required by `RELEASE.md`:

```bash
git status --short
make test
make vet
```

If release scripts changed, also run the focused tests or shell checks for those scripts. If these fail, fix the blocker on `main`, commit it, and rerun the failing command before tagging.

## Tag And Push

Create and push the annotated beta tag from `main`:

```bash
tag="v0.X.Y-beta.Z"
git tag -a "$tag" -m "Herald $tag"
git push origin "$tag"
```

Record the tag, commit SHA, and push result.

## Watch CI/CD

Find and watch the tag-triggered release run:

```bash
gh run list --workflow Release --limit 10
gh run watch <run-id> --exit-status
```

If it fails:

```bash
gh run view <run-id> --log-failed
```

Classify the failure:

- code/test bug: fix on `main`, commit, rerun local verification, then move or supersede the beta tag.
- missing secret or repository setup: report the exact secret/setup blocker from the logs; do not invent credentials.
- transient infrastructure failure: rerun the workflow if GitHub supports it and no code/tag change is needed.
- artifact, Homebrew, or `beta-latest` failure: inspect logs and `RELEASE.md`, fix the smallest release-path issue, then rerun or republish as appropriate.

Repeat the fix, verify, tag, push, and watch loop until the release succeeds or a non-code blocker remains.

## Moving Or Superseding A Failed Tag

If the failed tag did not publish release assets and is not likely to have been consumed, it is acceptable to move the same beta tag after telling the user what happened:

```bash
git tag -d "$tag"
git push origin ":refs/tags/$tag"
git tag -a "$tag" -m "Herald $tag"
git push origin "$tag"
```

If assets were published, a GitHub release exists for the tag, or external testers may have consumed it, leave that immutable tag alone and publish the next beta, usually by bumping `Z`.

Never force-move `beta-latest`.

## Post-Publish Verification

After the workflow succeeds, follow the asset, `beta-latest`, and Homebrew verification sections in `RELEASE.md` as far as the current machine can support. At minimum:

```bash
gh release view "$tag"
gh release view beta-latest
```

Download and checksum artifacts when feasible. Note any hardware-specific verification that was skipped, such as Intel smoke checks from Apple Silicon.

## Release Notes

Write concise user-facing release notes after the successful release run. Prefer a local notes file under `reports/`, for example:

```bash
mkdir -p reports
$EDITOR "reports/release-notes-${tag}.md"
```

Use this structure:

```markdown
# Herald v0.X.Y-beta.Z

## Highlights

- ...

## Fixes

- ...
```

Keep verification details, hardware-specific skips, and infrastructure warnings out of the public release notes. Report those in the final response instead.

If GitHub's generated notes need curation, update the versioned release with the notes file:

```bash
gh release edit "$tag" --notes-file "reports/release-notes-${tag}.md"
```

Do not edit `beta-latest` notes manually unless the release workflow failed to update the mutable channel and the user approves the manual repair.

## Final Response

Report:

- final tag and commit SHA
- version recommendation rationale
- local verification commands
- GitHub Actions run URL and conclusion
- asset, beta channel, and Homebrew checks performed
- release notes path and whether GitHub release notes were edited
- any skipped hardware or credential-dependent checks
