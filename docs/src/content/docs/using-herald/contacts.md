---
title: Contacts
description: Browse contacts, inspect recent mail, preview messages, search, and run enrichment.
---

Contacts turns the senders and imported address book data Herald knows about into a navigable tab. It is useful for finding a person or company, reviewing recent mail from that contact, opening an inline preview, and enriching contact metadata.

## Overview

Press `4` to open Contacts. Herald loads contacts from the backend and, on macOS, can import Apple Contacts at startup. Contacts can be searched by keyword or semantic similarity when AI/embeddings are available.

## Screen Anatomy

| Area | What it shows |
| --- | --- |
| Contact list panel | Contact count and rows for known contacts. Wide terminals show Name, Email, Company, and Count; narrower terminals reduce columns. |
| Detail panel | Selected contact identity, company/topics when available, and recent email list. |
| Recent email list | Up to 20 recent messages for the selected contact. |
| Inline preview | Body preview for the selected recent email. |
| Keyword search | `/` prompt and filtered contact list. |
| Semantic search | `?` prompt and AI/embedding-backed contact results. |
| Status and hints | Contact load/enrichment messages and contact-specific key hints. |

<!-- HERALD_SCREENSHOT id="contacts-main-list" page="contacts" alt="Contacts tab list and detail panels" state="demo mode, 120x40, Contacts tab active" desc="Shows contact list columns, empty or selected detail panel, contact count, and key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 4" -->

## Controls

| Key | Context | Preconditions | Result |
| --- | --- | --- | --- |
| `/` | Contacts list | Search mode closed. | Opens keyword search. |
| `?` | Contacts list | Search mode closed. | Opens semantic search. |
| Printable text | Search mode | Keyword or semantic search active. | Updates the search query and filtered list. |
| `backspace` / `ctrl+h` | Search mode | Search query has characters. | Deletes one character and reapplies search. |
| `enter` | Search mode | Search active. | Confirms current results and closes search input. |
| `esc` | Search mode | Search active. | Clears search and restores full contact list. |
| `j` / `down` | List panel | List focused. | Moves to next contact. |
| `k` / `up` | List panel | List focused. | Moves to previous contact. |
| `enter` | List panel | Contact row selected. | Opens contact detail and loads recent emails. |
| `tab` | Contacts | Detail is open. | Toggles focus between list and detail panels. |
| `j` / `down` | Detail panel | Recent email list focused. | Moves to next recent email. |
| `k` / `up` | Detail panel | Recent email list focused. | Moves to previous recent email. |
| `enter` | Detail panel | Recent email selected. | Opens inline email preview. |
| `esc` | Preview/detail | Preview or detail active. | Closes preview first, then clears detail and search state. |
| `e` | Contacts | A contact is selected or detail is open. | Runs single-contact AI enrichment. |

## Workflows

### Open a Contact Detail

1. Press `4`.
2. Move through contacts with `j`/`k`.
3. Press `enter`.
4. Review contact metadata and recent messages.

### Preview a Recent Email

1. Open a contact detail.
2. Press `tab` to focus the detail panel.
3. Use `j`/`k` to choose a recent email.
4. Press `enter`.
5. Press `esc` to return to contact detail.

### Search Contacts

1. Press `/` for keyword search or `?` for semantic search.
2. Type a name, email, company, topic, or concept.
3. Press `enter` to keep the filtered results.
4. Press `esc` to clear search.

### Enrich a Contact

1. Select a contact row or open detail.
2. Press `e`.
3. Wait for status to report enrichment progress or completion.
4. Review new company/topic details when available.

## States

| State | What happens |
| --- | --- |
| Loading contacts | Contacts tab waits for backend contact rows and displays status. |
| Empty contacts | List panel shows no contacts; open mail and sync folders to collect senders. |
| Keyword search | Search filters local contact fields while typing. |
| Semantic search | Search uses embeddings/AI availability; if unavailable, results may stay empty or status reports unavailable. |
| Detail loading | Recent emails load after `enter` on a contact. |
| Inline preview loading | Body fetch starts for the selected recent email. |
| AI unavailable | `e` cannot enrich contact metadata. |
| Narrow terminal | Contact columns compress; detail remains accessible through `tab` when open. |

## Data And Privacy

Contacts reads and stores sender addresses, display names, company/topic metadata, message counts, recent email references, and optional embeddings. On macOS, Apple Contacts import can add address book contacts through the backend. Enrichment sends selected contact and email-derived context to the configured AI backend.

## Troubleshooting

If Contacts is empty, sync at least one folder in Timeline and reopen Contacts.

If semantic search does not return expected results, confirm AI embeddings are configured and that enrichment or embedding processing has run.

If inline preview seems stale, close with `esc`, move to the message again, and press `enter` to refetch.

## Screenshot Placeholders

<!-- HERALD_SCREENSHOT id="contacts-detail" page="contacts" alt="Contact detail with recent emails" state="demo mode, 120x40, contact detail open" desc="Shows selected contact metadata, recent emails, detail focus behavior, and key hints." capture="tmux demo 120x40; ./bin/herald --demo; press 4; press enter" -->

<!-- HERALD_SCREENSHOT id="contacts-keyword-search" page="contacts" alt="Contacts keyword search mode" state="demo mode, 120x40, keyword search active" desc="Shows slash search prompt, filtered contact list, search text in hints, and clear behavior." capture="tmux demo 120x40; ./bin/herald --demo; press 4; press /; type demo" -->

<!-- HERALD_SCREENSHOT id="contacts-inline-preview" page="contacts" alt="Contact recent email inline preview" state="demo mode, 120x40, contact detail email preview" desc="Shows recent email preview loading or body text inside the detail panel and Esc return behavior." capture="tmux demo 120x40; ./bin/herald --demo; press 4; press enter; press tab; press enter" -->

## Related Pages

- [Compose](/using-herald/compose/)
- [Search](/features/search/)
- [AI Features](/features/ai/)
- [Sync and Status](/features/sync-status/)
