---
title: Chat Panel
description: Ask mailbox questions through Herald's right-side AI chat panel.
---

The chat panel lets you ask questions about the currently loaded mailbox context. Configured AI routes chat through the Gollem UI chat-agent path with read-only email tools and typed Timeline, summary, and Compose review intents.

## Overview

Press `g` from the main UI to open chat. The legacy `c` alias still works outside Timeline text and compose contexts. Chat is available when Herald is not loading, AI is configured, and the terminal is wide enough to render the main view beside the chat panel.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Chat title | Right-side panel title. |
| History | User and assistant messages from the current chat session. |
| Tool-derived answers | Assistant responses that may summarize read-only search, context, people, or sender-stat tool output. |
| Input | One-line chat input. |
| Waiting state | Chat stops accepting new submit actions while the assistant is responding. |
| Timeline filter | Assistant can return typed results that narrow Timeline to matching message IDs or route through existing Timeline search. |

<!-- HERALD_SCREENSHOT id="chat-panel-open" page="chat" alt="Herald chat panel open" state="demo mode, 120x40, chat panel focused" desc="Shows chat history area, input line, active focus, and compressed main Timeline view." capture="tmux demo 120x40; ./bin/herald --demo with AI configured; press 1; press c" -->

![Herald chat panel open](/screenshots/chat-panel-open.png)

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `g` | Main UI | Not loading and width allows chat. | Opens chat and focuses input; pressing again closes it. |
| `c` | Main UI | Legacy alias outside Timeline text and compose contexts. | Opens chat and focuses input; pressing again closes it. |
| `enter` | Chat input | Chat is focused and not waiting. | Sends the current message. |
| `esc` | Chat focused | Chat is open. | Closes chat and restores default panel focus. |
| `tab` | Chat focused | Chat is open. | Leaves/closes chat focus and returns to default panel focus. |
| `q` | Any chat state | Any state. | Quits Herald. |

## Workflows

### Ask a Mailbox Question

1. Open Timeline or Cleanup.
2. Press `g`.
3. Ask a specific question such as "show recent invoices" or "which senders have the most unread mail?"
4. Press `enter`.
5. Read the answer and any applied filter state.

### Use Timeline Results

1. Ask a question that implies a filtered set of messages.
2. If the assistant returns typed Timeline results, Timeline switches into a filtered view or opens the existing search pipeline.
3. Navigate and open messages normally.
4. Press `esc` in Timeline to clear the chat filter.

### Review A Compose Suggestion

1. Open Compose and draft some text.
2. Open chat with `g` and ask for a rewrite or subject suggestion.
3. If the agent returns a Compose suggestion, Herald opens the existing Compose AI review panel with the suggestion.
4. Accept, reject, or edit the suggestion from the review panel; the draft is not silently changed.

## States

| State | What happens |
| --- | --- |
| Width too small | Pressing `c` reports that chat is hidden at this size. |
| AI unavailable | Chat cannot produce assistant responses. |
| Waiting | Input remains visible, but `enter` does not submit another message until the current response finishes. |
| Provider or tool error | Herald shows a bounded assistant error and clears the waiting state. |
| Timeline filter active | Status indicates a filtered Timeline; `esc` clears it. |
| AI disabled | Explicit `ai.provider: disabled` keeps chat unavailable instead of falling back to a legacy runtime. |
| Compose suggestion outside Compose | Herald shows an `Open Compose` notice and does not open the review panel. |

## Data And Privacy

Chat sends the user's question, current folder, and bounded UI context to the configured AI backend. Read-only tools can search cached email metadata and fetch bounded body snippets by message ID; external providers receive the chat prompt and any tool results needed to answer. The first Gollem iteration cannot send, delete, archive, or mutate calendar events.

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
