# Herald Google OAuth verification recording plan

## Required content and installation

Google asks for the complete OAuth flow, the full English consent screen with the submitted scopes, and the app features that use those scopes. Installation is not listed as a required segment; start with Herald installed unless the reviewer specifically requests installation evidence. Source: [Google Cloud: Demo Video](https://support.google.com/cloud/answer/13804565?hl=en), checked November 7, 2026.

- [ ] Use the same Herald branding and production Desktop OAuth client submitted under project `herald-mail-494415`.
- [ ] Briefly show `herald -version` and launch the release candidate. Do not show build secrets or a source-build tutorial.
- [ ] Use a dedicated Google test account with invented email and event content. Disable desktop notifications and close unrelated windows before recording.
- [ ] Keep Google consent in English and expand the permission details. Existing accounts may collapse earlier grants under “5 services”; a dedicated test account makes the first-time flow clearer. Show every distinct Google authorization workflow offered by the app, including account-add or Calendar reconnect paths if presented separately.
- [ ] Keep the full authorization grant visible. Do not record passwords, MFA codes, access/refresh tokens, or the callback URL containing an authorization code.

## Suggested sequence

Aim for a readable five-to-seven-minute recording; that duration is a planning suggestion, not a Google requirement. Use brief narration or captions to connect each action to its purpose.

| Approximate time | Show | Suggested narration |
| --- | --- | --- |
| 0:00–0:20 | Herald version, launch, public homepage/privacy-policy links | “Herald is a desktop mail and calendar client. This is the application submitted for verification.” |
| 0:20–1:30 | Fresh Google setup, include Calendar, account selection, all consent permissions in English, grant and return to Herald | “The user connects their Google account and grants only the permissions used by these features.” |
| 1:30–2:30 | Open an invented email; mark it unread/read; archive it and find/restore it | “Herald reads mail and changes message state. Read-only Gmail permission would not support these actions.” |
| 2:30–3:45 | List/select Google calendars; open a test event; create/edit/delete an event with no attendees | “Herald lists calendars and lets the user manage events on calendars they can access.” |
| 3:45–5:00 | Settings → AI: actual destinations and default No; separate background and MCP choices; one synthetic-data feature after deliberate opt-in | “Google authorization does not automatically authorize AI or MCP sharing. The user reviews recipients and chooses each permission.” |
| 5:00–5:45 | Save No and show the corresponding feature refuses a new request; restart Herald and demonstrate the connected account works again | “Consent can be revoked for future requests. Herald reloads OAuth credentials from the operating system’s secure store.” |
| Optional appendix | Sanitized validation evidence and Keychain item metadata, without showing its password | “OAuth tokens are stored in Keychain; configuration contains references.” |

The Keychain/PKCE appendix supports the security explanation but is not a substitute for the Google scope demonstration. Do not claim that a video alone completes a restricted-scope security assessment.

## Scope coverage

Each requested scope needs a visible, concrete use in the real app. The rows below reflect the scope set prepared for the current submission.

| Scope | Evidence to show |
| --- | --- |
| `openid` and `userinfo.email` | Google login establishes account identity; Herald displays the connected email address and associates the correct sources. |
| `gmail.modify` | Read test mail and change message state through archive/restore or read/unread. Avoid permanently deleting mail. |
| `calendar.calendarlist.readonly` | Fetch and display the user's Google calendar list, then select a calendar. |
| `calendar.events` | Display test events and create/edit/delete a test event on a calendar the test account can edit. |

- [ ] Check the actual consent screen against the submitted scopes before recording; do not add scopes merely for the video.
- [ ] If demonstrating sending mail, use an explicitly authorized recipient. Sending is not required for the archive/read-state demonstration above.
- [ ] Use no attendees for event mutations so the recording sends no invitations.
- [ ] Demonstrate additional OAuth entry points if the final UI exposes them; do not assume the initial combined Mail + Calendar flow covers a separate reconnect workflow.

## AI and MCP segment

The recording should show the actual data-sharing behavior and choices. Use synthetic content, and state only retention/training claims verified for the configured provider.

- [ ] Show the chat, embedding, and agent destinations without API keys.
- [ ] Show that all three permissions initially default to No and that selecting a provider does not itself grant sharing permission.
- [ ] Explicitly opt in to AI with synthetic mailbox content; demonstrate the relevant feature.
- [ ] Separately opt in to automatic indexing/classification/enrichment, or show it remains disabled without that permission.
- [ ] Separately demonstrate MCP consent and a harmless read through the intended client, explaining that the client can forward tool results to its AI provider.
- [ ] Save No and show a subsequent request is blocked. Explain that already-dispatched requests and previously shared data cannot be recalled.

## Export and reviewer handoff

The review team must be able to open the link. Prepare an unlisted YouTube video or another reviewer-accessible link; do not expose a personal mailbox to make the link accessible.

- [ ] Review the final export for secrets, private mail, unrelated tabs, notification banners, and unreadable text.
- [ ] Verify the link opens without a separate access request and matches the deployed build shown.
- [ ] Keep the existing submitted link unchanged until the replacement video is ready.
- [ ] Supply the new link through the review team's requested channel when requested; preparing this plan does not upload a video or send an email.
