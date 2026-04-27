# OSC 8 Prerequisites Documentation Design

## Purpose

Herald's beta link rendering now uses hardened OSC 8 terminal hyperlinks so long URLs can stay clickable without corrupting adjacent terminal layout. The documentation should set expectations early: users get the best hyperlink experience in terminals that support OSC 8, while Herald still remains usable elsewhere.

## Scope

This change is intentionally narrow: it updates installation-adjacent documentation only. It does not change link rendering behavior, terminal detection, or any test fixtures.

- [ ] Add a concise OSC 8 prerequisite/recommendation note to the top-level `README.md` Quick Start area.
- [ ] Add a matching note to `docs/src/content/docs/getting-started.md` under `## Requirements`.
- [ ] Mention a few common compatible terminals, including iTerm2, Kitty, WezTerm, GNOME Terminal or other VTE-based terminals, and Windows Terminal.
- [ ] Link the phrase describing the full compatibility list to `https://github.com/Alhadis/OSC8-Adoption/`.
- [ ] Keep the wording clear that OSC 8 support improves clickable terminal links, not that it is required to launch Herald.

## Approach

Use short prerequisite bullets rather than a new compatibility page. This keeps install docs scannable while still surfacing the beta hyperlink hardening where users decide whether their terminal is ready.

## Affected Files

The implementation should touch only the two user-facing entrypoint documents where prerequisites belong. Keeping the file set small avoids turning a beta note into a broader terminal compatibility rewrite.

- [ ] `README.md`
- [ ] `docs/src/content/docs/getting-started.md`

## Testing

This is a documentation-only change, so verification should focus on rendered Markdown hygiene rather than Go behavior. A targeted diff review is enough unless the docs site build is already being run for nearby changes.

- [ ] Review the changed Markdown for accurate links, concise wording, and no broken heading conventions.
- [ ] Run a targeted diff review after editing.
