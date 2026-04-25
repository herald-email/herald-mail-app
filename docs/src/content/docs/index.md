---
title: Herald Docs
description: Learn how to install, configure, and integrate Herald.
---

Herald is a fast terminal email client for power users. It combines a keyboard-first inbox, bulk cleanup, local AI classification, semantic search, quick replies, contacts, and an MCP server for AI tools.

Use these docs when you want more detail than the project README: first-run setup, provider configuration, demo mode, MCP integration, privacy expectations, and cleanup instructions.

## Fastest path

```sh
git clone https://github.com/herald-email/herald-mail-app.git
cd herald-mail-app
make build
./bin/herald
```

On first launch, Herald opens the setup wizard if `~/.herald/conf.yaml` is missing or empty. Choose a provider, enter credentials, decide whether to configure local AI, and save the generated config.

## Main features

- Timeline inbox with split email preview
- Compose, reply, forward, Markdown preview, and quick replies
- Bulk cleanup by sender or domain
- AI classification, chat, and semantic search
- Contact book with local enrichment
- MCP server for AI agents and tools
- SSH mode for running the TUI remotely
- Demo mode for screenshots, GIFs, and testing without real mail

## Local docs commands

This docs site is local-only for now.

```sh
cd docs
npm install
npm run dev
```

Use `npm run build` to verify the docs compile.
