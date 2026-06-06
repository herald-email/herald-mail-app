# Obsidian-Friendly Email Memories Roadmap

## Purpose

Herald should turn important email history into local, source-backed memory that helps the user understand people, companies, threads, and open loops. The product promise is Obsidian-friendly email memories: useful inside Herald chat and Compose, but durable as editable Markdown outside the app.

- [x] Give users a trustworthy answer to "what should I remember before I reply?"
- [x] Make Sent mail a first-class source because user replies reveal commitments, decisions, tone, and resolved loops.
- [x] Produce memory artifacts that can sync to an Obsidian vault without becoming a separate closed database.
- [x] Keep every factual claim tied to source evidence from email, notes, calendar context, or explicit research.

## Product Positioning

This feature is a local relationship intelligence layer, not an autonomous assistant that reads everything and acts silently. Herald should feel like a careful email memory system that can chat, nudge, and prepare dossiers while keeping the user in control.

- [x] Name the capability "Herald Memories" in product copy.
- [x] Use "Obsidian-friendly email memories" as the power-user wedge for local-first and PKM users.
- [x] Position Compose Radar as the signature interaction: small, source-backed context nudges while writing.
- [x] Position dossiers as living people, company, and thread notes rather than generated reports.
- [x] Prefer "source-backed" and "editable Markdown" over vague "AI remembers" language.

## Current Context

Herald already has most of the raw ingredients: cached email metadata, body caching, semantic embeddings, contact enrichment, a Contacts tab, Compose AI review, quick replies, and an emerging Gollem chat-agent roadmap. The memory work should extend those foundations after the Gollem boundary exists, rather than deepen the legacy chat loop.

- [x] Contacts are derived from mail headers and include email counts, sent counts, company, topics, and semantic search.
- [x] Timeline threads already link sent replies through provider thread IDs or RFC reply headers.
- [x] Compose has an AI review/accept flow that can safely present suggestions without silently changing drafts.
- [x] Chat has a visible drawer, but the current legacy runtime is scheduled for Gollem replacement.
- [x] The user's Obsidian vault already uses `People/`, `Job search/active`, `Job search/backlog`, `Job search/done`, and `Scheduled Task Artifacts/` as durable surfaces.
- [x] Herald has a first-class memory model, reply-prep nudge model, immutable local store, and Obsidian generated-section preview contract.
- [x] Herald has track lifecycle, dossier UI, and approved Obsidian write-flow foundations for generated Markdown sections.

## User Jobs

The target users are people who use email as an operating layer for work, job search, consulting, recruiting, family administration, and founder/customer conversations. The feature should solve recall and continuity problems at the moment they read, chat, and compose.

- [x] As a user composing a reply, I can see relevant context from related threads before I send.
- [x] As a user researching a person or company, I can combine my private relationship history with explicit public research.
- [x] As a user reviewing job-search state, I can ask what is active, stale, waiting, rejected, or ready for follow-up.
- [x] As a user maintaining Obsidian notes, I can let Herald update concise memory sections without overwriting my hand-written notes.
- [x] As a privacy-conscious user, I can inspect, dismiss, correct, forget, or export memories.

## Core Objects

The memory layer should be built from small, understandable objects with clear evidence. These objects should be stable enough for SQLite, chat retrieval, Compose Radar, and Markdown export, but not overfit to one UI.

| Object | Purpose | Required evidence |
| --- | --- | --- |
| `Memory` | A compact extracted fact, commitment, question, date, status, or relationship note | Message ref, note path, calendar ref, or research URL |
| `Track` | An ongoing storyline such as a job process, recruiter thread, project, open loop, or relationship callback | One or more memories plus latest activity |
| `Dossier` | A person, company, or thread page assembled from memories, tracks, notes, and research | Source list and freshness timestamp |
| `Nudge` | A compose-time recommendation or warning | Evidence refs and confidence |
| `Evidence` | A normalized source pointer | Source type, stable ID or path, date, and snippet summary |

