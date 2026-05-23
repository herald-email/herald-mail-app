# Demo Onboarding Emails Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make demo mode open with eight Herald-authored onboarding emails that teach the main TUI workflows from inside the inbox.

**Architecture:** The shared mailbox fixtures in `internal/demo` remain the source of truth for TUI demo mode, deterministic demo AI, and MCP demo mode. The implementation adds the onboarding messages as the newest fixture emails, updates tests that refer to the old image sampler subject, and keeps existing supporting fixtures for cleanup, search, contacts, threads, and rules coverage.

**Tech Stack:** Go fixtures and tests, Bubble Tea demo backend data, MCP demo fixture readers, Markdown documentation.

---

## File Map

- Modify: `internal/demo/fixtures_test.go` - add TDD coverage for ordered Step 1 through Step 8 emails and instructional body content.
- Modify: `internal/demo/fixtures.go` - add onboarding messages, date override helper, and unique demo attachment part paths.
- Modify: `internal/backend/demo_behavior_test.go` - update image sampler subject expectation to Step 4.
- Modify: `internal/mcpserver/demo_mode_test.go` - update MCP demo expectations for top onboarding mail and Step 4 search.
- Modify: `TUI_TESTPLAN.md` - update TC-46 so manual QA treats onboarding messages as the public demo context.
- Create: `reports/TEST_REPORT_2026-05-07_demo-onboarding-emails.md` during implementation verification. The `reports/` directory is gitignored, so this file is saved for local evidence and is not committed.

## Task 1: Add Failing Fixture Tests

**Files:**
- Modify: `internal/demo/fixtures_test.go`

- [x] **Step 1: Add `sort` to the imports**

Change the import block at the top of `internal/demo/fixtures_test.go` to:

```go
import (
	"sort"
	"strings"
	"testing"
)
```

- [x] **Step 2: Add onboarding order and body tests**

Append these tests after `TestMailboxOmitsPrivateDemoIdentityTerms`:

