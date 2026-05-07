# Demo Onboarding Emails Design

## Purpose

Demo mode should teach Herald from inside the mailbox itself. The first visible Timeline messages should start with a welcome note and then read as a numbered example course from Herald, so a new user can open `--demo`, learn what Herald is, and then try the core workflows without leaving the TUI.

- [ ] Replace the current top-of-inbox demo story with explicit Herald onboarding messages.
- [ ] Make the onboarding sequence feel like email documentation, not a marketing page or generic synthetic inbox.
- [ ] Keep demo mode offline, deterministic, fictional, and safe.

## Scope

This design covers the original demo email list and body content strategy only. It does not implement the fixture changes yet, but it defines the accepted sequence, ordering rules, and verification expectations for the implementation plan.

- [ ] Add one primary welcome message and eight primary onboarding example messages to the demo Timeline.
- [ ] Use Herald-branded senders for every onboarding message.
- [ ] Order the messages exactly as Welcome, then Example 1 through Example 8 at the top of the demo Timeline.
- [ ] Include concrete keypress instructions and a short explanation of the underlying feature in every message body.
- [ ] Preserve or replace existing fixture coverage for attachments, inline images, HTML rendering, cleanup grouping, AI classification, semantic search, contacts, and MCP demo behavior.

## Onboarding Sequence

The sequence should be explicit and numbered after a short welcome message. The welcome subject should be `✉ Welcome to Herald`; Herald keeps that stable envelope glyph while still stripping arbitrary emoji from Timeline table subjects. Each example email should have a subject that starts with `Example N:` and a sender in the form `Herald <role> <local-part@herald.demo>`.

- [ ] `Herald Welcome <welcome@herald.demo>` sends `✉ Welcome to Herald`.
- [ ] The welcome email explains Herald as a terminal email client for keyboard navigation, inbox cleanup, rich previews, and AI-assisted triage, and it states that demo mode is synthetic and safe.
- [ ] `Herald Guide <guide@herald.demo>` sends `Example 1: Move around your inbox`.
- [ ] Example 1 covers Timeline navigation with `j/k`, arrow keys, `Enter` or right-arrow preview, `Esc`, `1/2/3` tabs, `f` folders, `/` search, and `?` shortcut help.
- [ ] `Herald Compose Coach <compose@herald.demo>` sends `Example 2: Reply, write, and preview Markdown`.
- [ ] Example 2 covers `R` reply, Compose, Markdown writing, `ctrl+p` preview, and `ctrl+s` demo sending.
- [ ] Example 2 explains that replies and forwards preserve original formatting, inline images, and attachments where possible, and that Herald sends Markdown as rendered HTML with a plain-text alternative.
- [ ] `Herald Attachments <attachments@herald.demo>` sends `Example 3: Open and save attachments`.
- [ ] Example 3 includes at least two attachments and covers attachment markers, the preview attachment list, `[` / `]` selection, `s` save, and the save path prompt.
- [ ] `Herald Image Lab <images@herald.demo>` sends `Example 4: View inline images in full screen`.
- [ ] Example 4 retains the Creative Commons sampler behavior with embedded images, `z` full-screen mode, Kitty/iTerm2 rendering, safe fallback links or placeholders, and no remote image fetching.
- [ ] `Herald Cleanup Coach <cleanup@herald.demo>` sends `Example 5: Clean up senders and domains safely`.
- [ ] Example 5 covers Cleanup tab `3`, sender and domain grouping, `space` selection, delete/archive actions, unsubscribe hints, and preview-before-action behavior.
- [ ] `Herald AI Rules <rules@herald.demo>` sends `Example 6: Classify mail and dry-run rules`.
- [ ] Example 6 covers `a` classification, deterministic demo AI, semantic search with a sample query such as `? infrastructure budget risk`, cleanup rules `C`, automation rules `W`, prompts `P`, and dry-run previews.
- [ ] `Herald Settings <settings@herald.demo>` sends `Example 7: Configure accounts, AI, and signatures`.
- [ ] Example 7 covers the `S` settings overlay, provider configuration, local/Ollama/OpenAI-compatible AI settings, embedding model choice, and email signatures.
- [ ] `Herald Next Steps <next@herald.demo>` sends `Example 8: Explore contacts, chat, SSH, and MCP`.
- [ ] Example 8 covers Contacts, chat panel `c`, quick replies, `herald mcp --demo`, `herald ssh`, and suggested practice searches.

## Message Body Pattern

Each message body should be concise enough to read in split preview while still rewarding full-screen reading. The bodies should teach what to press, what the user should notice, and why the feature matters.

- [ ] Start every example body with a one-sentence purpose for the example.
- [ ] Include a `Try now` section with concrete actions the user can perform immediately.
- [ ] Include a `What Herald is doing` section that explains the feature behavior in plain language.
- [ ] Prefer short paragraphs and compact lists over long manual-style prose.
- [ ] Use realistic demo content where a feature needs it, such as attachments on Example 3 and inline image attribution on Example 4.
- [ ] Avoid private identity terms, real vendor brands, lorem ipsum, and any implication that demo mode is connected to a real mailbox.

## Fixture Strategy

The onboarding messages should become the primary demo experience, but the demo data still needs enough structure to exercise Herald's public surfaces. Supporting messages may remain or be replaced as long as they do not compete with the Welcome plus Example 1-8 course at the top of Timeline.

- [ ] Give the welcome and eight onboarding example messages the newest demo dates so Timeline sorting places them first.
- [ ] Keep supporting fixture messages older than the onboarding sequence.
- [ ] Use Herald-branded senders for the onboarding messages; supporting practice fixtures may be kept only when they are needed for feature coverage.
- [ ] Ensure Cleanup has multiple messages from a sender or domain so sender/domain grouping remains meaningful.
- [ ] Ensure Contacts remains populated and can open recent emails inline.
- [ ] Ensure deterministic demo AI categories and semantic topic vectors still return meaningful results.
- [ ] Ensure MCP demo tools read the same updated mailbox fixtures.

## Testing

Testing should prove the mailbox now functions as onboarding documentation while preserving the existing demo coverage. The implementation plan should update tests before changing fixtures.

- [ ] Add fixture tests that assert the welcome email and Example 1 through Example 8 exist, are ordered newest-to-oldest, and use Herald senders.
- [ ] Add body-content tests for the key instructional promises: navigation, reply/Markdown, attachment saving, image preview, cleanup, AI rules, settings, and next steps.
- [ ] Update existing tests that hard-code old demo subjects, especially the Creative Commons image sampler subject if Example 4 renames it.
- [ ] Update `TUI_TESTPLAN.md` TC-46 to expect onboarding emails as the primary public demo context.
- [ ] Run Go tests for demo fixtures, demo backend behavior, and MCP demo behavior.
- [ ] Run tmux demo checks at `220x50`, `80x24`, and `50x15`, saving the report in `reports/`.
- [ ] If implementation touches demo docs or media, update the relevant README/docs copy and regenerate affected demo screenshots or GIFs.

## Out Of Scope

This change should not redesign the TUI or add a separate onboarding system. The mailbox content itself is the onboarding surface.

- [ ] Do not add a new onboarding modal or wizard.
- [ ] Do not change normal IMAP mailbox behavior.
- [ ] Do not make demo mode write or send real mail.
- [ ] Do not fetch remote images for the image step.
- [ ] Do not implement the first-run `Try without an account` wizard entry as part of this work.
