# Virtual Mail Lab

`internal/testmail` provides deterministic local IMAP/SMTP servers for tests. It is the realistic regression lane between polished demo mode and private live mailboxes.

## Use

```go
lab := testmail.Start(t)
alice := lab.Account(testmail.DefaultAliceAddress)
bob := lab.Account(testmail.DefaultBobAddress)
cfg := alice.Config(filepath.Join(t.TempDir(), "alice-cache.db"))
```

The default lab starts:

- `alice@herald.test`
- `bob@herald.test`

Each account has `INBOX`, `Sent`, `Drafts`, `Archive`, and `Trash`. SMTP delivery records every accepted message, appends the sender copy to `Sent`, and delivers known recipients to `INBOX`.

## Corpus

Sanitized realistic fixtures live under `internal/testmail/testdata/corpus/<scenario>/`.

Current scenarios cover:

- plain two-user thread
- Calendly-like calendar invite
- table-heavy newsletter
- HTML receipt
- malformed charset fallback
- inline CID image
- long safe link

## Sanitizing Real Repros

Raw exports belong in the gitignored quarantine area:

```sh
reports/quarantine/mail/
```

Sanitize and validate before committing:

```sh
go run ./tools/testmail-sanitize \
  -in reports/quarantine/mail/raw.eml \
  -out internal/testmail/testdata/corpus/<scenario>/message.eml \
  -validate internal/testmail/testdata/corpus
```

AI can help describe structure or suggest replacements, but deterministic validation is the commit gate.
