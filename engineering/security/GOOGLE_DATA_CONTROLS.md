# Google credentials and private-data sharing

## OAuth and token storage

Herald uses a fresh S256 PKCE verifier for each desktop authorization attempt. Tokens stay available in memory for Google API calls, but configuration saves contain secure-store references instead of access or refresh tokens.

- [x] macOS uses Keychain, Windows uses Credential Manager, and Linux uses Secret Service through `go-keyring`.
- [x] Loading a legacy configuration migrates inline OAuth tokens only after validation and successful secure storage. A locked or unavailable store stops migration with an error; there is no plaintext fallback.
- [x] Multi-account Google Mail and Calendar tokens are stored separately. Configuration files retain 0600 permissions and atomic replacement.

For SSH/headless hosts without an available credential service, supply `HERALD_TOKEN_STORE_KEY` from a secret manager before starting Herald, the daemon, SSH server, or MCP server. Its value must be a base64-encoded, cryptographically random 32-byte key. Generate it once (for example with `openssl rand -base64 32`), keep it stable, and inject the same value into every process using that configuration. Do not put it in YAML, source control, or alongside the encrypted files.

In this mode, AES-256-GCM encrypted token files live in `<config-path>.tokens/`; back up that directory with the configuration and retain the key separately. Moving an OS-keyring configuration to another machine requires reauthorization or a deliberate migration while the original credential store remains accessible. A missing key or tampered ciphertext fails closed.

Migration cannot erase old backups or filesystem snapshots of plaintext configurations. Disconnecting an account removes its configuration reference; unused secure-store records are not automatically deleted. This change protects OAuth tokens, not the SQLite mail cache or third-party AI API keys.

## Affirmative sharing consent

Settings → AI shows the chat, embedding, and agent destinations before asking for separate permissions. New and legacy configurations start without sharing grants; choosing an AI provider alone does not grant permission.

- [x] AI permission covers private mail, contacts, calendar, attachments/images, prompts, and retained cached history sent by enabled AI features, including local providers.
- [x] Background permission additionally enables indexing, classification, and contact enrichment. Interactive permission alone cannot authorize background embeddings, including mixed-provider configurations.
- [x] MCP permission independently authorizes attached clients to use enabled tools, including supported mutations and access to retained cached history. Clients may forward results to their AI providers; stdio does not authenticate a particular client application.
- [x] Changing account identities or recipients invalidates grants. Editing AI settings starts the choices at No; review and explicitly allow the desired permissions before saving.
- [x] Saving No revokes future calls. Running AI and MCP clients check the saved configuration before subsequent requests; agent follow-up requests are checked too. Already dispatched requests and previously shared data cannot be recalled.

Use Herald's Settings UI, locally or over SSH, to grant consent before starting a headless MCP client. Do not copy grant hashes between configurations. Demo mode uses synthetic data and remains available without grants.

## Review scope

These controls supply implementation evidence for Google's verification review. They do not establish provider retention/training terms or constitute an independent security assessment.

- [x] Regression coverage includes PKCE challenge/exchange binding, migration failure, multi-account storage, ciphertext tampering, consent revocation, changed recipients, agent requests, background embeddings, and MCP reads.
- [ ] Confirm production AI providers' retention/training settings and approved transport endpoints before recording the verification video.
- [ ] Record the actual deployed build's consent and revocation flows; a synthetic demo is not evidence of Google scope use.
