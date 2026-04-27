# Herald Release Runbook

This runbook covers the first beta release path. The first beta tag is `v0.1.0-beta.1`, and tag pushes matching `v*` publish GitHub releases with downloadable macOS artifacts.

## Release Scope

- GitHub-hosted releases only for the first beta.
- macOS artifacts only: `darwin/arm64` and `darwin/amd64`.
- Each artifact includes `herald`, `herald-mcp-server`, and `herald-ssh-server`.
- Stable releases are immutable `v*` tags without a hyphen, such as `v0.1.0`, and own GitHub's `/releases/latest` URL.
- Beta releases are immutable `v*-beta.*` prereleases and also update the mutable `beta-latest` convenience channel.
- The Homebrew tap `herald-email/homebrew-herald` is the recommended macOS install path: `brew tap herald-email/herald && brew install herald`.
- DMG packaging, code signing, and notarization are deferred to a later packaging milestone.
- Built-in Google OAuth credentials are convenience defaults for desktop OAuth. They are not confidential once distributed in a binary.

## Required GitHub Secrets

Release builds require these repository secrets:

```bash
gh auth login

gh secret set HERALD_GOOGLE_CLIENT_ID \
  --repo herald-email/herald-mail-app \
  --body "your-client-id.apps.googleusercontent.com"

gh secret set HERALD_GOOGLE_CLIENT_SECRET \
  --repo herald-email/herald-mail-app \
  --body "your-client-secret"
```

The release workflow also updates the separate Homebrew tap repository. Create a fine-grained token or classic PAT with write access to `herald-email/homebrew-herald`, then save it on the app repository:

```bash
gh secret set TAP_GITHUB_TOKEN \
  --repo herald-email/herald-mail-app \
  --body "github-token-with-homebrew-tap-write-access"
```

You can also use the GitHub web UI: repository `Settings` -> `Secrets and variables` -> `Actions` -> `New repository secret`.

The release workflow fails before building if either OAuth secret is missing, and fails after GitHub release publication with a clear error if `TAP_GITHUB_TOKEN` is missing. Runtime environment variables with the same names still override built-in defaults for local testing. Desktop OAuth client secrets bundled into release binaries are convenience defaults, not confidential once distributed.

## Homebrew Install

Homebrew installs the same release-built binaries as the GitHub tarballs, including the desktop OAuth convenience defaults embedded by the release workflow. Gmail OAuth remains experimental in first-run onboarding and is shown only when launched with `-experimental`.

```bash
brew tap herald-email/herald
brew install herald
herald --version
```

The Homebrew formula is generated from immutable versioned release assets and checksums. It must not point at the mutable `beta-latest` release assets.

## Local OAuth-Enabled Builds

For local builds that behave like release binaries:

```bash
cp .herald-release.env.example .herald-release.env
$EDITOR .herald-release.env
make build-release-local
./bin/herald --version
```

`.herald-release.env` is ignored by git. Do not commit real OAuth credentials.

Plain `make build` does not embed OAuth defaults from `.herald-release.env`; it builds a normal development binary. If you run `make build && ./bin/herald -experimental` and choose Gmail OAuth without exported runtime credentials, Herald will fail with `Google OAuth credentials are not configured`.

Use one of these local paths instead:

```bash
# Embed defaults into the local binary.
make build-release-local
./bin/herald -config ~/.herald/conf.yaml

# Or keep a development binary and provide credentials at runtime.
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
make build
./bin/herald -config ~/.herald/conf.yaml
```

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

The `Release` workflow runs tests, builds both macOS architectures, smokes all three binaries, uploads checksums, publishes a GitHub prerelease, and updates `herald-email/homebrew-herald`.

It also updates `beta-latest` when the tag matches `v*-beta.*`. Maintainers should not manually create, push, or edit the `beta-latest` tag or release; the workflow owns that mutable channel.

## Release Channels

Canonical releases are always immutable version tags:

- Stable: `https://github.com/herald-email/herald-mail-app/releases/latest`
- Current beta channel: `https://github.com/herald-email/herald-mail-app/releases/tag/beta-latest`
- Immutable beta: `https://github.com/herald-email/herald-mail-app/releases/tag/v0.1.0-beta.N`

Channel behavior:

- `v0.1.0` publishes as stable, is not a prerelease, and becomes GitHub's latest release.
- `v0.1.0-beta.1` publishes as a prerelease, does not become GitHub latest, and updates `beta-latest`.
- Future tags such as `v0.2.0-rc.1` publish as prereleases but do not update `beta-latest`.

The `beta-latest` release uses stable asset names:

- `herald-beta-latest-darwin-arm64.tar.gz`
- `herald-beta-latest-darwin-amd64.tar.gz`
- `checksums-beta-latest.txt`
- `beta-latest.json`

The binaries inside those tarballs still report the immutable tag, such as `v0.1.0-beta.2`, from `--version`.

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

For the mutable beta channel, verify:

```bash
gh release view beta-latest
gh release download beta-latest --pattern 'herald-beta-latest-darwin-arm64.tar.gz'
gh release download beta-latest --pattern 'checksums-beta-latest.txt'
shasum -a 256 -c checksums-beta-latest.txt
```

Verify the Homebrew tap:

```bash
brew update
brew tap herald-email/herald
brew audit --strict --online --formula herald-email/herald/herald
brew install herald-email/herald/herald
herald --version
herald-mcp-server --version
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' \
  | herald-mcp-server --demo
```

Run the Homebrew install verification on Apple Silicon for every release. Repeat on Intel hardware or Rosetta before announcing Intel support for a release.

## Rollback

If the tag workflow publishes a bad prerelease:

1. Mark the GitHub release as a draft or delete the prerelease from GitHub.
2. Delete the bad remote tag only if no one should consume it:
   `git push origin :refs/tags/v0.1.0-beta.1`
3. Delete the local tag:
   `git tag -d v0.1.0-beta.1`
4. Fix the issue and publish a new beta tag such as `v0.1.0-beta.2`.

Prefer a new beta tag once any external tester may have downloaded the old artifact.

If `beta-latest` points at a bad beta, publish a fixed immutable beta tag. The workflow will move `beta-latest` forward. Avoid manually rolling `beta-latest` backward unless the bad release must be hidden immediately.

## Future Packaging Milestone

- Keep the `herald-email/homebrew-herald` formula tracking immutable release tarballs.
- Decide whether to add a cask once a signed and notarized DMG exists.
- Add Apple Developer ID signing and notarization before DMG/cask distribution.
- Consider GoReleaser when the release matrix expands beyond the current macOS beta artifacts.

References:

- [GitHub Actions secrets](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets)
- [GitHub CLI secret setup](https://cli.github.com/manual/gh_secret_set)
- [Google OAuth for desktop apps](https://developers.google.com/identity/protocols/oauth2/native-app)
- [Homebrew tap guidance](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