```go
func TestMailboxIncludesOrderedHeraldOnboardingSteps(t *testing.T) {
	messages := append([]Message(nil), Mailbox().Messages...)
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].Email.Date.After(messages[j].Email.Date)
	})

	want := []struct {
		subject string
		sender  string
	}{
		{"Step 1: Try this - move around your inbox", "Herald Guide <guide@herald.demo>"},
		{"Step 2: Reply, write, and preview Markdown", "Herald Compose Coach <compose@herald.demo>"},
		{"Step 3: Open and save attachments", "Herald Attachments <attachments@herald.demo>"},
		{"Step 4: View inline images in full screen", "Herald Image Lab <images@herald.demo>"},
		{"Step 5: Clean up senders and domains safely", "Herald Cleanup Coach <cleanup@herald.demo>"},
		{"Step 6: Classify mail and dry-run rules", "Herald AI Rules <rules@herald.demo>"},
		{"Step 7: Configure accounts, AI, and signatures", "Herald Settings <settings@herald.demo>"},
		{"Step 8: Explore contacts, chat, SSH, and MCP", "Herald Next Steps <next@herald.demo>"},
	}

	if len(messages) < len(want) {
		t.Fatalf("expected at least %d demo messages, got %d", len(want), len(messages))
	}
	for i, w := range want {
		got := messages[i].Email
		if got.Subject != w.subject {
			t.Fatalf("message %d subject = %q, want %q", i+1, got.Subject, w.subject)
		}
		if got.Sender != w.sender {
			t.Fatalf("message %d sender = %q, want %q", i+1, got.Sender, w.sender)
		}
		if got.Folder != "INBOX" {
			t.Fatalf("message %d folder = %q, want INBOX", i+1, got.Folder)
		}
	}
	for i := 1; i < len(want); i++ {
		if !messages[i-1].Email.Date.After(messages[i].Email.Date) {
			t.Fatalf("onboarding dates are not strictly descending at messages %d and %d: %s then %s", i, i+1, messages[i-1].Email.Date, messages[i].Email.Date)
		}
	}
}

func TestMailboxOnboardingBodiesTeachCoreFeatures(t *testing.T) {
	cases := []struct {
		subject        string
		wants          []string
		attachments    int
		inlineImages   int
		htmlCIDSnippet string
	}{
		{
			subject: "Step 1: Try this - move around your inbox",
			wants:   []string{"try now", "j/k", "enter", "esc", "1/2/3", "?", "what herald is doing"},
		},
		{
			subject: "Step 2: Reply, write, and preview Markdown",
			wants:   []string{"try now", "r", "ctrl+p", "ctrl+s", "preserve original formatting", "rendered html", "plain-text"},
		},
		{
			subject:     "Step 3: Open and save attachments",
			wants:       []string{"try now", "[", "]", "s", "save to", "selected attachment"},
			attachments: 2,
		},
		{
			subject:        "Step 4: View inline images in full screen",
			wants:          []string{"creative commons", "z", "kitty", "iterm2", "remote images", "not fetched", "46x21", "330px", "960px", "![remote commons thumbnail]("},
			inlineImages:   4,
			htmlCIDSnippet: "cid:cc-by-sa-badge",
		},
		{
			subject: "Step 5: Clean up senders and domains safely",
			wants:   []string{"try now", "3", "space", "sender", "domain", "unsubscribe", "preview"},
		},
		{
			subject: "Step 6: Classify mail and dry-run rules",
			wants:   []string{"try now", "a", "? infrastructure budget risk", "cleanup rules", "automation rules", "prompts", "dry-run"},
		},
		{
			subject: "Step 7: Configure accounts, AI, and signatures",
			wants:   []string{"try now", "s", "settings", "provider", "embedding model", "signature"},
		},
		{
			subject: "Step 8: Explore contacts, chat, SSH, and MCP",
			wants:   []string{"try now", "contacts", "chat panel", "quick replies", "herald mcp --demo", "herald ssh"},
		},
	}

	for _, tc := range cases {
		msg := demoMessageBySubject(t, tc.subject)
		body := strings.ToLower(msg.Body.TextPlain)
		for _, want := range tc.wants {
			if !strings.Contains(body, strings.ToLower(want)) {
				t.Fatalf("%s body missing %q:\n%s", tc.subject, want, msg.Body.TextPlain)
			}
		}
		if tc.attachments > 0 && len(msg.Body.Attachments) != tc.attachments {
			t.Fatalf("%s attachment count = %d, want %d", tc.subject, len(msg.Body.Attachments), tc.attachments)
		}
		if tc.inlineImages > 0 && len(msg.Body.InlineImages) != tc.inlineImages {
			t.Fatalf("%s inline image count = %d, want %d", tc.subject, len(msg.Body.InlineImages), tc.inlineImages)
		}
		if tc.htmlCIDSnippet != "" && !strings.Contains(msg.Body.TextHTML, tc.htmlCIDSnippet) {
			t.Fatalf("%s HTML missing %q:\n%s", tc.subject, tc.htmlCIDSnippet, msg.Body.TextHTML)
		}
	}
}

func demoMessageBySubject(t *testing.T, subject string) Message {
	t.Helper()
	for _, msg := range Mailbox().Messages {
		if msg.Email.Subject == subject {
			return msg
		}
	}
	t.Fatalf("expected demo mailbox to include %q", subject)
	return Message{}
}
```

- [x] **Step 3: Run the focused failing tests**

Run:

```bash
go test ./internal/demo -run 'TestMailboxIncludesOrderedHeraldOnboardingSteps|TestMailboxOnboardingBodiesTeachCoreFeatures' -count=1
```

Expected: FAIL because the Step 1 through Step 8 onboarding messages do not exist yet.

- [x] **Step 4: Commit the failing tests**