- [x] `Memory` stores the claim, kind, confidence, freshness, source refs, and optional Obsidian target.
- [x] `Memory` exposes inspectable details: generated summary, normalized source quote/snippet, source count, extraction prompt version, confidence, last updated time, and stale/revalidated state.
- [x] `Track` stores topic, people, company/domain, status, open loops, claims, commitments, last activity, memory IDs, and evidence refs.
- [x] `Dossier` stores relationship summary, recent interactions, active tracks, open loops, research notes, vault links, and freshness.
- [x] `Nudge` stores type, message, why it matters, source refs, user action state, and dismissal scope.
- [x] `Evidence` can point to email messages, sent replies, notes, calendar events, attachments, and research URLs without copying full private content.

## Primary Surfaces

Herald Memories should appear where context changes user behavior: chat, Compose, Contacts, and the user's notes. Each surface should have a narrow first version and clear boundaries.

- [x] Chat gains memory-aware read-only tools for contact history, company status, related replies, open loops, and "what should I remember before replying?"
- [x] Compose gains Compose Radar, a compact panel that shows at most three relevant nudges while writing.
- [x] Contacts gains read-only person dossiers with relationship summary, recent messages, active tracks, open loops, linked notes, and source evidence.
- [x] Job-search company dossiers mirror the Obsidian `Job search/{active,backlog,done}/{Company}/` structure.
- [x] Email preview gains a read-only thread dossier with active track, open loop, vault link, and source evidence for the selected thread.
- [x] Daily briefing output becomes a diff over changed tracks, stale loops, and newly resolved questions.

## Compose Radar

Compose Radar is the flagship interaction because it catches context at the moment of reply. It should be quiet, bounded, and useful enough that users trust it instead of treating it as another AI panel.

- [x] Trigger retrieval when Compose opens for a reply or quick reply with known recipients.
- [x] Re-rank nudges when recipient, subject, or draft body changes, with debounce and stale-result protection.
- [x] Show at most three nudges by default.
- [x] Support nudge types: conflict, callback, open loop, relationship context, research update, and draft risk.
- [x] Provide explicit local actions: open source, insert phrase, dismiss, mark resolved, save memory, research person/company intent.
- [x] Keep Compose Radar read-only in v1 so it never mutates drafts silently.
- [x] Keep the panel hidden or collapsed when no high-confidence nudge exists.

## Obsidian Sync

Obsidian sync is a product boundary, not just export. Herald should write Markdown that users can read and edit, preserve user-written sections, and avoid duplicating canonical notes.

- [x] Add config entries for the Obsidian vault path and optional target folders.
- [x] Let users configure where each memory type goes: people notes, company/job notes, project notes, daily briefing notes, research notes, and an optional catch-all memory inbox.
- [x] Add an Obsidian output profile toggle with sensible defaults for YAML frontmatter, Dataview-friendly fields, tags, wiki links, backlinks, and plain-Markdown fallback.
- [x] Let users choose frontmatter mode: full YAML, minimal YAML, generated-section metadata only, or no visible YAML for cleaner human notes.
- [x] Let users choose link mode: Obsidian wiki links, standard Markdown links, plain paths, or no generated links.
- [x] Let users choose tag mode: no tags, conservative tags, workflow tags such as `#herald/memory` and `#job-search`, or custom tag templates.
- [x] Support generated sections inside existing people and company notes using stable markers.
- [x] Preserve user-edited content outside generated sections.
- [x] Use frontmatter for machine-readable fields such as `last_contact`, `status`, `company`, `source`, `memory_updated`, and `herald_memory_id`.
- [x] Use note links to canonical notes rather than copying long summaries across multiple files.
- [x] Offer a sync preview before first write and before destructive section rewrites.
- [x] Keep daily briefing output under a configured `Scheduled Task Artifacts/` path when enabled.

## Configuration And Defaults

The feature should be tweakable without making users design a memory system from scratch. Defaults should work for a typical local-first user, while advanced settings allow power users to steer storage, prompts, update cadence, and privacy boundaries.

