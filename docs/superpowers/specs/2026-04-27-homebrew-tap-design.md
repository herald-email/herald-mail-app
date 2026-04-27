# Homebrew Tap Design

## Goal

This section records the packaging decision for the first Homebrew install path. The goal is to make macOS installation feel native for terminal users while preserving the release-built OAuth defaults already embedded in GitHub release binaries.

- [x] Create a public `herald-email/homebrew-herald` tap so users can run `brew tap herald-email/herald && brew install herald`.
- [x] Use immutable GitHub release tarballs rather than mutable `beta-latest` assets.
- [x] Install `herald`, `herald-mcp-server`, and `herald-ssh-server` from the selected release artifact.

## Approach

This section captures the implementation shape and the tradeoff behind it. A binary formula is the right first step because a source-built formula would omit release-only Google OAuth defaults unless users supplied build-time credentials.

- [x] Generate `Formula/herald.rb` from `dist/checksums.txt` after release artifacts are published.
- [x] Select Apple Silicon and Intel tarballs with Homebrew `on_arm` and `on_intel` blocks.
- [x] Keep cask, DMG packaging, Developer ID signing, and notarization out of this first Homebrew pass.

## Release Automation

This section describes how the tap stays current without manual formula edits. The app repository release workflow owns formula generation, while the tap repository stores the generated formula that Homebrew users consume.

- [x] Add `.github/scripts/render-homebrew-formula.sh` to render the formula from a tag and checksum file.
- [x] Add a release workflow step that clones `herald-email/homebrew-herald`, updates `Formula/herald.rb`, commits, and pushes.
- [x] Require `TAP_GITHUB_TOKEN` with write access to the tap repository and fail clearly if it is absent.

## Verification

This section defines the acceptance checks for the tap and release automation. The checks cover formula generation, Homebrew audit, local install, and runtime smoke tests for all installed commands.

- [x] Unit-test formula rendering, including checksum mapping and missing-checksum failures.
- [x] Run `brew audit --strict --online --formula herald-email/herald/herald`.
- [x] Run a local Apple Silicon Homebrew install and smoke `herald`, `herald-mcp-server`, `herald-ssh-server`, and MCP `tools/list`.
- [ ] Repeat the install smoke on Intel hardware or Rosetta before announcing Intel release support.

## User Instructions

This section records the installed-user operations that should appear in the public docs. Homebrew is the default macOS install path, so users should have clear update, upgrade, and reset commands without needing to inspect the tap repository.

- [x] Document `brew tap herald-email/herald && brew install herald` as the default macOS install.
- [x] Document `brew update` and `brew upgrade herald` for routine upgrades.
- [x] Document uninstall, untap, retap, and reinstall as the full reset path.