Run:

```bash
git add internal/demo/fixtures_test.go
git commit -m "test: specify demo onboarding email sequence"
```

## Task 2: Add Onboarding Fixture Messages

**Files:**
- Modify: `internal/demo/fixtures.go`

- [x] **Step 1: Add `strconv` to the fixture imports**

Change the import block in `internal/demo/fixtures.go` to include `strconv`:

```go
import (
	"encoding/base64"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/herald-email/herald-mail-app/internal/ai"
	"github.com/herald-email/herald-mail-app/internal/models"
)
```

- [x] **Step 2: Make demo attachment part paths unique**

Replace the `withAttachment` helper in `buildMailbox` with:

```go
	withAttachment := func(filename, mime string, size int) func(*Message) {
		return func(msg *Message) {
			msg.Email.HasAttachments = true
			msg.Body.Attachments = append(msg.Body.Attachments, models.Attachment{
				Filename: filename,
				MIMEType: mime,
				Size:     size,
				PartPath: strconv.Itoa(2 + len(msg.Body.Attachments)),
				Data:     []byte("demo attachment: " + filename),
			})
		}
	}
```

- [x] **Step 3: Add a date override helper**

Add this helper after `withInlineImage` and before `withDraft`:

```go
	withDate := func(date time.Time) func(*Message) {
		return func(msg *Message) {
			msg.Email.Date = date
		}
	}
```

- [x] **Step 4: Add the eight onboarding messages before the existing `add(1, "Northstar Cloud...`) call**

Insert this block before the current first `add(1, ...)` call:

```go
	add(31, "Herald Guide <guide@herald.demo>", "Step 1: Try this - move around your inbox", "INBOX", 0, 11264, false, true, ai.CategoryImportant, []string{"onboarding", "navigation", "timeline", "search"},
		`Step 1 is a quick lap around Herald's Timeline.

Try now
- Move through the list with j/k or the arrow keys.
- Press Enter or the right arrow to preview the selected email.
- Press Esc to close a preview.
- Press 1/2/3 to jump between Timeline, Compose, and Cleanup.
- Press f to open the folder sidebar.
- Press / to search.
- Press ? when you want the current shortcut map.

What Herald is doing
Herald keeps the Timeline keyboard-first, but the same rows can also be clicked in terminals that support mouse events. Demo mode is offline, so every message you open here is synthetic and safe to explore.`,
		withDate(baseTime.Add(8*time.Hour)))
	add(32, "Herald Compose Coach <compose@herald.demo>", "Step 2: Reply, write, and preview Markdown", "INBOX", 0, 14336, false, true, ai.CategoryImportant, []string{"onboarding", "compose", "reply", "markdown", "html"},
		`Step 2 shows how Herald turns a terminal compose screen into a real email workflow.

Try now
- Highlight this message and press R to start a reply.
- Write a few Markdown lines in the body.
- Press ctrl+p to preview the rendered message.
- Press ctrl+s to send. In demo mode, sending is simulated and does not contact SMTP.

What Herald is doing
Replies and forwards preserve original formatting, inline images, and attachments where possible. New Markdown you write is rendered HTML for email clients that support rich mail, and Herald also keeps a plain-text alternative so the message stays readable everywhere.`,
		withDate(baseTime.Add(7*time.Hour)),
		withHTMLBody(`<html><body>
<h1>Step 2: Reply, write, and preview Markdown</h1>
<p>Use this message to practice replies, Markdown preview, and safe demo sending.</p>
<ul>
<li>Replies preserve original formatting where possible.</li>
<li>Markdown sends as rendered HTML with a plain-text alternative.</li>
</ul>
</body></html>`))
	add(33, "Herald Attachments <attachments@herald.demo>", "Step 3: Open and save attachments", "INBOX", 0, 28672, false, false, ai.CategoryTransactional, []string{"onboarding", "attachments", "download", "files"},
		`Step 3 gives you a safe attachment message to practice with.

