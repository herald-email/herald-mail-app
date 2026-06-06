# Obsidian-Friendly Email Memories Roadmap

## Purpose

Herald should turn important email history into local, source-backed memory that helps the user understand people, companies, threads, and open loops. The product promise is Obsidian-friendly email memories: useful inside Herald chat and Compose, but durable as editable Markdown outside the app.

- [ ] Give users a trustworthy answer to "what should I remember before I reply?"
- [ ] Make Sent mail a first-class source because user replies reveal commitments, decisions, tone, and resolved loops.
- [ ] Produce memory artifacts that can sync to an Obsidian vault without becoming a separate closed database.
- [ ] Keep every factual claim tied to source evidence from email, notes, calendar context, or explicit research.

## Product Positioning

This feature is a local relationship intelligence layer, not an autonomous assistant that reads everything and acts silently. Herald should feel like a careful email memory system that can chat, nudge, and prepare dossiers while keeping the user in control.

- [ ] Name the capability "Herald Memories" in product copy.
- [ ] Use "Obsidian-friendly email memories" as the power-user wedge for local-first and PKM users.
- [ ] Position Compose Radar as the signature interaction: small, source-backed context nudges while writing.
- [ ] Position dossiers as living people, company, and thread notes rather than generated reports.
- [ ] Prefer "source-backed" and "editable Markdown" over vague "AI remembers" language.

## Current Context

Herald already has most of the raw ingredients: cached email metadata, body caching, semantic embeddings, contact enrichment, a Contacts tab, Compose AI review, quick replies, and an emerging Gollem chat-agent roadmap. The memory work should extend those foundations after the Gollem boundary exists, rather than deepen the legacy chat loop.

- [x] Contacts are derived from mail headers and include email counts, sent counts, company, topics, and semantic search.
- [x] Timeline threads already link sent replies through provider thread IDs or RFC reply headers.
- [x] Compose has an AI review/accept flow that can safely present suggestions without silently changing drafts.
- [x] Chat has a visible drawer, but the current legacy runtime is scheduled for Gollem replacement.
- [x] The user's Obsidian vault already uses `People/`, `Job search/active`, `Job search/backlog`, `Job search/done`, and `Scheduled Task Artifacts/` as durable surfaces.
- [ ] Herald does not yet have a first-class memory model, track model, dossier model, or Obsidian sync contract.

## User Jobs

The target users are people who use email as an operating layer for work, job search, consulting, recruiting, family administration, and founder/customer conversations. The feature should solve recall and continuity problems at the moment they read, chat, and compose.

- [ ] As a user composing a reply, I can see relevant context from related threads before I send.
- [ ] As a user researching a person or company, I can combine my private relationship history with explicit public research.
- [ ] As a user reviewing job-search state, I can ask what is active, stale, waiting, rejected, or ready for follow-up.
- [ ] As a user maintaining Obsidian notes, I can let Herald update concise memory sections without overwriting my hand-written notes.
- [ ] As a privacy-conscious user, I can inspect, dismiss, correct, forget, or export memories.

## Core Objects

The memory layer should be built from small, understandable objects with clear evidence. These objects should be stable enough for SQLite, chat retrieval, Compose Radar, and Markdown export, but not overfit to one UI.

| Object | Purpose | Required evidence |
| --- | --- | --- |
| `Memory` | A compact extracted fact, commitment, question, date, status, or relationship note | Message ref, note path, calendar ref, or research URL |
| `Track` | An ongoing storyline such as a job process, recruiter thread, project, open loop, or relationship callback | One or more memories plus latest activity |
| `Dossier` | A person, company, or thread page assembled from memories, tracks, notes, and research | Source list and freshness timestamp |
| `Nudge` | A compose-time recommendation or warning | Evidence refs and confidence |
| `Evidence` | A normalized source pointer | Source type, stable ID or path, date, and snippet summary |

- [ ] `Memory` stores the claim, kind, confidence, freshness, source refs, and optional Obsidian target.
- [ ] `Track` stores topic, people, company/domain, status, open loops, claims, commitments, last activity, and evidence refs.
- [ ] `Dossier` stores relationship summary, recent interactions, active tracks, open loops, research notes, vault links, and freshness.
- [ ] `Nudge` stores type, message, why it matters, source refs, user action state, and dismissal scope.
- [ ] `Evidence` can point to email messages, sent replies, notes, calendar events, attachments, and research URLs without copying full private content.

## Primary Surfaces

Herald Memories should appear where context changes user behavior: chat, Compose, Contacts, and the user's notes. Each surface should have a narrow first version and clear boundaries.