- [x] Provide a default memory profile that stores people memories under `People/`, job/company memories under `Job search/`, daily diffs under `Scheduled Task Artifacts/`, uncategorized items under a configurable memory inbox, and uses minimal Obsidian frontmatter plus conservative links/tags.
- [x] Let users choose included sources: Inbox, Sent, selected folders, selected accounts, Contacts, Calendar, Obsidian, and explicit research notes.
- [x] Let users set per-memory-type destinations for people, companies, threads, job search, projects, research notes, and daily briefing output.
- [x] Let users set update cadence: manual only, on compose open, after sync, daily briefing, or background when idle.
- [x] Let users set confidence thresholds for chat retrieval, dossier inclusion, Obsidian writes, and Compose Radar nudges separately.
- [x] Let users choose whether low-confidence memories are hidden, shown only in chat, or saved to a review queue.
- [x] Surface the shipped safe defaults in Settings: immutable local records, configured cached-mail sources, external research opt-in, and private body text never sent to web research by default.
- [x] Provide a Settings screen section that summarizes memory status, configured vault path, prompt template count, update rules, and confidence thresholds.
- [x] Add read-only total, stale, and review-needed memory counts to Settings from the immutable local store without mutating memory records.
- [x] Extend the Settings screen section with last run, pending writes, failed writes, and approved write-flow state once Obsidian writes exist.

## Prompt Surface

Some memory behavior should be editable because the quality bar depends on user taste and workflow. Herald should expose prompts carefully as versioned templates with safe variables, not as a raw prompt-editing trap for every internal instruction.

- [x] Expose prompt templates for memory extraction, track status updates, Compose Radar nudge generation, dossier summarization, Obsidian section formatting, and research-note summarization.
- [x] Keep high-risk guardrail prompts internal, including privacy policy, external research boundaries, evidence requirements, and no-mutation rules.
- [x] Version every exposed prompt template so existing memories can report which prompt generated or updated them.
- [x] Let users reset any exposed prompt to the shipped default.
- [x] Let users test a prompt against a demo fixture or selected source message before saving it.
- [x] Restrict prompt variables to bounded snapshots such as source snippets, evidence metadata, current draft excerpt, configured vault targets, and user-written style preferences.
- [x] Show clear warnings when a custom prompt would weaken evidence discipline, request private-data export, or increase Compose Radar noise.

## Update Rules

Memory updates need predictable rules because stale or overwritten memory is worse than no memory. The default should be conservative: append evidence, update generated sections safely, and ask before overwriting user-authored content.

- [x] New evidence updates an existing memory when it matches the same source thread, person/company, topic, and memory kind above a configurable match threshold.
- [x] Conflicting evidence creates a conflict state instead of silently replacing the older memory.
- [x] User-authored Obsidian sections are never rewritten; only Herald-managed sections between stable markers are updated automatically.
- [x] Resolved open loops move to a resolved state with a source pointer and optional archive note instead of disappearing.
- [x] Stale memories remain visible with a stale label until revalidated, dismissed, archived, or forgotten.
- [x] Dismissed Compose Radar nudges store dismissal scope and do not reappear unless new evidence materially changes the situation.
- [x] Deleted or missing source emails mark dependent memories as source-missing and block them from high-confidence Compose Radar nudges.
- [x] Manual user corrections override generated memory text while keeping source evidence and edit history.
- [x] Daily briefing updates are diffs: changed tracks, newly resolved loops, newly stale loops, failed syncs, and review-needed memories.

## Research Mode

Research Mode should enrich dossiers with public information only when the user asks for it. Private emails should remain local, and external queries should use minimal public identifiers unless the user explicitly approves more context.

- [x] Add explicit actions: research this person, research this company, refresh dossier, and research before reply.
- [x] Build web queries from public identifiers such as name, company, domain, role, and user-provided URL.
- [x] Do not send private email bodies, private notes, attachments, or full thread summaries to external research by default.
- [x] Save research notes with source URLs, retrieval date, confidence, and "what changed since last contact."
- [x] Mark research stale after a configurable interval.
- [x] Distinguish "from your email", "from Obsidian", "from public research", and "inference" in chat answers.

