# Gollem Chat Agent Roadmap

## Purpose

Herald needs a UI-only chat agent that can search mail, summarize result sets, show source messages in Timeline, and safely help edit an active Compose draft. This design replaces the current hand-written Ollama chat loop with Gollem as the agent framework while keeping Ollama available only as one local provider behind Gollem.

## Decision

The first implementation should replace the existing chat runtime rather than expand it. The current Bubble Tea chat drawer can remain as the shell, but the old Ollama `/api/chat` loop, in-process chat tool registry, and `<filter>` response block are legacy scaffolding.

- [ ] Use Gollem as the only planned chat-agent runtime.
- [ ] Treat Ollama/local, Anthropic, Kimi, and Fireworks as providers behind Gollem.
- [ ] Keep the first agent iteration UI-only inside the TUI chat drawer.
- [ ] Let the agent propose typed intents, while `internal/app` remains the only layer that mutates Timeline or Compose state.
- [ ] Ship useful read-only search and summarization before any send, delete, archive, calendar mutation, memory, or daily-summary work.

## Current State

Herald already has a chat drawer, conversation history, a legacy Ollama tool loop, timeline chat-filter state, hybrid Timeline search, semantic search, and Compose AI review. The replacement should reuse these UI and backend capabilities but remove prompt-smuggled control flow and avoid adding another parallel search or compose-edit system.

- [x] The chat drawer opens from the TUI and can display user and assistant messages.
- [x] Timeline can already show a chat-filtered result set and clear it with existing unwind behavior.
- [x] Timeline search already supports keyword, FTS body search, cross-folder search, semantic search, and hybrid keyword-plus-semantic search.
- [x] Compose already has an AI review/accept flow with diff and undo behavior.
- [ ] The legacy chat path still uses direct Ollama/OpenAI-style messages and a `<filter>` text block for UI control.

## Architecture

The implementation should add an `internal/agent` boundary that hides Gollem construction, provider setup, tool registration, and typed result parsing from Bubble Tea. `internal/app` should build bounded snapshots, call the runner, display progress, and apply returned intents through existing app state transitions.

- [ ] `internal/app` builds `ChatAgentInput` from the latest user message, current folder, active tab, visible/selected message IDs, current search state, and optional Compose draft snapshot.
- [ ] `internal/agent` exposes a small runner interface such as `Run(ctx, input) (ChatAgentResult, error)`.
- [ ] Gollem tools call existing backend/search helpers through a narrow dependency object instead of reading SQLite, IMAP, MCP, or daemon state directly.
- [ ] Timeline mutation happens only when `internal/app` receives a typed `TimelineIntent`.
- [ ] Compose mutation happens only when `internal/app` receives a typed `ComposeIntent` and opens the existing review/accept state.
- [ ] Provider-specific behavior stays in the Gollem provider factory, not in key routing, rendering, or Timeline code.

## Typed Contracts

The first contract should be small enough for local models to follow and explicit enough for tests to assert. The exact package names can change during implementation, but the result shape should preserve these responsibilities.

```go
type ChatAgentInput struct {
    UserMessage     string
    CurrentFolder   string
    ActiveTab       string
    VisibleIDs      []string
    SelectedIDs     []string
    ComposeSnapshot *ComposeSnapshot
}

type ChatAgentResult struct {
    Reply    string          `json:"reply"`
    Timeline *TimelineIntent `json:"timeline,omitempty"`
    Summary  *EmailSummary   `json:"summary,omitempty"`
    Compose  *ComposeIntent  `json:"compose,omitempty"`
}

type TimelineIntent struct {
    Mode       string   `json:"mode"` // explicit_ids | keyword | semantic | hybrid
    Query      string   `json:"query,omitempty"`
    MessageIDs []string `json:"message_ids,omitempty"`
    Label      string   `json:"label"`
}

type ComposeIntent struct {
    SubjectSuggestion string `json:"subject_suggestion,omitempty"`
    BodySuggestion    string `json:"body_suggestion,omitempty"`
    Rationale         string `json:"rationale,omitempty"`
}
```

- [ ] `Reply` is always safe to show in chat even when every optional intent is empty.
- [ ] `TimelineIntent.Mode=explicit_ids` uses returned IDs directly after local validation.
- [ ] `TimelineIntent.Mode=keyword`, `semantic`, or `hybrid` routes through existing Timeline search behavior.
- [ ] `ComposeIntent` is ignored with a user-visible notice when Compose is not active.
- [ ] The agent result never contains send/delete/archive/calendar mutation commands in the first iteration.

## Milestones

The milestones are ordered so the legacy chat can be replaced before adding higher-value tools. Each milestone should be independently testable and should leave the TUI usable if a provider is unavailable.

- [ ] **M1: Gollem runner skeleton**: add config, provider factory, fake/test model support, and a no-tool Gollem reply path behind the existing chat drawer.
- [ ] **M2: Replace legacy chat runtime**: route chat submission through Gollem, remove the old direct Ollama chat loop from the chat panel, and keep existing chat history/rendering behavior.
- [ ] **M3: Read-only email tools**: add `find_emails`, `get_email_context`, `summarize_email_set`, and `explain_people` with bounded JSON outputs and deterministic caps.
- [ ] **M4: Timeline projection**: apply typed Timeline intents for explicit IDs and keyword/semantic/hybrid searches, including labels, empty states, stale-result protection, and `Esc` recovery.
- [ ] **M5: Search-result summarization**: summarize source-backed result sets with people, dates, action items, open questions, and cited message IDs.
- [ ] **M6: Compose-aware editing**: when Compose is active, let chat propose subject/body edits through the existing AI review/accept flow without silently changing drafts.
- [ ] **M7: Provider bakeoff and hardening**: validate Ollama/local, Anthropic, Kimi, and Fireworks against the same fake-mail scenarios for tool calls, structured output, error handling, and latency.
- [ ] **M8: Legacy cleanup**: remove the legacy chat tool registry, `<filter>` parser contract, and obsolete roadmap/public-doc claims once Gollem covers the replacement behavior.

