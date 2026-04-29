package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/herald-email/herald-mail-app/internal/logger"
	"github.com/herald-email/herald-mail-app/internal/models"
)

type imagePreviewLink struct {
	Label     string
	URL       string
	MIMEType  string
	Size      int
	ContentID string
}

type imagePreviewEntry struct {
	token    string
	mimeType string
	data     []byte
}

type imagePreviewServer struct {
	mu         sync.Mutex
	server     *http.Server
	listener   net.Listener
	base       string
	currentKey string
	entries    map[string]imagePreviewEntry
	links      []imagePreviewLink
}

func newImagePreviewServer() *imagePreviewServer {
	return &imagePreviewServer{
		entries: make(map[string]imagePreviewEntry),
	}
}

func (s *imagePreviewServer) baseURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.base
}

func (s *imagePreviewServer) CurrentKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentKey
}

func (s *imagePreviewServer) RegisterSet(key string, images []models.InlineImage) ([]imagePreviewLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(images) == 0 {
		s.clearLocked()
		return nil, nil
	}
	if s.currentKey == key && len(s.links) == previewableImageCount(images) {
		return append([]imagePreviewLink(nil), s.links...), nil
	}
	if err := s.ensureStartedLocked(); err != nil {
		return nil, err
	}

	s.clearLocked()
	s.currentKey = key
	for i, img := range images {
		if len(img.Data) == 0 {
			continue
		}
		token, err := randomImageToken()
		if err != nil {
			s.clearLocked()
			return nil, err
		}
		mimeType := strings.TrimSpace(img.MIMEType)
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			mimeType = "application/octet-stream"
		}
		entry := imagePreviewEntry{
			token:    token,
			mimeType: mimeType,
			data:     append([]byte(nil), img.Data...),
		}
		s.entries[token] = entry
		s.links = append(s.links, imagePreviewLink{
			Label:     fmt.Sprintf("open image %d", i+1),
			URL:       s.base + "/image/" + token,
			MIMEType:  mimeType,
			Size:      len(img.Data),
			ContentID: inlineImageDocumentKey(img, i),
		})
	}
	return append([]imagePreviewLink(nil), s.links...), nil
}

func previewableImageCount(images []models.InlineImage) int {
	count := 0
	for _, img := range images {
		if len(img.Data) > 0 {
			count++
		}
	}
	return count
}

func (s *imagePreviewServer) RevokeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLocked()
}

func (s *imagePreviewServer) Close() {
	s.mu.Lock()
	server := s.server
	s.server = nil
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}
	s.base = ""
	s.clearLocked()
	s.mu.Unlock()

	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
}

func (s *imagePreviewServer) ensureStartedLocked() error {
	if s.server != nil {
		return nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port
	s.base = fmt.Sprintf("http://127.0.0.1:%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/image/", s.handleImage)
	server := &http.Server{Handler: mux}
	s.server = server
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Warn("image preview server stopped unexpectedly: %v", err)
		}
	}()
	return nil
}

func (s *imagePreviewServer) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/image/")
	if token == "" || strings.Contains(token, "/") {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	entry, ok := s.entries[token]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", entry.mimeType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(entry.data)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(entry.data)
}

func (s *imagePreviewServer) clearLocked() {
	s.currentKey = ""
	for token := range s.entries {
		delete(s.entries, token)
	}
	s.links = nil
}

func randomImageToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
