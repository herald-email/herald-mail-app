---
title: Themes
description: Browse Herald's built-in themes, launch a one-off theme from the CLI, and use Jade Signal as a baseline custom theme.
---

Herald can inherit your terminal colors, use built-in app palettes, or load local YAML themes. The built-in catalog is useful for quickly choosing a readable mailbox style without editing config, while the YAML path remains available for personal or shared theme files.

## Quick Launch

Use `-theme` with either a built-in theme name or a local theme YAML file. The flag changes only the current session and does not save to `~/.herald/conf.yaml`.

```sh
./bin/herald --demo -theme jade-signal
./bin/herald --demo -theme ./my-theme.yaml
./bin/herald -config ~/.herald/conf.yaml -theme amber-furnace
```

## Build Your Own

Start from the [Jade Signal baseline YAML](/examples/themes/jade-signal-baseline.yaml) when you want a complete local theme file to copy and edit. The sample uses the installable slug `my-jade-signal` so it does not conflict with Herald's reserved built-in `jade-signal` theme.

```sh
./bin/herald --demo -theme ./jade-signal-baseline.yaml
```

You can also install the same file from Settings by opening `S`, choosing `Theme`, and entering the local YAML path in `Install local theme YAML`.

## Theme Index

Use these links to jump directly to a theme section. Each section has a stable heading anchor, so URLs such as `/themes/#jade-signal` can be shared. Click any screenshot to zoom it, then click the expanded image or press `Esc` to return to the page.

