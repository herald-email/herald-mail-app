---
title: Chat Panel
description: Ask mailbox questions through Herald's right-side AI chat panel.
---

The chat panel lets you ask questions about the currently loaded mailbox context. It can use built-in tools for search, sender mail, threads, and sender statistics, then display an answer without leaving the TUI.

## Overview

Press `c` from the main UI to open chat. Chat is available when Herald is not loading, AI is configured, and the terminal is wide enough to render the main view beside the chat panel.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Chat title | Right-side panel title. |
| History | User and assistant messages from the current chat session. |
| Tool-derived answers | Assistant responses that may summarize search, thread, or sender-stat tool output. |
| Input | One-line chat input. |
| Waiting state | Chat stops accepting new submit actions while the assistant is responding. |
| Timeline filter | Assistant can return a filter that narrows Timeline to matching message IDs. |

<!-- HERALD_SCREENSHOT id="chat-panel-open" page="chat" alt="Herald chat panel open" state="demo mode, 120x40, chat panel focused" desc="Shows chat history area, input line, active focus, and compressed main Timeline view." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; press 1; press c" -->

![Herald chat panel open](/screenshots/chat-panel-open.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `c` | Main UI | Not loading and width allows chat. | Opens chat and focuses input; pressing again closes it. |
| `enter` | Chat input | Chat is focused and not waiting. | Sends the current message. |
| `esc` | Chat focused | Chat is open. | Closes chat and restores default panel focus. |
| `tab` | Chat focused | Chat is open. | Leaves/closes chat focus and returns to default panel focus. |
| `q` | Any chat state | Any state. | Quits Herald. |

## Workflows

### Ask a Mailbox Question

1. Open Timeline or Cleanup.
2. Press `c`.
3. Ask a specific question such as "show recent invoices" or "which senders have the most unread mail?"
4. Press `enter`.
5. Read the answer and any applied filter state.

### Use a Chat Filter

1. Ask a question that implies a filtered set of messages.
2. If the assistant returns a filter, Timeline switches into filtered view.
3. Navigate and open messages normally.
4. Press `esc` in Timeline to clear the chat filter.

## States

| State | What happens |
| --- | --- |
| Width too small | Pressing `c` reports that chat is hidden at this size. |
| AI unavailable | Chat cannot produce assistant responses. |
| Waiting | Input remains visible, but `enter` does not submit another message until the current response finishes. |
| Tool unsupported | Herald falls back to ordinary assistant responses if the provider does not support tool calling. |
| Tool loop limit | Chat uses a bounded tool loop to avoid runaway tool calls. |
| Timeline filter active | Status indicates a filtered Timeline; `esc` clears it. |

## Data And Privacy

Chat sends the user's question, current folder, folder counts, and a compact set of recent email context to the configured AI backend. Chat tools can read cached email metadata, sender statistics, threads, and search results. External AI providers receive the chat prompt and any tool results needed to answer.

## Troubleshooting

If `c` does not open chat, widen the terminal or wait until loading completes.

If answers ignore recent messages, refresh the folder with `r` and try again.

If a filter hides too much, press `esc` in Timeline to clear the filter.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="chat-waiting-response" page="chat" alt="Chat panel waiting for assistant response" state="demo mode, 120x40, chat request in flight" desc="Shows chat waiting state, submitted user question, and status while AI is responding." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; press c; type summarize recent unread; press enter" -->

![Chat panel waiting for assistant response](/screenshots/chat-waiting-response.png)

<!-- HERALD_SCREENSHOT id="chat-filtered-timeline" page="chat" alt="Timeline filtered by chat result" state="demo mode, 120x40, chat filter active" desc="Shows Timeline rows narrowed by a chat filter and the status hint used to clear it." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; ask chat for a filter-producing query" -->

![Timeline filtered by chat result](/screenshots/chat-filtered-timeline.png)

## Related Pages

- [AI Features](/features/ai/)
- [Timeline](/using-herald/timeline/)
- [Search](/features/search/)
- [Privacy and Security](/security-privacy/)
