---
title: Regenerate Screenshots
description: Refresh Herald docs screenshots and theme gallery captures from repeatable demo-mode sessions.
---

Screenshots are generated from demo mode so documentation media can be refreshed without touching a real mailbox. Use these workflows after changing visible UI layout, theme roles, demo data, or docs pages that reference generated screenshots.

## Theme Screenshots

Run the theme screenshot script from the repository root after changing visible theme behavior:

```sh
scripts/regenerate-theme-screenshots.sh
```

By default, the script builds Herald, starts tmux demo sessions with `--demo -theme <name>`, dismisses the welcome overlay, captures both Timeline and Preview states, and writes PNGs plus `.ansi.txt` sidecars under `docs/public/screenshots/themes/`.

Pass one or more theme names to refresh only those screenshots:

```sh
scripts/regenerate-theme-screenshots.sh jade-signal solar-paper
```

Use `HERALD_THEME_SCREENSHOT_VIEW=timeline` or `HERALD_THEME_SCREENSHOT_VIEW=preview` for a single gallery lane:

```sh
HERALD_THEME_SCREENSHOT_VIEW=preview scripts/regenerate-theme-screenshots.sh jade-signal
```

The script requires `tmux`, `aha`, and Google Chrome for HTML-to-PNG rendering. If the binary is already built, skip the build step:

```sh
HERALD_THEME_SCREENSHOT_SKIP_BUILD=1 scripts/regenerate-theme-screenshots.sh jade-signal
```

## Demo GIFs And Broad Docs Media

Demo tapes live in `demos/*.tape`, canonical GIFs go to `assets/demo/*.gif`, docs-facing GIFs go to `docs/public/demo/*.gif`, and still screenshots go to `docs/public/screenshots/*.png`. Run media generation from the repository root because the tapes reference `./bin/herald`.

```sh
make build
make build-mcp
make docs-media
```

Most canonical tapes use VHS's `Builtin Solarized Dark` theme. Focused showcase captures use additional terminal themes where theme comparison is the point.

## Related Pages

These pages explain the user-facing theme gallery and the broader demo media workflow.

- [Themes](/themes/)
- [Demo Mode](/demo-mode/)
- [Demo GIF Workflow](/advanced/demo-gifs/)
