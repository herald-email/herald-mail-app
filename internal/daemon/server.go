package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"mail-processor/internal/ai"
	"mail-processor/internal/backend"
	"mail-processor/internal/cache"
	"mail-processor/internal/cleanup"
	"mail-processor/internal/config"
	"mail-processor/internal/logger"
	"mail-processor/internal/models"
)

// Server is the HTTP daemon that exposes the email backend over a REST+SSE API.
type Server struct {
	cfg         *config.Config
	configPath  string
	backend     backend.Backend
	cache       *cache.Cache
	classifier  ai.AIClient
	broadcaster *Broadcaster
	httpSrv     *http.Server
	startTime   time.Time
	log         *logger.Logger
}

// New creates a Server from the given config. It initialises the LocalBackend
// (IMAP + SQLite) and wires up the SSE broadcaster.
func New(cfg *config.Config, configPath string) (*Server, error) {
	classifier, err := ai.NewFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("daemon: init ai: %w", err)
	}

	var b backend.Backend
	lb, err := backend.NewLocal(cfg, configPath, classifier)
	if err != nil {
		return nil, fmt.Errorf("daemon: init backend: %w", err)
	}
	b = lb

	s := &Server{
		cfg:         cfg,
		configPath:  configPath,
		backend:     b,
		classifier:  classifier,
		broadcaster: NewBroadcaster(),
		startTime:   time.Now(),
		log:         logger.New(),
	}
	// Expose the underlying cache for components like the cleanup engine.
	s.cache = lb.Cache()
	return s, nil
}

// Start registers all routes, begins background event pumps, and serves HTTP.
// It blocks until the server is shut down.
func (s *Server) Start() error {
	addr := net.JoinHostPort(s.cfg.Daemon.BindAddr, strconv.Itoa(s.cfg.Daemon.Port))

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE connections are long-lived
		IdleTimeout:  120 * time.Second,
	}

	go s.startPumps()

	logger.Info("daemon: listening on %s", addr)
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server and closes the backend.
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error
	if s.httpSrv != nil {
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if s.backend != nil {
		if err := s.backend.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// startPumps fans backend channels into SSE events.
func (s *Server) startPumps() {
	go func() {
		for p := range s.backend.Progress() {
			s.broadcastJSON("progress", p)
		}
	}()
	go func() {
		for n := range s.backend.NewEmailsCh() {
			s.broadcastJSON("new_emails", n)
		}
	}()
}

// broadcastJSON marshals v and sends it as an SSE event.
func (s *Server) broadcastJSON(event string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		logger.Error("daemon: broadcast marshal: %v", err)
		return
	}
	s.broadcaster.Send(event, string(data))
}

// writeJSON writes v as JSON with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error object.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// statusResponse is the body of GET /v1/status.
type statusResponse struct {
	PID     int    `json:"pid"`
	Uptime  string `json:"uptime"`
	Version string `json:"version"`
}

// handleStatus returns basic daemon health information.
func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		PID:     os.Getpid(),
		Uptime:  time.Since(s.startTime).Truncate(time.Second).String(),
		Version: "phase2",
	})
}

// handleEvents streams SSE to the caller.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	s.broadcaster.ServeHTTP(w, r)
}

// syncRequest is the optional body for POST /v1/sync.
type syncRequest struct {
	Folder string `json:"folder"`
}

// handleSync triggers background email synchronisation.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var req syncRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Folder == "" {
		req.Folder = r.URL.Query().Get("folder")
	}
	if req.Folder == "" {
		req.Folder = "INBOX"
	}
	s.backend.Load(req.Folder)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "syncing", "folder": req.Folder})
}

// handleListFolders returns all IMAP folders.
func (s *Server) handleListFolders(w http.ResponseWriter, _ *http.Request) {
	folders, err := s.backend.ListFolders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, folders)
}

// handleGetEmails returns timeline emails for a folder.
func (s *Server) handleGetEmails(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	emails, err := s.backend.GetTimelineEmails(folder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

// handleGetEmail returns a single email by message ID.
func (s *Server) handleGetEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	email, err := s.backend.GetEmailByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, email)
}