## Architecture Direction

The implementation should add a memory service behind the backend/agent boundary and reuse existing cache, contact, embedding, and Compose review capabilities. It should not create a second chat framework or bypass the existing Backend discipline.

- [x] Add memory storage and retrieval behind backend-facing methods rather than direct SQLite access from Bubble Tea.
- [x] Add extraction jobs that run through the existing AI scheduler with interactive-before-background priority.
- [x] Reuse existing body-cache, classification, contact-enrichment, thread-header, and embedding-cache signals before fetching additional message bodies.
- [x] Ingest opt-in cached calendar events, configured Obsidian note bodies, and saved research notes without broad vault crawling or silent external web research.
- [x] Add memory-aware Gollem tools after the Gollem chat-agent runner is in place.
- [x] Keep external research as a separate opt-in capability from local memory extraction.
- [x] Keep Obsidian sync as an export/sync adapter so the local memory index remains usable without Obsidian.
- [x] Ensure deletion, archive, and cache cleanup can mark memories stale or remove evidence links when source messages disappear.

## Roadmap

The roadmap is ordered so the feature becomes useful before it becomes broad. The first slices should focus on job-search and work-related threads because they have high value, clear statuses, and existing Obsidian folder conventions.

- [x] **M0: Product examples, defaults, and test fixtures** - create realistic demo scenarios plus default memory profiles, prompt templates, and update-rule examples for job-search threads, conflicting timelines, open loops, callbacks, and sent-reply resolution.
- [x] **M1: Local email memories MVP** - extract last contact, last user reply, open questions, commitments, deadlines, people, company, topic, evidence, prompt version, confidence, and stale state from cached Inbox plus Sent messages.
- [x] **M2: Memory-aware chat tools** - add read-only Gollem tools for contact history, company tracks, related replies, open loops, and reply-prep context.
- [x] Track lifecycle assembly derives active, waiting, stale, resolved, backlog, and done track views from immutable source-backed memories.
- [x] **M3: Obsidian sync preview and settings** - configure vault path, memory destinations, Obsidian output profile, update cadence, prompt templates, confidence thresholds, generated sections, and write previews before saving.
- [x] **M4: Compose Radar v1** - surface source-backed nudges for job-search replies and high-confidence people callbacks, with open/dismiss/insert actions.
- [x] Compose Radar refreshes reply-prep nudges after relevant draft context changes without letting stale results replace newer context.
- [x] Compose Radar nudges carry typed conflict/callback/open-loop/relationship/research/draft-risk states plus action-state and dismissal-scope metadata.
- [x] **M5: Dossier views** - enrich Contacts and company/thread detail views with relationship summaries, active tracks, open loops, vault links, and evidence.
- [x] Person dossier v1 appears inside Contacts detail as a bounded, source-backed, read-only section built from immutable local memory records.
- [x] Company dossier v1 appears inside Contacts detail as a bounded, source-backed, read-only section for company/domain-backed tracks, open loops, vault links, and evidence.
- [x] Thread dossier v1 appears in email preview as a bounded, source-backed, read-only section for the selected thread, using canonical note links instead of copying full dossier summaries.
- [x] **M6: Research Mode** - add explicit person/company research, sourced research notes, freshness checks, and "research before reply."
- [x] **M7: Daily memory briefing** - produce a diff over changed tracks, resolved questions, stale loops, and vault hygiene items.
- [x] **M8: Hardening and privacy controls** - add forget, pin, correct, source audit, update-rule audit, retention settings, prompt reset, and deletion propagation.

## First Shippable Slice

The first implementation should make one narrow scenario feel excellent instead of trying to remember every mailbox. Job-search Compose Radar is the recommended slice because it uses Inbox, Sent, Contacts, and Obsidian notes in a way users can immediately judge.

