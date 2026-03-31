package daemon

import "net/http"

// registerRoutes wires all v1 REST+SSE routes to the provided mux.
// Method-prefixed patterns (Go 1.22+) ensure correct HTTP verb enforcement.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health / control
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("POST /v1/sync", s.handleSync)
	mux.HandleFunc("GET /v1/events", s.handleEvents)

	// Folders
	mux.HandleFunc("GET /v1/folders", s.handleListFolders)

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

	// Threads
	mux.HandleFunc("GET /v1/threads", s.handleGetThread)

	// Senders
	mux.HandleFunc("DELETE /v1/senders/{sender}", s.handleDeleteSender)

	// Statistics
	mux.HandleFunc("GET /v1/stats", s.handleGetStats)

	// Search
	mux.HandleFunc("GET /v1/search", s.handleSearch)
	mux.HandleFunc("GET /v1/search/all", s.handleSearchAll)
	mux.HandleFunc("GET /v1/search/semantic", s.handleSearchSemantic)

	// Classifications
	mux.HandleFunc("GET /v1/classifications", s.handleGetClassifications)

	// Rules
	mux.HandleFunc("GET /v1/rules", s.handleGetRules)
	mux.HandleFunc("POST /v1/rules", s.handleSaveRule)
	mux.HandleFunc("DELETE /v1/rules/{id}", s.handleDeleteRule)

	// Custom prompts
	mux.HandleFunc("GET /v1/prompts", s.handleGetPrompts)
	mux.HandleFunc("POST /v1/prompts", s.handleSavePrompt)
}