Try now
- Open this preview and look for the attachment list below the message body.
- Use [ and ] to move between attachments.
- Press s to save the selected attachment.
- In the Save to prompt, choose a path such as /tmp/herald-demo-attachment.txt.

What Herald is doing
The subject row shows an attachment marker when a message has files. Save actions use the selected attachment, not just the first one, so multi-file messages can be handled deliberately.`,
		withDate(baseTime.Add(6*time.Hour)),
		withAttachment("herald-demo-checklist.txt", "text/plain", 2048),
		withAttachment("herald-demo-routing.csv", "text/csv", 4096))
	add(34, "Herald Image Lab <images@herald.demo>", "Step 4: View inline images in full screen", "INBOX", 0, 270336, true, false, ai.CategoryNewsletter, []string{"onboarding", "images", "creative commons", "rendering", "terminal"},
		`Step 4 is the image rendering tour.

Try now
- Open this message and press z for full-screen reading.
- In Kitty or Ghostty, try ./bin/herald --demo -image-protocol=kitty.
- In iTerm2, try ./bin/herald --demo -image-protocol=iterm2.
- Scroll through the image section and watch for safe fallback links or text when raster graphics are unavailable.

What Herald is doing
This email includes embedded inline images, so Herald can render the local MIME bytes without downloading anything. Remote images are shown as links and are intentionally not fetched.

Embedded Creative Commons images:
- CC BY-SA badge: 46x21 PNG, CC0 1.0, by Heflox. Source: https://commons.wikimedia.org/wiki/File:CC-BY-SA.png
- Color chart: 330px PNG thumbnail, CC0 1.0, by Ccompagnon with a simplified revision by Iketsi. Source: https://commons.wikimedia.org/wiki/File:ColorChart.svg
- Bee on sunflower: 330px JPEG thumbnail, CC BY 4.0, by Mbrickn. Source: https://commons.wikimedia.org/wiki/File:Bee_on_Sunflower.jpg
- Changing Landscape: 960px JPEG thumbnail, CC BY 4.0, by Mit.d.sheth. Source: https://commons.wikimedia.org/wiki/File:Changing_Landscape.jpg

