---
title: Themes
description: Browse Herald's built-in themes, launch a one-off theme from the CLI, and regenerate docs screenshots.
---

Herald can inherit your terminal colors, use built-in app palettes, or load local YAML themes. The built-in catalog is useful for quickly choosing a readable mailbox style without editing config, while the YAML path remains available for personal or shared theme files.

## Quick Launch

Use `-theme` with either a built-in theme name or a local theme YAML file. The flag changes only the current session and does not save to `~/.herald/conf.yaml`.

```sh
./bin/herald --demo -theme jade-signal
./bin/herald --demo -theme ./my-theme.yaml
./bin/herald -config ~/.herald/conf.yaml -theme amber-furnace
```

## Timeline Gallery

These screenshots are generated from demo mode so the gallery can be refreshed without a real mailbox. Each image proves that the named theme resolves through the app theme system and can render the Timeline surface.

| Red / Warm | Green / Earth |
| --- | --- |
| ![Red Black theme](/screenshots/themes/red-black.png)<br />`red-black` | ![Jade Signal theme](/screenshots/themes/jade-signal.png)<br />`jade-signal` |
| ![Crimson theme](/screenshots/themes/crimson.png)<br />`crimson` | ![Viridian Glass theme](/screenshots/themes/viridian-glass.png)<br />`viridian-glass` |
| ![Ember theme](/screenshots/themes/ember.png)<br />`ember` | ![Forest CRT theme](/screenshots/themes/forest-crt.png)<br />`forest-crt` |
| ![Ruby Noir theme](/screenshots/themes/ruby-noir.png)<br />`ruby-noir` | ![Pine Mail theme](/screenshots/themes/pine-mail.png)<br />`pine-mail` |
| ![Garnet Console theme](/screenshots/themes/garnet-console.png)<br />`garnet-console` | ![Olive Circuit theme](/screenshots/themes/olive-circuit.png)<br />`olive-circuit` |
| ![Amber Furnace theme](/screenshots/themes/amber-furnace.png)<br />`amber-furnace` | ![Sepia Debug theme](/screenshots/themes/sepia-debug.png)<br />`sepia-debug` |
| ![Copper Ash theme](/screenshots/themes/copper-ash.png)<br />`copper-ash` | ![Solar Paper theme](/screenshots/themes/solar-paper.png)<br />`solar-paper` |
| ![Magma Core theme](/screenshots/themes/magma-core.png)<br />`magma-core` | ![Zenbones Light theme](/screenshots/themes/zenbones-light.png)<br />`zenbones-light` |

| Blue / Violet | Diverse Terminal Palettes |
| --- | --- |
| ![Peacock Ink theme](/screenshots/themes/peacock-ink.png)<br />`peacock-ink` | ![Ayu Courier theme](/screenshots/themes/ayu-courier.png)<br />`ayu-courier` |
| ![Ultramarine Desk theme](/screenshots/themes/ultramarine-desk.png)<br />`ultramarine-desk` | ![Cobalt Dispatch theme](/screenshots/themes/cobalt-dispatch.png)<br />`cobalt-dispatch` |
| ![Amethyst Night theme](/screenshots/themes/amethyst-night.png)<br />`amethyst-night` | ![Kanagawa Post theme](/screenshots/themes/kanagawa-post.png)<br />`kanagawa-post` |
| ![Graphite Rose theme](/screenshots/themes/graphite-rose.png)<br />`graphite-rose` | ![Rose Pine Desk theme](/screenshots/themes/rose-pine-desk.png)<br />`rose-pine-desk` |
| ![Arctic Signal theme](/screenshots/themes/arctic-signal.png)<br />`arctic-signal` | ![Tokyo Dusk theme](/screenshots/themes/tokyo-dusk.png)<br />`tokyo-dusk` |
| ![Iceberg Queue theme](/screenshots/themes/iceberg-queue.png)<br />`iceberg-queue` | ![Panda Packet theme](/screenshots/themes/panda-packet.png)<br />`panda-packet` |
| ![Sonokai Signal theme](/screenshots/themes/sonokai-signal.png)<br />`sonokai-signal` | ![Tomorrow Desk theme](/screenshots/themes/tomorrow-desk.png)<br />`tomorrow-desk` |

## Open Preview Gallery

These screenshots open the first demo email before capture. Use them to check preview header contrast, body readability, and selected-row colors side by side.

