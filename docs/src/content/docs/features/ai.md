---
title: AI Features
description: Configure and use classification, embeddings, semantic search, quick replies, chat, compose assist, and enrichment.
---

AI in Herald is optional. When enabled, it powers classification tags, semantic search, mailbox chat, quick replies, Compose rewriting, subject suggestions, image descriptions, custom prompts, and contact enrichment.

## Overview

The default AI path is Ollama on a local host. Herald can also be configured for Claude or an OpenAI-compatible provider. Non-AI mail reading, composing, cleanup, and sync keep working when AI is disabled or unavailable.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| AI status chip | Bottom status fragment such as idle, tag, embed, reply, search, chat, defer, down, or off. |
| Classification tags | Timeline and Cleanup tag columns or preview tag lines. |
| Classification progress | Status fragment like current/total tag progress. |
| Embedding progress | Status fragment for embedding batch processing. |
| Semantic search | Timeline and Contacts search queries that start with `?` after opening search with `/`. |
| Quick reply picker | Canned replies plus optional AI-generated replies. |
| Chat panel | AI conversation over recent mailbox context and tool results. |
| Compose AI panel | Rewrite prompt, quick actions, AI response, and accept control. |
| Prompt editor | Saved reusable AI prompts opened with `P`. |

<!-- HERALD_SCREENSHOT id="ai-status-chip" page="ai" alt="AI status chip in Herald status bar" state="demo mode, 120x40, AI configured" desc="Shows AI status chip in the bottom status bar alongside folder status and key hints." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; press 1" -->

![AI status chip in Herald status bar](/screenshots/ai-status-chip.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `a` | Main UI | AI classifier configured and folder has classifiable mail. | Starts classification for the current folder. |
| `A` | Timeline or Cleanup preview | AI configured and a target email is selected. | Re-classifies the current single email. |
| `? query` | Timeline search after `/` | AI/embeddings available. | Runs semantic email search. |
| `? query` | Contacts search after `/` | AI/embeddings available. | Runs semantic contact search. |
| `ctrl+q` | Timeline | Current email exists. | Opens quick reply picker with canned and optional AI choices. |
| `c` | Main UI | AI configured, not loading, width allows chat. | Opens chat panel. |
| `ctrl+g` | Compose | AI configured. | Opens Compose AI assistant panel. |
| `ctrl+j` | Compose | AI configured and body or reply context exists. | Generates subject suggestion. |
| `ctrl+enter` | Compose AI panel | AI response available. | Accepts generated response into the body. |
| `P` | Main UI | Prompt editor closed. | Opens custom prompt editor. |
| `e` | Contacts | A contact is selected. | Runs contact enrichment. |

## Workflows

### Configure Local Ollama

1. Install and start Ollama.
2. Pull the configured chat/classification model and embedding model.
3. Set `ollama.host`, `ollama.model`, and `ollama.embedding_model` in config or through settings.
4. Launch Herald and check the AI chip.

### Classify a Folder

1. Open the folder in Timeline or Cleanup.
2. Press `a`.
3. Watch status progress.
4. Review the Tag column after classification completes.

### Generate Quick Replies

1. Select or open a Timeline email.
2. Press `ctrl+q`.
3. Choose a canned reply immediately or wait for AI replies when available.
4. Select a reply to open Compose.

### Use Compose AI

1. Open Compose and write body text.
2. Press `ctrl+g`.
3. Use quick actions `1` through `5` or type a custom instruction.
4. Press `ctrl+enter` to accept the generated text.

### Enrich Contacts

1. Press `3`.
2. Select a contact.
3. Press `e`.
4. Review enriched company/topics in the detail panel when complete.

## States

| State | What happens |
| --- | --- |
| AI off | No classifier is configured; chip may show `AI: off` outside demo mode. |
| AI idle | AI is configured but no task is active. |
| AI tag | Classification is running. |
| AI embed | Embedding generation is running. |
| AI reply | Quick replies are being generated. |
| AI search | Semantic search is running. |
| AI chat | Chat request/tool loop is running. |
| AI defer | Scheduler has deferred a task. |
| AI down | Provider is unavailable or failed recently. |
| External provider | Selected message/draft/search context may leave the machine for the requested feature. |
| Model changed | Herald can invalidate stale embeddings tied to a previous embedding model. |

## Data And Privacy

AI features send only the context needed for the requested action, but that context can include sender, subject, body snippets, full body text, contact metadata, folder summaries, or tool results. Ollama keeps requests local to the configured Ollama host. Claude and OpenAI-compatible providers receive prompts through their APIs. Semantic embeddings are stored in SQLite and tied to the configured embedding model.

## Troubleshooting

If AI actions report unavailable, test Ollama with `curl http://localhost:11434/api/tags` or verify external provider keys in settings.

If tags are blank, run `a` on the current folder and wait for classification progress to finish.

If semantic search is weak, confirm the embedding model is installed and allow embedding progress to finish.

If Compose AI says "Write something first", add body text or open Compose from a reply context.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="ai-classification-progress" page="ai" alt="AI classification progress in status bar" state="demo mode, 120x40, classification running" desc="Shows classification progress, AI tag status, tag column changes, and responsive UI while classification is active." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; press 1; press a" -->

![AI classification progress in status bar](/screenshots/ai-classification-progress.png)

<!-- HERALD_SCREENSHOT id="ai-compose-assist" page="ai" alt="Compose AI assistant response" state="demo mode, 120x40, Compose AI panel with response" desc="Shows AI rewrite response, quick action controls, custom prompt field, and ctrl+enter accept state." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; press C; write body; press ctrl+g; press 1" -->

![Compose AI assistant response](/screenshots/ai-compose-assist.png)

<!-- HERALD_SCREENSHOT id="ai-prompt-editor" page="ai" alt="Custom prompt editor overlay" state="demo mode, 120x40, prompt editor open" desc="Shows prompt name, output variable, system prompt, user template, saved prompt summary, and form help." capture="tmux demo 120x40; ./bin/herald --demo; press P" -->

![Custom prompt editor overlay](/screenshots/ai-prompt-editor.png)

## Related Pages

- [Search](/features/search/)
- [Chat Panel](/features/chat/)
- [Compose](/using-herald/compose/)
- [Rules and Automation](/features/rules-automation/)
- [Privacy and Security](/security-privacy/)