Remote image link, intentionally not fetched by Herald:
![Remote Commons thumbnail](https://upload.wikimedia.org/wikipedia/commons/thumb/c/c0/ColorChart.svg/330px-ColorChart.svg.png)`,
		withDate(baseTime.Add(5*time.Hour)),
		withHTMLBody(`<html><body>
<h1>Step 4: View inline images in full screen</h1>
<p>Open this message and press <strong>z</strong> for full-screen reading.</p>
<p><img alt="CC BY-SA badge" src="cid:cc-by-sa-badge"></p>
<p><img alt="Color chart" src="cid:color-chart-330px"></p>
<p><img alt="Bee on sunflower" src="cid:bee-on-sunflower-330px"></p>
<p><img alt="Changing landscape" src="cid:changing-landscape-960px"></p>
<h2>Embedded Creative Commons images</h2>
<ul>
<li>CC BY-SA badge: 46x21 PNG, CC0 1.0, by Heflox. Source: <a href="https://commons.wikimedia.org/wiki/File:CC-BY-SA.png">CC-BY-SA.png</a></li>
<li>Color chart: 330px PNG thumbnail, CC0 1.0, by Ccompagnon with a simplified revision by Iketsi. Source: <a href="https://commons.wikimedia.org/wiki/File:ColorChart.svg">ColorChart.svg</a></li>
<li>Bee on sunflower: 330px JPEG thumbnail, CC BY 4.0, by Mbrickn. Source: <a href="https://commons.wikimedia.org/wiki/File:Bee_on_Sunflower.jpg">Bee on Sunflower</a></li>
<li>Changing Landscape: 960px JPEG thumbnail, CC BY 4.0, by Mit.d.sheth. Source: <a href="https://commons.wikimedia.org/wiki/File:Changing_Landscape.jpg">Changing Landscape</a></li>
</ul>
<p>Remote image link, intentionally not fetched by Herald:</p>
<p><img alt="Remote Commons thumbnail" src="https://upload.wikimedia.org/wikipedia/commons/thumb/c/c0/ColorChart.svg/330px-ColorChart.svg.png"></p>
</body></html>`),
		withInlineImage("cc-by-sa-badge", "image/png", demoCCBySABadgePNG),
		withInlineImage("color-chart-330px", "image/png", demoColorChartPNG),
		withInlineImage("bee-on-sunflower-330px", "image/jpeg", demoBeeOnSunflowerJPG),
		withInlineImage("changing-landscape-960px", "image/jpeg", demoChangingLandscapeJPG))
	add(35, "Herald Cleanup Coach <cleanup@herald.demo>", "Step 5: Clean up senders and domains safely", "INBOX", 0, 12288, false, false, ai.CategoryNewsletter, []string{"onboarding", "cleanup", "sender", "domain", "unsubscribe"},
		`Step 5 points you at Herald's bulk cleanup workflow.

Try now
- Press 3 to open Cleanup.
- Use j/k to move between senders.
- Press d to switch between sender and domain grouping.
- Press space to select a sender or domain.
- Preview before taking action, then use delete, archive, or unsubscribe when the hints show those actions.

What Herald is doing
Cleanup groups messages by sender or domain so repeated mail can be handled in batches. Herald keeps destructive actions deliberate: preview first, select what you mean, and use unsubscribe only when the message exposes a safe unsubscribe header.`,
		withDate(baseTime.Add(4*time.Hour)),
		withHTML,
		withUnsub("https://herald.demo/unsubscribe/cleanup-coach"))
	add(36, "Herald AI Rules <rules@herald.demo>", "Step 6: Classify mail and dry-run rules", "INBOX", 0, 13568, false, true, ai.CategoryImportant, []string{"onboarding", "ai", "rules", "dry-run", "infrastructure", "budget", "risk"},
		`Step 6 introduces the offline demo AI and rule previews.

Try now
- Press a to classify the current folder.
- Press /, type ? infrastructure budget risk, and press Enter for semantic search.
- Press C to open cleanup rules.
- Press W to open automation rules.
- Press P to open custom prompts.
- Use dry-run previews before running rules.

What Herald is doing
Demo AI is deterministic and offline, so classification, semantic search, quick replies, and rule previews work without Ollama. Dry-runs show the matching messages and planned actions before mail is changed.`,
		withDate(baseTime.Add(3*time.Hour)))
	add(37, "Herald Settings <settings@herald.demo>", "Step 7: Configure accounts, AI, and signatures", "INBOX", 0, 11008, true, false, ai.CategoryImportant, []string{"onboarding", "settings", "configuration", "signature", "embedding model"},
		`Step 7 shows where Herald configuration lives.

Try now
- Press S to open Settings.
- Review the account provider fields.
- Review the AI provider fields, including local Ollama and OpenAI-compatible options.
- Find the embedding model field used by semantic search.
- Add or review an email signature, then close Settings with Esc if you are only exploring.

What Herald is doing
The settings overlay writes the same YAML shape used by normal config files. Demo mode itself does not read your mailbox or send mail, but saving settings is still a real configuration action, so inspect safely and save only when you mean it.`,
		withDate(baseTime.Add(2*time.Hour)))
	add(38, "Herald Next Steps <next@herald.demo>", "Step 8: Explore contacts, chat, SSH, and MCP", "INBOX", 0, 9984, true, false, ai.CategoryNewsletter, []string{"onboarding", "contacts", "chat", "quick replies", "mcp", "ssh"},
		`Step 8 gives you a few extra paths to try after the core tour.

Try now
- Open Contacts and inspect a recent email from a contact.
- Press c to open the chat panel.
- Open a preview and try quick replies.
- Run herald mcp --demo to expose the same synthetic mailbox to an agent.
- Run herald ssh when you want the TUI served over SSH.

What Herald is doing
The demo mailbox is shared across the TUI and MCP demo surfaces, so search, stats, classifications, and previews all point at the same fictional data. Good practice searches include ? infrastructure budget risk, images, attachments, and cleanup.`,
		withDate(baseTime.Add(1*time.Hour)))
