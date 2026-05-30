---
title: Nightly Builds
description: Download short-lived Herald nightly builds from GitHub Actions for testing the latest main branch.
---

Nightly builds are for testers who want the latest successful `main` build before the next beta tag. They are short-lived GitHub Actions artifacts, not signed releases, not Homebrew packages, and not guaranteed to be stable.

Use Homebrew or beta releases for normal installation. Use nightlies when you are validating a fix, trying fresh behavior, or helping test the next release.

## Download from GitHub

The Nightly workflow publishes macOS artifacts after tests, vet, package builds, and binary smoke checks pass. Pick the artifact that matches your Mac.

1. Open [Actions > Nightly](https://github.com/herald-email/herald-mail-app/actions/workflows/nightly.yml).
2. Select the latest successful run.
3. Scroll to **Artifacts**.
4. Download `herald-nightly-darwin-arm64` for Apple Silicon or `herald-nightly-darwin-amd64` for Intel.
5. Unzip the downloaded artifact.
6. Optionally verify the checksum file.
7. Extract the included `.tar.gz`.

```sh
shasum -a 256 -c *.sha256
tar -xzf herald-nightly-*.tar.gz
```

The extracted folder contains `herald`, `herald-mcp-server`, and `herald-ssh-server`.

## Download with GitHub CLI

The GitHub CLI is the quickest path if you already have repository access configured. The example downloads the latest Apple Silicon artifact.

```sh
run_id="$(gh run list --repo herald-email/herald-mail-app --workflow Nightly --limit 1 --json databaseId --jq '.[0].databaseId')"
gh run download "$run_id" --repo herald-email/herald-mail-app --name herald-nightly-darwin-arm64
```

For Intel Macs, use `herald-nightly-darwin-amd64` as the artifact name.

## Nightly Channel Rules

Nightly builds are disposable test-channel builds. They use separate Google OAuth defaults from beta and stable release builds, and artifacts are retained for 14 days.

- They do not create Git tags.
- They do not update `beta-latest`.
- They do not update Homebrew.
- They do not publish GitHub Releases.
- Their binary version looks like `nightly-YYYYMMDD-<commit>`.

## macOS Warning

Nightly binaries are direct downloads from GitHub Actions. They are not Developer ID signed or notarized yet, so macOS Gatekeeper may warn before the packaging milestone adds signing.
