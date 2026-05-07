# Demo Onboarding Emails Design

## Purpose

Demo mode should teach Herald from inside the mailbox itself. The first visible Timeline messages should start with a welcome note and then read as a numbered step-by-step course from Herald, so a new user can open `--demo`, learn what Herald is, and then try the core workflows without leaving the TUI.

- [ ] Replace the current top-of-inbox demo story with explicit Herald onboarding messages.
- [ ] Make the onboarding sequence feel like email documentation, not a marketing page or generic synthetic inbox.
- [ ] Keep demo mode offline, deterministic, fictional, and safe.

## Scope

This design covers the original demo email list and body content strategy only. It does not implement the fixture changes yet, but it defines the accepted sequence, ordering rules, and verification expectations for the implementation plan.

- [ ] Add one primary welcome message and nine primary onboarding step messages to the demo Timeline.
- [ ] Use Herald-branded senders for every onboarding message.
- [ ] Order the messages exactly as Welcome, then Step 1 through Step 9 at the top of the demo Timeline.
- [ ] Include concrete keypress instructions and a short explanation of the underlying feature in every message body.
- [ ] Preserve or replace existing fixture coverage for attachments, inline images, HTML rendering, cleanup grouping, AI classification, semantic search, contacts, and MCP demo behavior.

## Demo Welcome Overlay

Demo mode may introduce the mailbox with a compact centered overlay before the user starts reading the course. This overlay is an additive orientation layer: the inbox itself remains the onboarding source of truth, and dismissing the overlay returns the user to the first Timeline email.

- [ ] Show the welcome overlay only in TUI `--demo` mode, never in normal IMAP sessions or MCP demo tools.
- [ ] Explain that the mailbox is synthetic and safe, that the first Timeline email starts the onboarding guide, and that the user can dismiss the overlay to explore freely.
- [ ] Dismiss the overlay with `Esc`, `Space`, or `Enter` without also opening a message, toggling selection, or recording the dismiss key in the demo keypress overlay.
- [ ] Swallow other keys while the overlay is visible so accidental navigation does not change the underlying Timeline.
- [ ] Keep `q` and `ctrl+c` available to quit from the overlay.

## Onboarding Sequence

The sequence should be explicit and numbered after a short welcome message. The welcome subject should be `✉ Welcome to Herald`; Herald keeps that stable envelope glyph while still stripping arbitrary emoji from Timeline table subjects. Each onboarding email should have a subject that starts with `Step N:` and a sender in the form `Herald <role> <local-part@herald.demo>`. Supporting fixture emails below the course should use `Example:` subjects, such as `Example: Link rendering stress preview`, so they read as practice data rather than additional onboarding steps.

- [ ] `Herald Welcome <welcome@herald.demo>` sends `✉ Welcome to Herald`.
- [ ] The welcome email explains Herald as a terminal email client for keyboard navigation, inbox cleanup, rich previews, and AI-assisted triage, and it states that demo mode is synthetic and safe.
- [ ] `Herald Guide <guide@herald.demo>` sends `Step 1: Move around your inbox`.
- [ ] Step 1 covers vertical movement with `j/k` or up/down arrows, horizontal panel movement with `h/l`, left/right arrows, `Tab`, and `Shift+Tab`, preview opening with `Enter`, right arrow, `l`, or `Tab`, mouse scrolling/clicking, `Esc`, `1/2/3` tabs, `f` folders, `/` search, and `?` shortcut help.
- [ ] `Herald Compose Coach <compose@herald.demo>` sends `Step 2: Reply, write, and preview Markdown`.
- [ ] Step 2 covers `R` reply, Compose, Markdown writing, `ctrl+p` preview, and `ctrl+s` demo sending.
- [ ] Step 2 explains that replies and forwards preserve original formatting, inline images, and attachments where possible, and that Herald sends Markdown as rendered HTML with a plain-text alternative.
- [ ] `Herald Attachments <attachments@herald.demo>` sends `Step 3: Open and save attachments`.
- [ ] Step 3 includes at least two attachments and covers attachment markers, the preview attachment list, `[` / `]` selection, `s` save, and the save path prompt.
- [ ] `Herald Selection Coach <selection@herald.demo>` sends `Step 4: Select text from an email`.
- [ ] Step 4 covers terminal text selection, full-screen preview `z`, mouse capture, `m` to release/restore mouse handling, and why mouse capture is enabled by default.
- [ ] `Herald Image Lab <images@herald.demo>` sends `Step 5: View inline images in full screen`.
- [ ] Step 5 retains the Creative Commons sampler behavior with embedded images, `z` full-screen mode, Kitty/iTerm2 rendering, safe fallback links or placeholders, and no remote image fetching.
- [ ] `Herald Cleanup Coach <cleanup@herald.demo>` sends `Step 6: Clean up senders and domains safely`.
- [ ] Step 6 covers Cleanup tab `3`, sender and domain grouping, `space` selection, delete/archive actions, unsubscribe hints, and preview-before-action behavior.
- [ ] `Herald AI Rules <rules@herald.demo>` sends `Step 7: Classify mail and dry-run rules`.
- [ ] Step 7 covers `a` classification, deterministic demo AI, semantic search with a sample query such as `? infrastructure budget risk`, cleanup rules `C`, automation rules `W`, prompts `P`, dry-run previews, and explanations of what rules and custom prompts are for.
- [ ] `Herald Settings <settings@herald.demo>` sends `Step 8: Configure accounts, AI, and signatures`.
- [ ] Step 8 covers the `S` settings overlay, provider configuration, local/Ollama/OpenAI-compatible AI settings, embedding model choice, and email signatures.
- [ ] `Herald Next Steps <next@herald.demo>` sends `Step 9: Explore contacts, chat, SSH, and MCP`.
- [ ] Step 9 covers Contacts, chat panel `c`, quick replies, `herald mcp --demo`, `herald ssh`, and suggested practice searches.