```

- [x] **Step 5: Remove the old Open Commons sampler fixture**

Delete the existing `add(29, "Open Commons Gallery <images@opencommons.example>", "Creative Commons image sampler for terminal previews", ...)` block from `internal/demo/fixtures.go`. The new Step 4 message now owns the same inline image coverage and Creative Commons attribution.

- [x] **Step 6: Run the focused fixture tests**

Run:

```bash
go test ./internal/demo -run 'TestMailboxIncludesOrderedHeraldOnboardingSteps|TestMailboxOnboardingBodiesTeachCoreFeatures' -count=1
```

Expected: PASS.

- [x] **Step 7: Run all demo fixture tests**

Run:

```bash
go test ./internal/demo -count=1
```

Expected: FAIL because existing tests still look for the old Creative Commons sampler subject and sender.

- [x] **Step 8: Commit the fixture implementation**

Run:

```bash
git add internal/demo/fixtures.go
git commit -m "feat: add Herald onboarding demo emails"
```

## Task 3: Update Demo Test Expectations

**Files:**
- Modify: `internal/demo/fixtures_test.go`
- Modify: `internal/backend/demo_behavior_test.go`
- Modify: `internal/mcpserver/demo_mode_test.go`

- [x] **Step 1: Update the image sampler fixture test constants**

In `internal/demo/fixtures_test.go`, change `TestMailboxIncludesCreativeCommonsImageSampler` to use:

```go
	const subject = "Step 4: View inline images in full screen"
```

Then change its sender expectation to:

```go
		if msg.Email.Sender != "Herald Image Lab <images@herald.demo>" {
			t.Fatalf("unexpected sampler sender: %q", msg.Email.Sender)
		}
```

- [x] **Step 2: Update the HTML CID placement subject**

In `internal/demo/fixtures_test.go`, change the subject checked in `TestCreativeCommonsSamplerIncludesHTMLCIDPlacement` to:

```go
		if box.Messages[i].Email.Subject == "Step 4: View inline images in full screen" {
			found = &box.Messages[i]
			break
		}
```

- [x] **Step 3: Update the backend image sampler subject**

In `internal/backend/demo_behavior_test.go`, change the subject constant in `TestDemoBackendFetchesCreativeCommonsImageSampler` to:

```go
	const subject = "Step 4: View inline images in full screen"
```

- [x] **Step 4: Update MCP list and search expectations**

In `internal/mcpserver/demo_mode_test.go`, change the recent-mail expectation in `TestDemoMCPServerListsAndReadsDemoEmails` to:

```go
	if !strings.Contains(string(callJSON), "Herald Guide") {
		t.Fatalf("expected Herald onboarding demo mailbox data in response: %s", callJSON)
	}