- [ ] Chat gains memory-aware read-only tools for contact history, company status, related replies, open loops, and "what should I remember before replying?"
- [ ] Compose gains Compose Radar, a compact panel that shows at most three relevant nudges while writing.
- [ ] Contacts gains person dossiers with relationship summary, recent messages, active tracks, open loops, linked notes, and source evidence.
- [ ] Job-search company dossiers mirror the Obsidian `Job search/{active,backlog,done}/{Company}/` structure.
- [ ] Daily briefing output becomes a diff over changed tracks, stale loops, and newly resolved questions.

## Compose Radar

Compose Radar is the flagship interaction because it catches context at the moment of reply. It should be quiet, bounded, and useful enough that users trust it instead of treating it as another AI panel.

- [ ] Trigger retrieval when Compose opens for a reply, forward, draft edit, or new message with known recipients.
- [ ] Re-rank nudges when recipient, subject, or draft body changes, with debounce and stale-result protection.
- [ ] Show at most three nudges by default.
- [ ] Support nudge types: conflict, callback, open loop, relationship context, research update, and draft risk.
- [ ] Provide actions: open source, insert phrase, dismiss, mark resolved, save memory, research person/company.
- [ ] Route insertions through the existing Compose edit/review path instead of mutating drafts silently.
- [ ] Keep the panel hidden or collapsed when no high-confidence nudge exists.

## Obsidian Sync

Obsidian sync is a product boundary, not just export. Herald should write Markdown that users can read and edit, preserve user-written sections, and avoid duplicating canonical notes.

- [ ] Add a settings entry for the Obsidian vault path and optional target folders.
- [ ] Support generated sections inside existing people and company notes using stable markers.
- [ ] Preserve user-edited content outside generated sections.
- [ ] Use frontmatter for machine-readable fields such as `last_contact`, `status`, `company`, `source`, `memory_updated`, and `herald_memory_id`.
- [ ] Use note links to canonical notes rather than copying long summaries across multiple files.
- [ ] Offer a sync preview before first write and before destructive section rewrites.
- [ ] Keep daily briefing output under a configured `Scheduled Task Artifacts/` path when enabled.

## Research Mode

Research Mode should enrich dossiers with public information only when the user asks for it. Private emails should remain local, and external queries should use minimal public identifiers unless the user explicitly approves more context.

- [ ] Add explicit actions: research this person, research this company, refresh dossier, and research before reply.
- [ ] Build web queries from public identifiers such as name, company, domain, role, and user-provided URL.
- [ ] Do not send private email bodies, private notes, attachments, or full thread summaries to external research by default.
- [ ] Save research notes with source URLs, retrieval date, confidence, and "what changed since last contact."
- [ ] Mark research stale after a configurable interval.
- [ ] Distinguish "from your email", "from Obsidian", "from public research", and "inference" in chat answers.

## Architecture Direction

The implementation should add a memory service behind the backend/agent boundary and reuse existing cache, contact, embedding, and Compose review capabilities. It should not create a second chat framework or bypass the existing Backend discipline.

- [ ] Add memory storage and retrieval behind backend-facing methods rather than direct SQLite access from Bubble Tea.
- [ ] Add extraction jobs that run through the existing AI scheduler with interactive-before-background priority.
- [ ] Reuse existing body-cache and embedding data before fetching additional message bodies.
- [ ] Add memory-aware Gollem tools after the Gollem chat-agent runner is in place.
- [ ] Keep external research as a separate opt-in capability from local memory extraction.
- [ ] Keep Obsidian sync as an export/sync adapter so the local memory index remains usable without Obsidian.
- [ ] Ensure deletion, archive, and cache cleanup can mark memories stale or remove evidence links when source messages disappear.

## Roadmap

The roadmap is ordered so the feature becomes useful before it becomes broad. The first slices should focus on job-search and work-related threads because they have high value, clear statuses, and existing Obsidian folder conventions.