## Message Body Pattern

Each message body should be concise enough to read in split preview while still rewarding full-screen reading. The bodies should teach what to press, what the user should notice, and why the feature matters.

- [ ] Start every onboarding step body with a one-sentence purpose for the step.
- [ ] Include a `Try now` section with concrete actions the user can perform immediately.
- [ ] Include a `What Herald is doing` section that explains the feature behavior in plain language.
- [ ] Prefer short paragraphs and compact lists over long manual-style prose.
- [ ] Use realistic demo content where a feature needs it, such as attachments on Step 3 and inline image attribution on Step 5.
- [ ] Avoid private identity terms, real vendor brands, lorem ipsum, and any implication that demo mode is connected to a real mailbox.

## Fixture Strategy

The onboarding messages should become the primary demo experience, but the demo data still needs enough structure to exercise Herald's public surfaces. Supporting messages may remain or be replaced as long as they do not compete with the Welcome plus Step 1-9 course at the top of Timeline.

- [ ] Give the welcome and nine onboarding step messages the newest demo dates so Timeline sorting places them first.
- [ ] Keep supporting fixture messages older than the onboarding sequence.
- [ ] Use Herald-branded senders for the onboarding messages; supporting practice fixtures may be kept only when they are needed for feature coverage.
- [ ] Rename supporting practice fixtures with `Example:` subjects and keep the mailbox focused by removing repetitive promo, statement, receipt, newsletter, and alert duplicates.
- [ ] Ensure Cleanup has multiple messages from a sender or domain so sender/domain grouping remains meaningful.
- [ ] Ensure Contacts remains populated and can open recent emails inline.
- [ ] Ensure deterministic demo AI categories and semantic topic vectors still return meaningful results.
- [ ] Ensure MCP demo tools read the same updated mailbox fixtures.

## Testing

Testing should prove the mailbox now functions as onboarding documentation while preserving the existing demo coverage. The implementation plan should update tests before changing fixtures.

- [ ] Add fixture tests that assert the welcome email and Step 1 through Step 9 exist, are ordered newest-to-oldest, and use Herald senders.
- [ ] Add body-content tests for the key instructional promises: navigation, reply/Markdown, attachment saving, text selection, image preview, cleanup, AI rules, settings, and next steps.
- [ ] Update existing tests that hard-code old demo subjects, especially the Creative Commons image sampler subject if Step 5 renames it.
- [ ] Update `TUI_TESTPLAN.md` TC-46 to expect onboarding emails as the primary public demo context.
- [ ] Run Go tests for demo fixtures, demo backend behavior, and MCP demo behavior.
- [ ] Run tmux demo checks at `220x50`, `80x24`, and `50x15`, saving the report in `reports/`.
- [ ] If implementation touches demo docs or media, update the relevant README/docs copy and regenerate affected demo screenshots or GIFs.

## Out Of Scope

This change should not redesign the TUI or add a separate persistent onboarding system. The mailbox content itself remains the onboarding surface, with the demo welcome overlay serving only as a dismissible entry point.

- [ ] Do not add a new persistent onboarding wizard.
- [ ] Do not change normal IMAP mailbox behavior.
- [ ] Do not make demo mode write or send real mail.
- [ ] Do not fetch remote images for the image step.
- [ ] Do not implement the first-run `Try without an account` wizard entry as part of this work.