// handleGetEmailBody fetches the full body of an email by message ID.
// It requires the email to be in the cache to look up its UID and folder.
func (s *Server) handleGetEmailBody(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	email, err := s.backend.GetEmailByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if email == nil {
		writeError(w, http.StatusNotFound, "email not found")
		return
	}
	body, err := s.backend.FetchEmailBody(email.Folder, email.UID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// handleDeleteEmail deletes a single email by message ID.
func (s *Server) handleDeleteEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	if err := s.backend.DeleteEmail(id, folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleArchiveEmail archives a single email.
func (s *Server) handleArchiveEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	if err := s.backend.ArchiveEmail(id, folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// moveRequest is the body for POST /v1/emails/{id}/move.
type moveRequest struct {
	FromFolder string `json:"fromFolder"`
	ToFolder   string `json:"toFolder"`
}

// handleMoveEmail moves an email between folders.
func (s *Server) handleMoveEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.FromFolder == "" || req.ToFolder == "" {
		writeError(w, http.StatusBadRequest, "fromFolder and toFolder are required")
		return
	}
	if err := s.backend.MoveEmail(id, req.FromFolder, req.ToFolder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// classifyRequest is the body for POST /v1/emails/{id}/classify.
type classifyRequest struct {
	Category string `json:"category"`
}

// handleClassifyEmail sets the classification for an email.
func (s *Server) handleClassifyEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req classifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Category == "" {
		writeError(w, http.StatusBadRequest, "category is required")
		return
	}
	if err := s.backend.SetClassification(id, req.Category); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMarkRead marks an email as read.
func (s *Server) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	if err := s.backend.MarkRead(id, folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteSender deletes all emails from a sender.
func (s *Server) handleDeleteSender(w http.ResponseWriter, r *http.Request) {
	sender := r.PathValue("sender")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	if err := s.backend.DeleteSenderEmails(sender, folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetStats returns per-sender statistics.
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	stats, err := s.backend.GetSenderStatistics(folder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleSearch searches emails in a single folder.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	bodySearch := r.URL.Query().Get("body") == "true"
	emails, err := s.backend.SearchEmails(folder, q, bodySearch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

// handleSearchAll searches emails across all folders.
func (s *Server) handleSearchAll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	emails, err := s.backend.SearchEmailsCrossFolder(q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

// handleSearchSemantic searches emails using semantic similarity.
func (s *Server) handleSearchSemantic(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	minScore := s.cfg.Semantic.MinScore
	if ms := r.URL.Query().Get("min_score"); ms != "" {
		if f, err := strconv.ParseFloat(ms, 64); err == nil {
			minScore = f
		}
	}
	emails, err := s.backend.SearchEmailsSemantic(folder, q, limit, minScore)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

// handleGetClassifications returns AI classifications for a folder.
func (s *Server) handleGetClassifications(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	classifications, err := s.backend.GetClassifications(folder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, classifications)
}

// handleGetRules returns all enabled rules.
func (s *Server) handleGetRules(w http.ResponseWriter, _ *http.Request) {
	rules, err := s.backend.GetEnabledRules()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleSaveRule creates or updates a rule.
func (s *Server) handleSaveRule(w http.ResponseWriter, r *http.Request) {
	var rule models.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.backend.SaveRule(&rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// handleDeleteRule removes a rule by ID.
func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	if err := s.backend.DeleteRule(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetPrompts returns all custom prompts.
func (s *Server) handleGetPrompts(w http.ResponseWriter, _ *http.Request) {
	prompts, err := s.backend.GetAllCustomPrompts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prompts)
}

// handleMarkUnread marks an email as unread.
func (s *Server) handleMarkUnread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	if err := s.backend.MarkUnread(id, folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMarkStarred sets the \Flagged flag on an email.
func (s *Server) handleMarkStarred(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	if err := s.backend.MarkStarred(id, folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUnmarkStarred removes the \Flagged flag from an email.
func (s *Server) handleUnmarkStarred(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	if err := s.backend.UnmarkStarred(id, folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetThread returns all emails in the same thread as the given subject.
func (s *Server) handleGetThread(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "INBOX"
	}
	subject := r.URL.Query().Get("subject")
	if subject == "" {
		writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	emails, err := s.backend.GetEmailsByThread(folder, subject)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emails)
}

// sendEmailRequest is the body for POST /v1/emails/send.
type sendEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	From    string `json:"from"`
}

// handleSendEmail sends an email via SMTP.
// Note: this route MUST be registered BEFORE POST /v1/emails/{id}/... to avoid
// the literal "send" segment being captured as a path variable.
// Go 1.22 ServeMux resolves literal segments before wildcards, so route order
// within the same pattern group is fine, but we add this comment for clarity.
func (s *Server) handleSendEmail(w http.ResponseWriter, r *http.Request) {
	var req sendEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.To == "" || req.Subject == "" {
		writeError(w, http.StatusBadRequest, "to and subject are required")
		return
	}
	if err := s.backend.SendEmail(req.To, req.Subject, req.Body, req.From); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

// handleSavePrompt creates or updates a custom prompt.
func (s *Server) handleSavePrompt(w http.ResponseWriter, r *http.Request) {
	var prompt models.CustomPrompt
	if err := json.NewDecoder(r.Body).Decode(&prompt); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.backend.SaveCustomPrompt(&prompt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prompt)
}

// handleListCleanupRules returns all cleanup rules.
func (s *Server) handleListCleanupRules(w http.ResponseWriter, _ *http.Request) {
	rules, err := s.backend.GetAllCleanupRules()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleCreateCleanupRule creates or updates a cleanup rule.
func (s *Server) handleCreateCleanupRule(w http.ResponseWriter, r *http.Request) {
	var rule models.CleanupRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if rule.Name == "" || rule.MatchValue == "" {
		writeError(w, http.StatusBadRequest, "name and match_value are required")
		return
	}
	if rule.MatchType != "sender" && rule.MatchType != "domain" {
		http.Error(w, "match_type must be 'sender' or 'domain'", http.StatusBadRequest)
		return
	}
	if rule.Action != "delete" && rule.Action != "archive" {
		http.Error(w, "action must be 'delete' or 'archive'", http.StatusBadRequest)
		return
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	if err := s.backend.SaveCleanupRule(&rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// handleDeleteCleanupRule removes a cleanup rule by ID.
func (s *Server) handleDeleteCleanupRule(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	if err := s.backend.DeleteCleanupRule(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleClassifyFolder classifies all unclassified emails in a folder.
func (s *Server) handleClassifyFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Folder == "" {
		writeError(w, http.StatusBadRequest, "folder is required")
		return
	}
	if s.classifier == nil {
		writeError(w, http.StatusServiceUnavailable, "no AI classifier configured")
		return
	}

	// Get unclassified message IDs for the folder
	ids, err := s.cache.GetUnclassifiedIDs(req.Folder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Count total emails in folder to compute skipped
	allClassifications, err := s.cache.GetClassifications(req.Folder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	skipped := len(allClassifications)
	total := len(ids) + skipped

	classified := 0
	for _, msgID := range ids {
		email, err := s.cache.GetEmailByID(msgID)
		if err != nil || email == nil {
			continue
		}
		cat, err := s.classifier.Classify(email.Sender, email.Subject)
		if err != nil {
			continue
		}
		if err := s.cache.SetClassification(msgID, string(cat)); err == nil {
			classified++
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{
		"classified": classified,
		"skipped":    skipped,
		"total":      total,
	})
}

// saveDraftRequest is the body for POST /v1/drafts.
type saveDraftRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// handleSaveDraft saves a draft email to the IMAP Drafts folder.
func (s *Server) handleSaveDraft(w http.ResponseWriter, r *http.Request) {
	var req saveDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	uid, folder, err := s.backend.SaveDraft(req.To, req.Subject, req.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"uid": uid, "folder": folder})
}

// handleListDrafts returns all draft emails from the IMAP Drafts folder.
func (s *Server) handleListDrafts(w http.ResponseWriter, _ *http.Request) {
	drafts, err := s.backend.ListDrafts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if drafts == nil {
		drafts = []*models.Draft{}
	}
	writeJSON(w, http.StatusOK, drafts)
}

// handleDeleteDraft removes a draft by UID from the given folder.
func (s *Server) handleDeleteDraft(w http.ResponseWriter, r *http.Request) {
	uidStr := r.PathValue("uid")
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uid")
		return
	}
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "Drafts"
	}
	if err := s.backend.DeleteDraft(uint32(uid64), folder); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSendDraft fetches a draft, sends it via SMTP, then deletes it.
func (s *Server) handleSendDraft(w http.ResponseWriter, r *http.Request) {
	uidStr := r.PathValue("uid")
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid uid")
		return
	}
	uid := uint32(uid64)
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		folder = "Drafts"
	}

	drafts, err := s.backend.ListDrafts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var draft *models.Draft
	for _, d := range drafts {
		if d.UID == uid {
			draft = d
			break
		}
	}
	if draft == nil {
		writeError(w, http.StatusNotFound, "draft not found")
		return
	}

	// Fetch the full body — ListDrafts only returns envelope metadata.
	body, fetchErr := s.backend.FetchEmailBody(draft.Folder, draft.UID)
	if fetchErr != nil {
		logger.Warn("daemon: fetch draft body uid=%d: %v — sending with empty body", draft.UID, fetchErr)
	} else if body != nil {
		draft.Body = body.TextPlain
	}

	if err := s.backend.SendEmail(draft.To, draft.Subject, draft.Body, ""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.backend.DeleteDraft(uid, folder); err != nil {
		logger.Error("daemon: delete draft after send: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Draft sent and deleted"})
}

// handleRunCleanupRules triggers immediate execution of all enabled cleanup rules.
// The cleanup engine runs in-process using the server's backend and cache.
func (s *Server) handleRunCleanupRules(w http.ResponseWriter, r *http.Request) {
	engine := cleanup.NewEngine(s.cache, s.backend, s.log)
	results, err := engine.RunAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	total := 0
	for _, n := range results {
		total += n
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"results": results, "total": total})
}