- [ ] **M0: Product examples and test fixtures** - create realistic demo scenarios for job-search threads, conflicting timelines, open loops, callbacks, and sent-reply resolution.
- [ ] **M1: Local email memories MVP** - extract last contact, last user reply, open questions, commitments, deadlines, people, company, topic, and evidence from cached Inbox plus Sent messages.
- [ ] **M2: Memory-aware chat tools** - add read-only Gollem tools for contact history, company tracks, related replies, open loops, and reply-prep context.
- [ ] **M3: Obsidian sync preview** - configure vault path, detect People and Job Search folders, generate Markdown sections, and preview writes before saving.
- [ ] **M4: Compose Radar v1** - surface source-backed nudges for job-search replies and high-confidence people callbacks, with open/dismiss/insert actions.
- [ ] **M5: Dossier views** - enrich Contacts and company/thread detail views with relationship summaries, active tracks, open loops, vault links, and evidence.
- [ ] **M6: Research Mode** - add explicit person/company research, sourced research notes, freshness checks, and "research before reply."
- [ ] **M7: Daily memory briefing** - produce a diff over changed tracks, resolved questions, stale loops, and vault hygiene items.
- [ ] **M8: Hardening and privacy controls** - add forget, pin, correct, source audit, retention settings, and deletion propagation.

## First Shippable Slice

The first implementation should make one narrow scenario feel excellent instead of trying to remember every mailbox. Job-search Compose Radar is the recommended slice because it uses Inbox, Sent, Contacts, and Obsidian notes in a way users can immediately judge.

- [ ] Focus on job-search threads in `Job search/active` plus related Inbox and Sent messages.
- [ ] Detect "already replied", "awaiting response", "deadline", "timeline mismatch", and "relationship callback" nudges.
- [ ] Show nudges only in reply Compose and only when evidence is strong.
- [ ] Let the user open the source email or Obsidian note from each nudge.
- [ ] Write no Obsidian changes until sync preview is approved.
- [ ] Treat uncertain claims as chat/search suggestions, not Compose warnings.

## Error Handling And Safety

Memory features can become creepy or noisy if they overclaim. The system should fail quiet, cite sources, and give the user direct control over corrections and deletion.

- [ ] No evidence means no factual memory answer.
- [ ] Low-confidence memories remain searchable but do not become Compose Radar nudges.
- [ ] Provider or local-model failure leaves Compose usable and shows a concise memory-unavailable state.
- [ ] Obsidian write conflicts show a preview and never overwrite user sections silently.
- [ ] External research failure keeps local dossiers usable.
- [ ] Deleted or missing source emails mark dependent memories stale until revalidated.
- [ ] User-dismissed nudges respect dismissal scope: this draft, this thread, this person, or permanently.

## Testing And Verification

Testing should start with deterministic fixtures and then graduate to tmux-visible Compose behavior. Live private mail should not be required to prove the feature.

- [ ] Update `engineering/testplans/TUI_TESTPLAN.md` with memory chat, Compose Radar, Contacts dossier, and Obsidian sync cases before implementation.
- [ ] Unit-test memory extraction on synthetic inbound and sent messages.
- [ ] Unit-test track status transitions for active, waiting, stale, resolved, backlog, and done.
- [ ] Unit-test evidence validation, deletion propagation, and stale-memory behavior.
- [ ] Unit-test Obsidian Markdown generation with frontmatter and generated-section preservation.
- [ ] App-level tests prove Compose Radar retrieval does not mutate draft content without user action.
- [ ] Snapshot or tmux tests cover Compose Radar at `220x50`, `80x24`, and `50x15`.
- [ ] Demo fixtures include enough job-search and relationship examples for docs screenshots and future VHS tapes.

## Non-Goals For First Execution

The first implementation should avoid autonomous behavior and broad private-data export. These items can be revisited after local extraction, source evidence, Compose Radar, and Obsidian sync are trustworthy.

- [ ] No automatic send, delete, archive, calendar mutation, or reply scheduling from memory features.
- [ ] No silent external research.
- [ ] No external upload of private email bodies by default.
- [ ] No whole-mailbox unbounded summarization.
- [ ] No automatic rewrite of user-authored Obsidian sections.
- [ ] No replacement for the existing scheduled task system until the daily memory briefing has proven value.
- [ ] No MCP or daemon memory API until the UI-only path is stable.

## Execution Handoff

When this roadmap moves into implementation planning, start with the job-search Compose Radar slice after the Gollem runner has replaced the legacy chat runtime. The implementation plan should preserve non-chat AI features and keep memory extraction read-only until the Obsidian sync preview is accepted.

- [ ] First plan should cover M0 through M2 if Gollem is already available, or defer memory-aware chat until Gollem M1-M3 land.
- [ ] First plan should explicitly name the memory storage tables, backend methods, extraction jobs, and fixture data it introduces.
- [ ] First plan should include degradation behavior for no AI, slow AI, no Obsidian vault, no Sent cache, and missing source evidence.
- [ ] First plan should keep Compose Radar insertion inside existing Compose review/accept mechanics.
- [ ] First plan should update public docs only after demo fixtures can show the feature without private mailbox data.