```

Then update `TestDemoMCPSearchFindsCreativeCommonsImageSampler` to:

```go
func TestDemoMCPSearchFindsCreativeCommonsImageSampler(t *testing.T) {
	s := newDemoMCPServer()

	callResp := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"creative commons"}}}`))
	callJSON, err := json.Marshal(callResp)
	if err != nil {
		t.Fatalf("marshal search_emails response: %v", err)
	}
	body := strings.ToLower(string(callJSON))
	if !strings.Contains(body, "step 4: view inline images in full screen") {
		t.Fatalf("expected Step 4 image sampler in search response: %s", callJSON)
	}
	if !strings.Contains(body, "message_id=") {
		t.Fatalf("expected search_emails response to expose message_id values: %s", callJSON)
	}

	callResp = s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"images"}}}`))
	callJSON, err = json.Marshal(callResp)
	if err != nil {
		t.Fatalf("marshal image search_emails response: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(callJSON)), "step 4: view inline images in full screen") {
		t.Fatalf("expected image query to find Step 4 sampler: %s", callJSON)
	}
}
```

- [x] **Step 5: Run updated demo tests**

Run:

```bash
go test ./internal/demo ./internal/backend ./internal/mcpserver -run 'Demo|Mailbox|CreativeCommons|Onboarding' -count=1
```

Expected: PASS.

- [x] **Step 6: Commit the test expectation updates**

Run:

```bash
git add internal/demo/fixtures_test.go internal/backend/demo_behavior_test.go internal/mcpserver/demo_mode_test.go
git commit -m "test: align demo expectations with onboarding mailbox"
```

## Task 4: Update Manual TUI Test Plan

**Files:**
- Modify: `TUI_TESTPLAN.md`

- [x] **Step 1: Replace TC-46 steps and expectations**

Replace the TC-46 block in `TUI_TESTPLAN.md` with:

```markdown
### TC-46 — Demo fixtures cover onboarding and public UI context

**Lane:** A
**Sizes:** `220x50`, `80x24`

**Steps:**
1. Start `/tmp/herald --demo`.
2. Confirm the top Timeline messages are `Step 1:` through `Step 8:` from Herald senders.
3. Open Step 1 and confirm the body teaches navigation keys.
4. Open Step 2 and confirm the body explains reply, Markdown preview, preserved original formatting, rendered HTML, and plain-text fallback.
5. Open Step 3 and confirm at least two attachments are available and selection hints appear.
6. Open Step 4 and confirm inline image/full-screen instructions are present.
7. Switch to Cleanup, open sender details, and preview one message.
8. Switch to Contacts, open one contact detail, and open a recent email inline.

**Expect:**
- Timeline starts with explicit Herald onboarding messages ordered Step 1 through Step 8.
- Preview bodies are specific instructional docs rather than generic lorem ipsum.
- Attachment, unsubscribe, HTML, inline image, cleanup, AI, semantic search, contacts, and MCP demo coverage remains represented in the fixture set.
- Contacts are populated from demo data and their recent emails open inline.
```

- [x] **Step 2: Run a docs diff check**

Run:

```bash
git diff -- TUI_TESTPLAN.md
```

Expected: the diff only changes TC-46.

- [x] **Step 3: Commit the manual test plan update**

Run:

```bash
git add TUI_TESTPLAN.md
git commit -m "docs: update demo onboarding TUI test plan"
```

## Task 5: Full Verification

**Files:**
- Create: `reports/TEST_REPORT_2026-05-07_demo-onboarding-emails.md`

- [x] **Step 1: Run focused Go verification**

Run:

```bash
go test ./internal/demo ./internal/backend ./internal/mcpserver -count=1
```

Expected: PASS.

- [x] **Step 2: Run all Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [x] **Step 3: Build the TUI binary**

Run:

```bash
go build -o /tmp/herald-demo-onboarding .
```

Expected: command exits 0 and writes `/tmp/herald-demo-onboarding`.

- [x] **Step 4: Verify the wide demo Timeline in tmux**

Run:

```bash
tmux new-session -d -s herald-demo-onboarding-220 -x 220 -y 50
tmux send-keys -t herald-demo-onboarding-220 '/tmp/herald-demo-onboarding --demo' Enter
sleep 2
tmux capture-pane -t herald-demo-onboarding-220 -p -e > /tmp/herald-demo-onboarding-220.txt
tmux kill-session -t herald-demo-onboarding-220
```

Expected: `/tmp/herald-demo-onboarding-220.txt` shows Step 1 through Step 8 near the top of Timeline, with Herald senders and no panic.

- [x] **Step 5: Verify the standard demo Timeline in tmux**

Run:

```bash
tmux new-session -d -s herald-demo-onboarding-80 -x 80 -y 24
tmux send-keys -t herald-demo-onboarding-80 '/tmp/herald-demo-onboarding --demo' Enter
sleep 2
tmux capture-pane -t herald-demo-onboarding-80 -p -e > /tmp/herald-demo-onboarding-80.txt
tmux kill-session -t herald-demo-onboarding-80
```

Expected: `/tmp/herald-demo-onboarding-80.txt` shows the onboarding sequence without horizontal overflow or broken chrome.

- [x] **Step 6: Verify the minimum-size guard**

Run:

```bash
tmux new-session -d -s herald-demo-onboarding-50 -x 50 -y 15
tmux send-keys -t herald-demo-onboarding-50 '/tmp/herald-demo-onboarding --demo' Enter
sleep 2
tmux capture-pane -t herald-demo-onboarding-50 -p -e > /tmp/herald-demo-onboarding-50.txt
tmux kill-session -t herald-demo-onboarding-50
```

Expected: `/tmp/herald-demo-onboarding-50.txt` shows the standard minimum-size guard or compact recovery behavior, with no clipped onboarding UI.

- [x] **Step 7: Write the verification report**

Create `reports/TEST_REPORT_2026-05-07_demo-onboarding-emails.md` with this content:

```markdown
# Demo Onboarding Emails Test Report

