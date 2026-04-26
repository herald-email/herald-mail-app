# Herald Release Runbook

This runbook covers the first beta release path. The first beta tag is `v0.1.0-beta.1`, and tag pushes matching `v*` publish GitHub prereleases with downloadable macOS artifacts.

## Release Scope

- GitHub prerelease only for the first beta.
- macOS artifacts only: `darwin/arm64` and `darwin/amd64`.
- Each artifact includes `herald`, `herald-mcp-server`, and `herald-ssh-server`.
- Homebrew, DMG packaging, code signing, and notarization are deferred to the next packaging milestone.
- Built-in Google OAuth credentials are convenience defaults for desktop OAuth. They are not confidential once distributed in a binary.

## Required GitHub Secrets

Release builds require these repository secrets:

```bash
gh secret set HERALD_GOOGLE_CLIENT_ID --body "your-client-id.apps.googleusercontent.com"
gh secret set HERALD_GOOGLE_CLIENT_SECRET --body "your-client-secret"
```

The release workflow fails before building if either secret is missing. Runtime environment variables with the same names still override built-in defaults for local testing.

## Local OAuth-Enabled Builds

For local builds that behave like release binaries:

```bash
cp .herald-release.env.example .herald-release.env
$EDITOR .herald-release.env
make build-release-local
./bin/herald --version
```

`.herald-release.env` is ignored by git. Do not commit real OAuth credentials.

## First Beta Tag

Before tagging:

```bash
git status --short
make test
make vet
```

Create and push the annotated beta tag from `main`:

```bash
git tag -a v0.1.0-beta.1 -m "Herald v0.1.0-beta.1"
git push origin v0.1.0-beta.1
```

The `Release` workflow runs tests, builds both macOS architectures, smokes all three binaries, uploads checksums, and publishes a GitHub prerelease.

## Asset Verification

After the prerelease is published, download both macOS tarballs and `checksums.txt`, then verify:

```bash
shasum -a 256 -c checksums.txt
tar -tzf herald-v0.1.0-beta.1-darwin-arm64.tar.gz
tar -tzf herald-v0.1.0-beta.1-darwin-amd64.tar.gz
```

Smoke the downloaded binaries:

```bash
./herald-v0.1.0-beta.1-darwin-arm64/herald --version
./herald-v0.1.0-beta.1-darwin-arm64/herald --help
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' \
  | ./herald-v0.1.0-beta.1-darwin-arm64/herald-mcp-server --demo
./herald-v0.1.0-beta.1-darwin-arm64/herald-ssh-server --version
```

Repeat the smoke checks for the Intel tarball on Intel hardware or Rosetta.

## Rollback

If the tag workflow publishes a bad prerelease:

1. Mark the GitHub release as a draft or delete the prerelease from GitHub.
2. Delete the bad remote tag only if no one should consume it:
   `git push origin :refs/tags/v0.1.0-beta.1`
3. Delete the local tag:
   `git tag -d v0.1.0-beta.1`
4. Fix the issue and publish a new beta tag such as `v0.1.0-beta.2`.

Prefer a new beta tag once any external tester may have downloaded the old artifact.

## Future Packaging Milestone

- Create a `herald-email/homebrew-herald` tap so users can install with `brew tap herald-email/herald && brew install herald`.
- Decide whether Homebrew should install formula tarballs first or a cask once a DMG exists.
- Add Apple Developer ID signing and notarization before DMG/cask distribution.
- Consider GoReleaser when the release matrix expands beyond the current macOS beta artifacts.

References:

- [GitHub Actions secrets](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets)
- [GitHub CLI secret setup](https://cli.github.com/manual/gh_secret_set)
- [Google OAuth for desktop apps](https://developers.google.com/identity/protocols/oauth2/native-app)
- [Homebrew tap guidance](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
