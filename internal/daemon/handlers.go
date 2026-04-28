package daemon

import "net/http"

// registerRoutes wires all v1 REST+SSE routes to the provided mux.
// Method-prefixed patterns (Go 1.22+) ensure correct HTTP verb enforcement.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health / control
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("POST /v1/sync", s.handleSync)
	mux.HandleFunc("GET /v1/events", s.handleEvents)

	// Folders — literal "POST /v1/folders" must be registered before folder-name routes.
	mux.HandleFunc("GET /v1/folders", s.handleListFolders)
	mux.HandleFunc("POST /v1/folders", s.handleCreateFolder)
	mux.HandleFunc("POST /v1/folders/{name}/rename", s.handleRenameFolder)
	mux.HandleFunc("DELETE /v1/folders/{name...}", s.handleDeleteFolder)

	// Sync — /v1/sync/all and /v1/sync/status must be registered before POST /v1/sync wildcard
	mux.HandleFunc("POST /v1/sync/all", s.handleSyncAllFolders)
	mux.HandleFunc("GET /v1/sync/status", s.handleGetSyncStatus)

	// Emails
	mux.HandleFunc("POST /v1/emails/send", s.handleSendEmail) // must be before /{id}/... routes
	mux.HandleFunc("GET /v1/emails", s.handleGetEmails)
	mux.HandleFunc("GET /v1/emails/{id}", s.handleGetEmail)
	mux.HandleFunc("GET /v1/emails/{id}/body", s.handleGetEmailBody)
	mux.HandleFunc("DELETE /v1/emails/{id}", s.handleDeleteEmail)
	mux.HandleFunc("POST /v1/emails/{id}/archive", s.handleArchiveEmail)
	mux.HandleFunc("POST /v1/emails/{id}/move", s.handleMoveEmail)
	mux.HandleFunc("POST /v1/emails/{id}/classify", s.handleClassifyEmail)
	mux.HandleFunc("POST /v1/emails/{id}/read", s.handleMarkRead)
	mux.HandleFunc("POST /v1/emails/{id}/unread", s.handleMarkUnread)
	mux.HandleFunc("POST /v1/emails/{id}/star", s.handleMarkStarred)
	mux.HandleFunc("DELETE /v1/emails/{id}/star", s.handleUnmarkStarred)
	mux.HandleFunc("POST /v1/emails/{id}/reply", s.handleReplyEmail)
	mux.HandleFunc("POST /v1/emails/{id}/forward", s.handleForwardEmail)
	mux.HandleFunc("GET /v1/emails/{id}/attachments", s.handleListAttachments)
	mux.HandleFunc("GET /v1/emails/{id}/attachments/{filename...}", s.handleGetAttachment)

	// Threads
	mux.HandleFunc("GET /v1/threads", s.handleGetThread)
	mux.HandleFunc("POST /v1/threads/delete", s.handleDeleteThread)
	mux.HandleFunc("POST /v1/threads/archive", s.handleArchiveThread)

	// Bulk email operations
	mux.HandleFunc("POST /v1/emails/bulk-delete", s.handleBulkDelete)
	mux.HandleFunc("POST /v1/emails/bulk-move", s.handleBulkMove)
	mux.HandleFunc("POST /v1/emails/{id}/unsubscribe", s.handleUnsubscribeSender)

	// Senders
	mux.HandleFunc("DELETE /v1/senders/{sender}", s.handleDeleteSender)
	mux.HandleFunc("POST /v1/senders/{sender}/archive", s.handleArchiveSender)
	mux.HandleFunc("POST /v1/senders/{sender}/soft-unsubscribe", s.handleSoftUnsubscribeSender)

	// Statistics
	mux.HandleFunc("GET /v1/stats", s.handleGetStats)

	// Search
	mux.HandleFunc("GET /v1/search", s.handleSearch)
	mux.HandleFunc("GET /v1/search/all", s.handleSearchAll)
	mux.HandleFunc("GET /v1/search/semantic", s.handleSearchSemantic)

	// Classifications
	mux.HandleFunc("GET /v1/classifications", s.handleGetClassifications)
	mux.HandleFunc("POST /v1/classify/folder", s.handleClassifyFolder)

	// Rules
	mux.HandleFunc("GET /v1/rules", s.handleGetRules)
	mux.HandleFunc("POST /v1/rules", s.handleSaveRule)
	mux.HandleFunc("DELETE /v1/rules/{id}", s.handleDeleteRule)

	// Custom prompts
	mux.HandleFunc("GET /v1/prompts", s.handleGetPrompts)
	mux.HandleFunc("POST /v1/prompts", s.handleSavePrompt)

	// Cleanup rules — literal "run" segment must be registered before the {id} wildcard
	mux.HandleFunc("POST /v1/cleanup-rules/run", s.handleRunCleanupRules)
	mux.HandleFunc("GET /v1/cleanup-rules", s.handleListCleanupRules)
	mux.HandleFunc("POST /v1/cleanup-rules", s.handleCreateCleanupRule)
	mux.HandleFunc("DELETE /v1/cleanup-rules/{id}", s.handleDeleteCleanupRule)

	// Drafts — literal "send" segment must be registered before the {uid} wildcard
	mux.HandleFunc("POST /v1/drafts/{uid}/send", s.handleSendDraft)
	mux.HandleFunc("GET /v1/drafts", s.handleListDrafts)
	mux.HandleFunc("POST /v1/drafts", s.handleSaveDraft)
	mux.HandleFunc("DELETE /v1/drafts/{uid}", s.handleDeleteDraft)
}
