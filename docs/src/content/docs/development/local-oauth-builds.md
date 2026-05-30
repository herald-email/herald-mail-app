---
title: Local OAuth Builds
description: Embed Google OAuth defaults in local source builds without committing credentials.
---

Herald's Google OAuth paths use desktop OAuth application credentials before they can open a browser consent flow. Release binaries include those defaults, while source builds can either provide them at runtime or embed local development defaults during `make build`.

## What The Credentials Do

The same Google OAuth client ID and client secret are used for Gmail OAuth and Google Calendar OAuth. These are desktop application defaults that let Herald start the local browser authorization flow; the user consent screen and saved refresh token still control account access.

OAuth client secrets embedded in desktop binaries are convenience defaults, not private secrets. Treat real values carefully in your shell history and local files, but do not rely on the embedded secret as a security boundary.

## Normal Development Builds

Plain `make build` reads `.herald-dev.env` when it exists. If both Google OAuth variables are present, the built `bin/herald` binary gets those values as OAuth defaults; if either value is missing, the build still succeeds and OAuth setup later reports `Google OAuth credentials are not configured`.

```sh
cp .herald-dev.env.example .herald-dev.env
$EDITOR .herald-dev.env
make build
./bin/herald
```

Use the same variable names in `.herald-dev.env`:

```sh
HERALD_GOOGLE_CLIENT_ID=your-dev-client-id.apps.googleusercontent.com
HERALD_GOOGLE_CLIENT_SECRET=your-dev-client-secret
```

`.herald-dev.env` is ignored by git and must not be committed. The general `.env` file is also ignored, but the Makefile does not read it for OAuth build defaults.

## Runtime-Only Credentials

For a one-off source run, you can avoid embedding credentials and export them in the same shell that launches Herald. Runtime environment variables override any defaults that were compiled into the binary.

```sh
make build
export HERALD_GOOGLE_CLIENT_ID="your-client-id.apps.googleusercontent.com"
export HERALD_GOOGLE_CLIENT_SECRET="your-client-secret"
./bin/herald -config ~/.herald/conf.yaml
```

`go install github.com/herald-email/herald-mail-app/cmd/herald@latest` does not read `.herald-dev.env` because it bypasses the repository Makefile. Use runtime credentials with that install path, or build from a checkout with `make build`.

## Release-Style Local Builds

`make build-release-local` is intentionally stricter than `make build`. It reads `.herald-release.env`, fails before compiling if either Google OAuth value is missing, and builds all local binaries with release-style OAuth defaults.

```sh
cp .herald-release.env.example .herald-release.env
$EDITOR .herald-release.env
make build-release-local
```

Use `.herald-release.env` when you want to test the local release packaging contract. Normal development should use `.herald-dev.env`.

## Custom Env File Paths

The Makefile paths can be overridden for tests and local automation:

```sh
DEV_ENV=/private/tmp/herald-dev.env make build
RELEASE_ENV=/private/tmp/herald-release.env make build-release-local
```

In release mode, the release env file is the source of build defaults; a dev env file does not satisfy `make build-release-local`.

## Troubleshooting

If OAuth fails before showing an authorization URL, confirm which binary you are running and how it was built. A source binary built without `.herald-dev.env`, Makefile variable values, or runtime environment variables will show `Google OAuth credentials are not configured`.

If you changed `.herald-dev.env` after building, run `make build` again. Build-time defaults are copied into the binary, so editing the env file does not change an already-built `bin/herald`.

If `make build-release-local` fails with missing credentials, fill `.herald-release.env` or set `RELEASE_ENV` to a file containing the two same-name `HERALD_GOOGLE_*` variables. `.herald-dev.env` is ignored for that target.