## Summary

- Added Herald Step 1 through Step 8 onboarding emails to demo mode.
- Verified fixture, backend, MCP, and TUI smoke coverage.

## Commands

- `go test ./internal/demo ./internal/backend ./internal/mcpserver -count=1`
- `go test ./...`
- `go build -o /tmp/herald-demo-onboarding .`

## tmux Captures

- `220x50`: `/tmp/herald-demo-onboarding-220.txt`
- `80x24`: `/tmp/herald-demo-onboarding-80.txt`
- `50x15`: `/tmp/herald-demo-onboarding-50.txt`

## Results

- `220x50`: Step 1 through Step 8 appear at the top of Timeline.
- `80x24`: Onboarding sequence renders without overflow.
- `50x15`: Minimum-size guard or compact recovery appears cleanly.

## Notes

- Demo mode remains offline and deterministic.
- Supporting fixture coverage remains present for attachments, inline images, cleanup, AI, semantic search, contacts, and MCP.
```

- [x] **Step 8: Confirm the local verification report exists**

Run:

```bash
test -s reports/TEST_REPORT_2026-05-07_demo-onboarding-emails.md
```

Expected: command exits 0. Do not commit this file because `reports/` is intentionally gitignored.

## Task 6: Final Integration Check

**Files:**
- Review: all files changed by Tasks 1 through 5

- [x] **Step 1: Check final status**

Run:

```bash
git status --short
```

Expected: only pre-existing unrelated local changes remain, or a clean tree if the implementer started from a clean worktree.

- [x] **Step 2: Review the final diff**

Run:

```bash
git log --oneline -5
git diff HEAD~4..HEAD -- internal/demo/fixtures.go internal/demo/fixtures_test.go internal/backend/demo_behavior_test.go internal/mcpserver/demo_mode_test.go TUI_TESTPLAN.md
```

Expected: the diff adds onboarding fixtures, updates demo expectations, and updates TC-46. It does not alter normal IMAP behavior, SMTP behavior, or TUI key routing.

- [x] **Step 3: Confirm the design requirements are covered**

Check the implementation against `docs/superpowers/specs/2026-05-07-demo-onboarding-emails-design.md`:

```bash
sed -n '1,220p' docs/superpowers/specs/2026-05-07-demo-onboarding-emails-design.md
```

Expected: every checked design area maps to changed fixtures, tests, or TUI test plan coverage.