## First Tools

The first tool set should be read-only and source-backed. Tool payloads must stay compact because local providers can struggle with long schemas and large tool responses.

| Tool | Purpose | Output discipline |
| --- | --- | --- |
| `find_emails` | Search by topic, keyword, sender, unread flag, date hint, folder, and mode | Return capped metadata rows plus source, score when semantic, and total/capped counts |
| `get_email_context` | Fetch bounded context for selected message IDs | Return subject, sender, date, folder, body snippet, and thread hint without full raw MIME |
| `summarize_email_set` | Summarize an explicit bounded set | Return summary, involved people, dates, action items, open questions, and cited message IDs |
| `explain_people` | Identify who was involved and likely roles | Return people with role labels and evidence message IDs |

- [ ] Tool implementations must not fetch unbounded full bodies.
- [ ] Tool outputs must include enough message IDs for Timeline projection and citation.
- [ ] Tool errors must be concise and recoverable so Gollem can choose another path or produce a helpful reply.

## Provider Policy

Gollem is the framework contract. Provider differences should be isolated so app code does not care whether the model is local or remote.

| Provider | Route | Notes |
| --- | --- | --- |
| Ollama/local | Gollem OpenAI-compatible/Ollama provider | Offline path; reliability depends on local model tool and structured-output quality |
| Anthropic | Gollem Anthropic provider | Primary high-quality remote path |
| Kimi | Gollem OpenAI-compatible provider | Useful for Kimi-specific models and long-context experiments |
| Fireworks | Gollem OpenAI-compatible provider | Hosted open-model path, including Kimi variants when desired |

- [ ] Config selects provider, model, optional base URL, and provider-specific safety toggles under a chat-agent config block.
- [ ] Existing embedding config remains separate because semantic search depends on the current embedding cache contract.
- [ ] Provider bakeoff results should become recommendations before making any nonlocal provider a default.

## Error Handling

The chat drawer should feel responsive even when local models are slow or remote providers fail. Errors should be user-facing state, not raw provider traces.

- [ ] Empty input still does nothing.
- [ ] Missing provider config shows a concise disabled-agent message.
- [ ] Provider failure appends a bounded assistant error and clears waiting state.
- [ ] Tool failure is fed back to Gollem when possible and surfaced in the final reply when no recovery path exists.
- [ ] Invalid structured output produces a bounded retry or a clear "could not apply action" message.
- [ ] Timeline and Compose intents are validated locally before application.

## Testing And Verification

This touches chat routing, Timeline state, Compose state, provider boundaries, and TUI rendering. Verification should scale by milestone, with deterministic fake providers before live provider bakeoffs.

- [ ] Update `engineering/testplans/TUI_TESTPLAN.md` with Gollem chat replacement cases before implementation.
- [ ] Unit-test the agent runner with a fake model and fake tools.
- [ ] Unit-test typed result parsing and invalid-output handling.
- [ ] App-level tests prove chat submission uses Gollem instead of the legacy Ollama path.
- [ ] App-level tests prove Timeline intents apply and unwind through existing `Esc` behavior.
- [ ] App-level tests prove Compose intents open review mode and do not mutate the draft until accepted.
- [ ] Focused Go tests cover provider config selection without requiring live API keys.
- [ ] Manual tmux verification captures the chat drawer at `220x50`, `80x24`, and `50x15` once visible behavior changes.
- [ ] Provider bakeoff runs against fake/demo mail before any live mailbox testing.

## Non-Goals For First Execution

Keeping the first pass narrow makes replacement safer and gives us a real agent surface before adding risky mutations. These items can return after search, summarization, Timeline projection, and Compose review are reliable.

- [ ] No autonomous memory store.
- [ ] No daily summary scheduler.
- [ ] No daemon agent.
- [ ] No MCP mirroring of the new agent.
- [ ] No send-email command from chat.
- [ ] No delete/archive command from chat.
- [ ] No calendar create/edit/delete/RSVP from chat.
- [ ] No whole-mailbox unbounded summarization.

## Execution Handoff

Start execution with M1 and M2 as one focused replacement plan: wire Gollem into the existing chat drawer, prove plain replies work, and remove the old direct Ollama chat runtime. After that lands, implement M3 through M6 as small read-only/tool/UI intent slices rather than one giant agent feature.

- [ ] The first implementation plan should explicitly name which old chat files are replaced, which tests prove no legacy Ollama chat call remains, and which behavior remains temporarily unchanged.
- [ ] The first implementation plan should defer provider bakeoff to M7 unless a fake provider cannot cover a required behavior.
- [ ] The first implementation plan should preserve existing non-chat AI features such as classification, semantic embeddings, quick replies, contact enrichment, calendar AI summaries, and Compose AI assist.
