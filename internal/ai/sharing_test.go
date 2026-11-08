package ai

import (
	"github.com/herald-email/herald-mail-app/internal/config"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSharingPreventsRequestsAndSeparatesBackground(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer srv.Close()
	cfg := &config.Config{}
	cfg.AI.Provider = "ollama"
	cfg.Ollama.Host = "http://localhost:11434"
	cfg.Semantic.Provider = "openai"
	cfg.OpenAI.APIKey = "fake"
	cfg.OpenAI.BaseURL = srv.URL
	client, err := NewFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Embed("private message"); err == nil || hits.Load() != 0 {
		t.Fatal("unconsented request escaped")
	}
	cfg.GrantSharing(true, false, false)
	if _, err = WithPriority(client, PriorityBackground).Embed("private message"); err == nil || hits.Load() != 0 {
		t.Fatal("background request escaped without separate consent")
	}
	if _, err = client.Embed("user query"); err != nil {
		t.Fatal(err)
	}
	cfg.GrantSharing(true, true, false)
	if _, err = WithPriority(client, PriorityBackground).Embed("private message"); err != nil {
		t.Fatal(err)
	}
	cfg.OpenAI.BaseURL = srv.URL + "/changed"
	if _, err = client.Embed("private message"); err == nil {
		t.Fatal("destination change retained consent")
	}
	if hits.Load() != 2 {
		t.Fatalf("requests=%d want 2", hits.Load())
	}
}