- [x] Focus on job-search threads in `Job search/active` plus related Inbox and Sent messages.
- [x] Use shipped defaults for destination folders, Obsidian output profile, prompt templates, update cadence, and confidence thresholds before exposing advanced tuning.
- [x] Detect "already replied", "awaiting response", "deadline", "timeline mismatch", and "relationship callback" nudges.
- [x] Show nudges only in reply Compose and only when evidence is strong.
- [x] Let the user inspect the source email, Obsidian note, research URL, or compact evidence pointer from each nudge.
- [x] Write no Obsidian changes until sync preview is approved.
- [x] Treat uncertain claims as chat/search suggestions, not Compose warnings.

## Error Handling And Safety

Memory features can become creepy or noisy if they overclaim. The system should fail quiet, cite sources, and give the user direct control over corrections and deletion.

- [x] No evidence means no factual memory answer.
- [x] Low-confidence memories remain searchable but do not become Compose Radar nudges.
- [x] Provider or local-model failure leaves Compose usable and shows a concise memory-unavailable state.
- [x] Obsidian write conflicts show a preview and never overwrite user sections silently.
- [x] External research failure keeps local dossiers usable.
- [x] Deleted or missing source emails mark dependent memories stale until revalidated.
- [x] User-dismissed nudges respect dismissal scope: this draft, this thread, this person, or permanently.

## Testing And Verification

Testing should start with deterministic fixtures and then graduate to tmux-visible Compose behavior. Live private mail should not be required to prove the feature.

- [x] Update `engineering/testplans/TUI_TESTPLAN.md` with memory chat, Compose Radar, Contacts dossier, and Obsidian sync cases before implementation.
- [x] Unit-test memory extraction on synthetic inbound and sent messages.
- [x] Unit-test cache-backed extraction metadata for classifications, contacts, body-cache presence, and embedding-cache presence.
- [x] Unit-test track status transitions for active, waiting, stale, resolved, backlog, and done.
- [x] Unit-test evidence source-type validation and snippet bounding for email, sent replies, notes, calendar events, attachments, and research URLs.
- [x] Unit-test daily briefing diff generation, configured Obsidian destination paths, failed syncs, and bounded non-recap rendering.
- [x] Unit-test deletion propagation and stale-memory behavior.
- [x] Unit-test Obsidian Markdown generation with frontmatter and generated-section preservation.
- [x] App-level tests prove Compose Radar retrieval does not mutate draft content without user action.
- [x] Snapshot or tmux tests cover Compose Radar at `220x50`, `80x24`, and `50x15`.
- [x] Demo fixtures include enough job-search and relationship examples for memory chat, Compose Radar, and Contacts dossier screenshots.

## Non-Goals For First Execution

The first implementation should avoid autonomous behavior and broad private-data export. These items can be revisited after local extraction, source evidence, Compose Radar, and Obsidian sync are trustworthy.

- [x] No automatic send, delete, archive, calendar mutation, or reply scheduling from memory features.
- [x] No silent external research.
- [x] No external upload of private email bodies by default.
- [x] No whole-mailbox unbounded summarization.
- [x] No automatic rewrite of user-authored Obsidian sections.
- [x] No replacement for the existing scheduled task system until the daily memory briefing has proven value.
- [x] No MCP or daemon memory API until the UI-only path is stable.

## Execution Handoff

When this roadmap moves into implementation planning, start with the job-search Compose Radar slice after the Gollem runner has replaced the legacy chat runtime. The implementation plan should preserve non-chat AI features and keep memory extraction read-only until the Obsidian sync preview is accepted.

- [x] First plan should cover M0 through M2 if Gollem is already available, or defer memory-aware chat until Gollem M1-M3 land.
- [x] First plan should explicitly name the memory storage tables, backend methods, extraction jobs, and fixture data it introduces.
- [x] First plan should include degradation behavior for no AI, slow AI, no Obsidian vault, no Sent cache, and missing source evidence.
- [x] First plan should keep Compose Radar insertion inside existing Compose review/accept mechanics.
- [x] First plan should update public docs only after demo fixtures can show the feature without private mailbox data.