| Red / Warm | Green / Earth |
| --- | --- |
| ![Red Black preview theme](/screenshots/themes/preview/red-black.png)<br />`red-black` | ![Jade Signal preview theme](/screenshots/themes/preview/jade-signal.png)<br />`jade-signal` |
| ![Crimson preview theme](/screenshots/themes/preview/crimson.png)<br />`crimson` | ![Viridian Glass preview theme](/screenshots/themes/preview/viridian-glass.png)<br />`viridian-glass` |
| ![Ember preview theme](/screenshots/themes/preview/ember.png)<br />`ember` | ![Forest CRT preview theme](/screenshots/themes/preview/forest-crt.png)<br />`forest-crt` |
| ![Ruby Noir preview theme](/screenshots/themes/preview/ruby-noir.png)<br />`ruby-noir` | ![Pine Mail preview theme](/screenshots/themes/preview/pine-mail.png)<br />`pine-mail` |
| ![Garnet Console preview theme](/screenshots/themes/preview/garnet-console.png)<br />`garnet-console` | ![Olive Circuit preview theme](/screenshots/themes/preview/olive-circuit.png)<br />`olive-circuit` |
| ![Amber Furnace preview theme](/screenshots/themes/preview/amber-furnace.png)<br />`amber-furnace` | ![Sepia Debug preview theme](/screenshots/themes/preview/sepia-debug.png)<br />`sepia-debug` |
| ![Copper Ash preview theme](/screenshots/themes/preview/copper-ash.png)<br />`copper-ash` | ![Solar Paper preview theme](/screenshots/themes/preview/solar-paper.png)<br />`solar-paper` |
| ![Magma Core preview theme](/screenshots/themes/preview/magma-core.png)<br />`magma-core` | ![Zenbones Light preview theme](/screenshots/themes/preview/zenbones-light.png)<br />`zenbones-light` |

| Blue / Violet | Diverse Terminal Palettes |
| --- | --- |
| ![Peacock Ink preview theme](/screenshots/themes/preview/peacock-ink.png)<br />`peacock-ink` | ![Ayu Courier preview theme](/screenshots/themes/preview/ayu-courier.png)<br />`ayu-courier` |
| ![Ultramarine Desk preview theme](/screenshots/themes/preview/ultramarine-desk.png)<br />`ultramarine-desk` | ![Cobalt Dispatch preview theme](/screenshots/themes/preview/cobalt-dispatch.png)<br />`cobalt-dispatch` |
| ![Amethyst Night preview theme](/screenshots/themes/preview/amethyst-night.png)<br />`amethyst-night` | ![Kanagawa Post preview theme](/screenshots/themes/preview/kanagawa-post.png)<br />`kanagawa-post` |
| ![Graphite Rose preview theme](/screenshots/themes/preview/graphite-rose.png)<br />`graphite-rose` | ![Rose Pine Desk preview theme](/screenshots/themes/preview/rose-pine-desk.png)<br />`rose-pine-desk` |
| ![Arctic Signal preview theme](/screenshots/themes/preview/arctic-signal.png)<br />`arctic-signal` | ![Tokyo Dusk preview theme](/screenshots/themes/preview/tokyo-dusk.png)<br />`tokyo-dusk` |
| ![Iceberg Queue preview theme](/screenshots/themes/preview/iceberg-queue.png)<br />`iceberg-queue` | ![Panda Packet preview theme](/screenshots/themes/preview/panda-packet.png)<br />`panda-packet` |
| ![Sonokai Signal preview theme](/screenshots/themes/preview/sonokai-signal.png)<br />`sonokai-signal` | ![Tomorrow Desk preview theme](/screenshots/themes/preview/tomorrow-desk.png)<br />`tomorrow-desk` |

## Regenerate Screenshots

Run the screenshot script from the repository root after changing visible UI layout or theme roles:

```sh
scripts/regenerate-theme-screenshots.sh
```

Pass one or more theme names to refresh only those screenshots:

```sh
scripts/regenerate-theme-screenshots.sh jade-signal solar-paper
```

The script builds Herald, starts tmux demo sessions with `--demo -theme <name>`, dismisses the welcome overlay, and writes PNGs plus `.ansi.txt` sidecars to `docs/public/screenshots/themes/`.

Set `HERALD_THEME_SCREENSHOT_VIEW=preview` to open the first demo email before capture and write PNGs to `docs/public/screenshots/themes/preview/`:

```sh
HERALD_THEME_SCREENSHOT_VIEW=preview scripts/regenerate-theme-screenshots.sh
```

## Custom Themes

Settings can still install local YAML files into `~/.herald/themes`. Config-level `theme.overrides` apply semantic role edits on top of the selected theme, so you can start from a built-in palette and tune individual roles such as `chrome.status_bar`, `focus.selection_active`, or `metadata.sender`.

## Related Pages

- [Settings](/features/settings/)
- [Config Reference](/reference/config/)
- [Demo Mode](/demo-mode/)
