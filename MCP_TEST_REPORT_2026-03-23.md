## Test Run — 2026-03-23 — MCP server (cmd/mcp-server)

**Plan:** MCP_TESTPLAN.md
**Binary:** built from HEAD, output to `/tmp/mcp-server-test`
**Method:** raw JSON-RPC via stdin
**Cache:** populated (INBOX ~5100 emails, AI classifications not run)

---

### Bugs

None found.

---

### UX Issues

| ID | TC     | Description                                                                                        | Suggestion                                                                              |
|----|--------|----------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|
| U1 | MCP-01 | All 4 tools have `destructiveHint: true` and `readOnlyHint: false` in MCP annotations, but all tools are read-only (SELECT only, no DB writes). | Set `readOnlyHint: true` and `destructiveHint: false` on all four tool registrations.  |
| U2 | MCP-04 | Unknown folder returns `"Recent emails in DoesNotExist (0 results):\n\n"` with no further hint — indistinguishable from a real empty folder. | Append `"(folder not found or empty)"` when result count is 0.                         |

---

### All Good

- **TC-MCP-01** Server starts and registers tools: PASS — 4 tools registered (`list_recent_emails`, `search_emails`, `get_sender_stats`, `get_email_classifications`), each with `description` and `inputSchema`. No error field.
- **TC-MCP-02** list_recent_emails basic: PASS — 20 results, date/sender/subject columns, newest-first order.
- **TC-MCP-03** list_recent_emails with limit: PASS — exactly 5 rows returned.
- **TC-MCP-04** list_recent_emails unknown folder: PASS (with U2 note) — valid JSON-RPC, no crash, 0 results.
- **TC-MCP-05** search_emails by sender: PASS — 70 results for `nerdwallet`, all from `*nerdwallet*` senders, no unrelated senders.
- **TC-MCP-06** search_emails by subject keyword: PASS — 100 results for `receipt`, mixed senders; case-insensitive match confirmed.
- **TC-MCP-07** search_emails no results: PASS — returns `"No emails found matching \"xyzzy_no_match_12345\" in INBOX"`, valid JSON, no crash.
- **TC-MCP-08** search_emails special characters (SQL safety): PASS — `escapeLike()` in `internal/cache/cache.go:342` escapes `%` → `\%` and uses `ESCAPE '\'`. Query for `%` returned only emails with literal `%` in subject (verified all 100 results). Backslash query returned 0-result message. No SQL error, no panic.
- **TC-MCP-09** get_sender_stats basic: PASS — 20 senders, count-descending order (top: USPS 131, Finish Line 129, Quicken 99).
- **TC-MCP-10** get_sender_stats with top_n: PASS — exactly 3 rows, same ordering as TC-MCP-09.
- **TC-MCP-11** get_email_classifications with data: SKIP — prerequisite not met (AI classifier not run; `email_classifications` table empty). Response was the valid "no classifications" message confirming no crash on unclassified INBOX.
- **TC-MCP-12** get_email_classifications with no data: PASS — returns `"No classifications found for Sent. Open the TUI and press 'a' to classify."` — valid JSON, helpful guidance, no crash.
- **TC-MCP-13** Missing cache file: PASS — server exits cleanly before accepting input with `"Failed to load config: config file permission check failed: stat /nonexistent/proton.yaml: no such file or directory"` to stderr, exit code 1. No panic, no stack trace.
- **TC-MCP-14** Claude Code integration: NOT RUN — manual step requiring settings.json config and Claude Code restart.
