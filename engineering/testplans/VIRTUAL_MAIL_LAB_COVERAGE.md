# Virtual Mail Lab Coverage Matrix

This matrix records how `internal/testmail` scenarios map to Herald verification surfaces. Use it before adding new virtual-lab tests so realistic fixtures stay reusable instead of being rediscovered one slice at a time.

Report-template surfaces that may appear in test reports: `demo`, `virtual lab`, `live config`, `tmux`, `ttyd`, `SSH`, `MCP`, `daemon`.

## Scenario Coverage

Every committed scenario is loaded through the virtual IMAP path and validated by the corpus tests. Surface notes are intentionally conservative: "direct" means a named scenario is used by that surface, while "lab helper" means the virtual-lab infrastructure is covered by runtime-generated mail rather than that exact fixture.

| Scenario | Fixture shape | IMAP and LocalBackend | Render and TUI | SSH | MCP and daemon | ttyd or image evidence |
|----------|---------------|-----------------------|----------------|-----|----------------|------------------------|
| `plain-thread` | Bob original plus Alice reply across Alice and Bob folders | Direct scenario seed and backend body-fetch coverage | No dedicated preview assertion; thread placement is covered in `internal/testmail` | Not direct | Lab helper for send/reply concepts | Not direct |
| `calendly-invite` | `text/calendar` invite with meeting-like HTML | Direct scenario seed and backend body-fetch coverage | Direct preview and size coverage | Direct SSH preview and resize coverage | Direct cache-backed MCP read coverage | Not direct |
| `newsletter-table` | Table-heavy newsletter HTML | Direct scenario seed and backend body-fetch coverage | Direct preview and size coverage | Not direct | Not direct | Not direct |
| `receipt-html` | Transactional receipt HTML | Direct scenario seed and backend body-fetch coverage | Direct preview coverage | Not direct | Not direct | Not direct |
| `malformed-charset` | Plaintext fallback for malformed charset | Direct scenario seed and backend body-fetch coverage | Direct preview and size coverage | Not direct | Not direct | Not direct |
| `inline-cid-image` | Multipart HTML with inline CID image | Direct scenario seed and backend body-fetch coverage | Direct preview, full-screen, fallback-link, placeholder, and forced iTerm2-mode coverage | Not direct | Not direct | Go image-mode assertions direct; browser pixels stay demo-backed |
| `remote-html-images` | Sanitized Buttondown-style HTML with linked remote images | Direct scenario seed and backend body-fetch coverage | Direct split/full-screen placeholder and reveal-hint coverage | Not direct | Not direct | tmux evidence for demo-backed reveal; ttyd remains demo-backed |
| `long-link-tracking` | HTML with long safe links and tracking-like noise | Direct scenario seed and backend body-fetch coverage | Direct preview and size coverage | Direct SSH resize and leak-check coverage | Not direct | Not direct |
| `unsubscribe-headers` | One-click, mailto, and absent unsubscribe headers | Direct scenario seed and backend header/body coverage | Direct Timeline and Cleanup hint coverage | Not direct | Lab helper for daemon/MCP execution with dynamic local one-click URL | Not direct |

## Focused Commands

Use these commands when changing virtual-lab scenarios, docs, or the relevant surface. They are narrower than `go test ./...` and should run before broader gates.

| Lane | Command |
|------|---------|
| Scenario and corpus | `go test ./internal/testmail -run 'Scenario|Corpus'` |
| Sanitizer gate | `go run ./tools/testmail-sanitize -validate internal/testmail/testdata/corpus` |
| SMTP router and backend flows | `go test ./internal/testmail ./internal/backend -run 'LabRoutesSMTP|Virtual|Draft|Reply|Send|Scenario|Corpus'` |
| App preview and TUI surface | `go test ./internal/app -run 'VirtualLab|Scenario|Preview|MinimumSize|InlineCID|Unsubscribe|HideFuture'` |
| SSH virtual-lab surface | `go test ./internal/sshserver -run VirtualLab` |
| MCP cache-backed read surface | `go test ./internal/mcpserver -run 'VirtualLab|DemoMCP'` |
| Daemon mutation surface | `go test ./internal/daemon -run 'VirtualLab|Draft|Reply|Send|Forward|Attachment|Archive|Delete|Move|Unread|Read|Bulk|Unsubscribe|Soft'` |
| MCP daemon mutation surface | `go test ./internal/mcpserver -run 'VirtualLab|Daemon|Draft|Reply|Send|Forward|Attachment|Archive|Delete|Move|Unread|Read|Bulk|Unsubscribe|Soft|MissingDaemon'` |
| ttyd image harness contract | `go test ./tools/ttyd-image-harness` |
| Full package gate | `go test ./internal/repoassert ./internal/testmail ./internal/backend ./internal/app ./internal/sshserver ./internal/daemon ./internal/mcpserver` |

## Boundary Notes

These boundaries prevent false confidence and keep verification budgets proportional. They should be copied into reports when a change touches the corresponding lane.

- `virtual lab` proves deterministic IMAP, SMTP, MIME, cache, backend, TUI, SSH, MCP, and `daemon` paths without private mail.
- `demo` remains the polished presentation lane and the source for default browser-pixel `ttyd` evidence.
- `live config` remains necessary for provider-specific behavior such as OAuth, Bridge quirks, folder naming, throttling, or production IMAP/SMTP differences.
- `tmux` is still the preferred text-layout lane for manual TUI evidence, but it is not required for docs-only virtual-lab matrix changes.
- `ttyd` proves browser-visible raster evidence for the demo image sampler; v1 does not run a virtual-lab ttyd process.