- Red / Warm: [Red Black](#red-black), [Crimson](#crimson), [Ember](#ember), [Ruby Noir](#ruby-noir), [Garnet Console](#garnet-console), [Amber Furnace](#amber-furnace), [Copper Ash](#copper-ash), [Magma Core](#magma-core)
- Green / Earth: [Jade Signal](#jade-signal), [Viridian Glass](#viridian-glass), [Forest CRT](#forest-crt), [Pine Mail](#pine-mail), [Olive Circuit](#olive-circuit), [Sepia Debug](#sepia-debug), [Solar Paper](#solar-paper), [Zenbones Light](#zenbones-light)
- Blue / Violet: [Peacock Ink](#peacock-ink), [Ultramarine Desk](#ultramarine-desk), [Amethyst Night](#amethyst-night), [Graphite Rose](#graphite-rose), [Arctic Signal](#arctic-signal), [Iceberg Queue](#iceberg-queue), [Sonokai Signal](#sonokai-signal)
- Diverse Terminal Palettes: [Ayu Courier](#ayu-courier), [Cobalt Dispatch](#cobalt-dispatch), [Kanagawa Post](#kanagawa-post), [Rose Pine Desk](#rose-pine-desk), [Tokyo Dusk](#tokyo-dusk), [Panda Packet](#panda-packet), [Tomorrow Desk](#tomorrow-desk)

## Red / Warm

These themes emphasize red, amber, copper, and high-heat contrast. They are useful when you want strong status colors and a dramatic terminal profile without losing table readability.

### Red Black

<div class="theme-shot" aria-label="Timeline and Preview screenshots for red-black">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/red-black.png"><img src="/screenshots/themes/red-black.png" alt="Red Black Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/red-black.png"><img src="/screenshots/themes/preview/red-black.png" alt="Red Black Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Crimson

<div class="theme-shot" aria-label="Timeline and Preview screenshots for crimson">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/crimson.png"><img src="/screenshots/themes/crimson.png" alt="Crimson Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/crimson.png"><img src="/screenshots/themes/preview/crimson.png" alt="Crimson Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Ember

<div class="theme-shot" aria-label="Timeline and Preview screenshots for ember">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/ember.png"><img src="/screenshots/themes/ember.png" alt="Ember Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/ember.png"><img src="/screenshots/themes/preview/ember.png" alt="Ember Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Ruby Noir

<div class="theme-shot" aria-label="Timeline and Preview screenshots for ruby-noir">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/ruby-noir.png"><img src="/screenshots/themes/ruby-noir.png" alt="Ruby Noir Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/ruby-noir.png"><img src="/screenshots/themes/preview/ruby-noir.png" alt="Ruby Noir Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Garnet Console

<div class="theme-shot" aria-label="Timeline and Preview screenshots for garnet-console">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/garnet-console.png"><img src="/screenshots/themes/garnet-console.png" alt="Garnet Console Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/garnet-console.png"><img src="/screenshots/themes/preview/garnet-console.png" alt="Garnet Console Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Amber Furnace

<div class="theme-shot" aria-label="Timeline and Preview screenshots for amber-furnace">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/amber-furnace.png"><img src="/screenshots/themes/amber-furnace.png" alt="Amber Furnace Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/amber-furnace.png"><img src="/screenshots/themes/preview/amber-furnace.png" alt="Amber Furnace Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Copper Ash

<div class="theme-shot" aria-label="Timeline and Preview screenshots for copper-ash">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/copper-ash.png"><img src="/screenshots/themes/copper-ash.png" alt="Copper Ash Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/copper-ash.png"><img src="/screenshots/themes/preview/copper-ash.png" alt="Copper Ash Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Magma Core

<div class="theme-shot" aria-label="Timeline and Preview screenshots for magma-core">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/magma-core.png"><img src="/screenshots/themes/magma-core.png" alt="Magma Core Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/magma-core.png"><img src="/screenshots/themes/preview/magma-core.png" alt="Magma Core Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

## Green / Earth

These themes lean into green, jade, olive, paper, and earth-toned palettes. They keep the mailbox calm while preserving obvious focus and selection states.

### Jade Signal

<div class="theme-shot" aria-label="Timeline and Preview screenshots for jade-signal">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/jade-signal.png"><img src="/screenshots/themes/jade-signal.png" alt="Jade Signal Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/jade-signal.png"><img src="/screenshots/themes/preview/jade-signal.png" alt="Jade Signal Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Viridian Glass

<div class="theme-shot" aria-label="Timeline and Preview screenshots for viridian-glass">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/viridian-glass.png"><img src="/screenshots/themes/viridian-glass.png" alt="Viridian Glass Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/viridian-glass.png"><img src="/screenshots/themes/preview/viridian-glass.png" alt="Viridian Glass Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Forest CRT

<div class="theme-shot" aria-label="Timeline and Preview screenshots for forest-crt">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/forest-crt.png"><img src="/screenshots/themes/forest-crt.png" alt="Forest CRT Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/forest-crt.png"><img src="/screenshots/themes/preview/forest-crt.png" alt="Forest CRT Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Pine Mail

<div class="theme-shot" aria-label="Timeline and Preview screenshots for pine-mail">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/pine-mail.png"><img src="/screenshots/themes/pine-mail.png" alt="Pine Mail Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/pine-mail.png"><img src="/screenshots/themes/preview/pine-mail.png" alt="Pine Mail Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Olive Circuit

<div class="theme-shot" aria-label="Timeline and Preview screenshots for olive-circuit">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/olive-circuit.png"><img src="/screenshots/themes/olive-circuit.png" alt="Olive Circuit Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/olive-circuit.png"><img src="/screenshots/themes/preview/olive-circuit.png" alt="Olive Circuit Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Sepia Debug

<div class="theme-shot" aria-label="Timeline and Preview screenshots for sepia-debug">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/sepia-debug.png"><img src="/screenshots/themes/sepia-debug.png" alt="Sepia Debug Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/sepia-debug.png"><img src="/screenshots/themes/preview/sepia-debug.png" alt="Sepia Debug Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Solar Paper

<div class="theme-shot" aria-label="Timeline and Preview screenshots for solar-paper">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/solar-paper.png"><img src="/screenshots/themes/solar-paper.png" alt="Solar Paper Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/solar-paper.png"><img src="/screenshots/themes/preview/solar-paper.png" alt="Solar Paper Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Zenbones Light

<div class="theme-shot" aria-label="Timeline and Preview screenshots for zenbones-light">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/zenbones-light.png"><img src="/screenshots/themes/zenbones-light.png" alt="Zenbones Light Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/zenbones-light.png"><img src="/screenshots/themes/preview/zenbones-light.png" alt="Zenbones Light Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

## Blue / Violet

These themes are built around blue, cyan, violet, rose, and cool graphite accents. They suit long reading sessions where preview contrast and row selection need to stay crisp.

### Peacock Ink

<div class="theme-shot" aria-label="Timeline and Preview screenshots for peacock-ink">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/peacock-ink.png"><img src="/screenshots/themes/peacock-ink.png" alt="Peacock Ink Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/peacock-ink.png"><img src="/screenshots/themes/preview/peacock-ink.png" alt="Peacock Ink Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Ultramarine Desk

<div class="theme-shot" aria-label="Timeline and Preview screenshots for ultramarine-desk">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/ultramarine-desk.png"><img src="/screenshots/themes/ultramarine-desk.png" alt="Ultramarine Desk Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/ultramarine-desk.png"><img src="/screenshots/themes/preview/ultramarine-desk.png" alt="Ultramarine Desk Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Amethyst Night

<div class="theme-shot" aria-label="Timeline and Preview screenshots for amethyst-night">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/amethyst-night.png"><img src="/screenshots/themes/amethyst-night.png" alt="Amethyst Night Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/amethyst-night.png"><img src="/screenshots/themes/preview/amethyst-night.png" alt="Amethyst Night Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Graphite Rose

<div class="theme-shot" aria-label="Timeline and Preview screenshots for graphite-rose">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/graphite-rose.png"><img src="/screenshots/themes/graphite-rose.png" alt="Graphite Rose Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/graphite-rose.png"><img src="/screenshots/themes/preview/graphite-rose.png" alt="Graphite Rose Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Arctic Signal

<div class="theme-shot" aria-label="Timeline and Preview screenshots for arctic-signal">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/arctic-signal.png"><img src="/screenshots/themes/arctic-signal.png" alt="Arctic Signal Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/arctic-signal.png"><img src="/screenshots/themes/preview/arctic-signal.png" alt="Arctic Signal Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Iceberg Queue

<div class="theme-shot" aria-label="Timeline and Preview screenshots for iceberg-queue">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/iceberg-queue.png"><img src="/screenshots/themes/iceberg-queue.png" alt="Iceberg Queue Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/iceberg-queue.png"><img src="/screenshots/themes/preview/iceberg-queue.png" alt="Iceberg Queue Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Sonokai Signal

<div class="theme-shot" aria-label="Timeline and Preview screenshots for sonokai-signal">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/sonokai-signal.png"><img src="/screenshots/themes/sonokai-signal.png" alt="Sonokai Signal Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/sonokai-signal.png"><img src="/screenshots/themes/preview/sonokai-signal.png" alt="Sonokai Signal Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

## Diverse Terminal Palettes

These themes borrow from well-known terminal palette families and mixed editorial palettes. They give Herald broader visual range without changing the underlying semantic roles.

### Ayu Courier

<div class="theme-shot" aria-label="Timeline and Preview screenshots for ayu-courier">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/ayu-courier.png"><img src="/screenshots/themes/ayu-courier.png" alt="Ayu Courier Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/ayu-courier.png"><img src="/screenshots/themes/preview/ayu-courier.png" alt="Ayu Courier Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Cobalt Dispatch

<div class="theme-shot" aria-label="Timeline and Preview screenshots for cobalt-dispatch">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/cobalt-dispatch.png"><img src="/screenshots/themes/cobalt-dispatch.png" alt="Cobalt Dispatch Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/cobalt-dispatch.png"><img src="/screenshots/themes/preview/cobalt-dispatch.png" alt="Cobalt Dispatch Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Kanagawa Post

<div class="theme-shot" aria-label="Timeline and Preview screenshots for kanagawa-post">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/kanagawa-post.png"><img src="/screenshots/themes/kanagawa-post.png" alt="Kanagawa Post Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/kanagawa-post.png"><img src="/screenshots/themes/preview/kanagawa-post.png" alt="Kanagawa Post Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Rose Pine Desk

<div class="theme-shot" aria-label="Timeline and Preview screenshots for rose-pine-desk">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/rose-pine-desk.png"><img src="/screenshots/themes/rose-pine-desk.png" alt="Rose Pine Desk Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/rose-pine-desk.png"><img src="/screenshots/themes/preview/rose-pine-desk.png" alt="Rose Pine Desk Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Tokyo Dusk

<div class="theme-shot" aria-label="Timeline and Preview screenshots for tokyo-dusk">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/tokyo-dusk.png"><img src="/screenshots/themes/tokyo-dusk.png" alt="Tokyo Dusk Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/tokyo-dusk.png"><img src="/screenshots/themes/preview/tokyo-dusk.png" alt="Tokyo Dusk Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Panda Packet

<div class="theme-shot" aria-label="Timeline and Preview screenshots for panda-packet">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/panda-packet.png"><img src="/screenshots/themes/panda-packet.png" alt="Panda Packet Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/panda-packet.png"><img src="/screenshots/themes/preview/panda-packet.png" alt="Panda Packet Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

### Tomorrow Desk

<div class="theme-shot" aria-label="Timeline and Preview screenshots for tomorrow-desk">
<div class="theme-shot-grid">
<figure><a href="/screenshots/themes/tomorrow-desk.png"><img src="/screenshots/themes/tomorrow-desk.png" alt="Tomorrow Desk Timeline screenshot" loading="lazy" /></a><figcaption>Timeline</figcaption></figure>
<figure><a href="/screenshots/themes/preview/tomorrow-desk.png"><img src="/screenshots/themes/preview/tomorrow-desk.png" alt="Tomorrow Desk Preview screenshot" loading="lazy" /></a><figcaption>Preview</figcaption></figure>
</div>
</div>

## Custom Themes

Settings can install local YAML files into `~/.herald/themes`. Config-level `theme.overrides` apply semantic role edits on top of the selected theme, so you can start from a built-in palette or the [Jade Signal baseline YAML](/examples/themes/jade-signal-baseline.yaml) and tune individual roles such as `chrome.status_bar`, `focus.selection_active`, or `metadata.sender`.

## Related Pages

These pages cover the settings, config, and media workflows that connect to theme selection and screenshots.

- [Settings](/features/settings/)
- [Config Reference](/reference/config/)
- [Regenerate Screenshots](/development/regenerate-screenshots/)
- [Demo Mode](/demo-mode/)
