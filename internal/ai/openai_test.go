package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func openAIOKResponse(content string) openAIChatResponse {
	return openAIChatResponse{
		Choices: []struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []openAIToolCall `json:"tool_calls"`
			} `json:"message"`
		}{{Message: struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		}{Content: content}}},
	}
}

func openAIEmbedOKResponse(vec []float32) openAIEmbedResponse {
	return openAIEmbedResponse{
		Data: []struct {
			Embedding []float32 `json:"embedding"`
		}{{Embedding: vec}},
	}
}

func newTestOpenAIClient(srvURL string) *OpenAICompatClient {
	return NewOpenAICompatClient("test-key", srvURL, "gpt-4o")
}

func TestOpenAICompatClientChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIOKResponse("Hello from OpenAI"))
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL)
	reply, err := c.Chat([]ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "Hello from OpenAI" {
		t.Errorf("got %q", reply)
	}
}

func TestOpenAICompatClientEmbed(t *testing.T) {
	expected := []float32{0.1, 0.2, 0.3}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIEmbedOKResponse(expected))
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL)
	vec, err := c.Embed("test text")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != len(expected) {
		t.Fatalf("expected %d floats, got %d", len(expected), len(vec))
	}
	for i := range expected {
		if vec[i] != expected[i] {
			t.Errorf("vec[%d] = %v, want %v", i, vec[i], expected[i])
		}
	}
}

func TestOpenAICompatClientClassify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIOKResponse("txn"))
	}))
	defer srv.Close()

	c := newTestOpenAIClient(srv.URL)
	cat, err := c.Classify("store@shop.com", "Your order has shipped")
	if err != nil {
		t.Fatal(err)
	}
	if cat != CategoryTransactional {
		t.Errorf("expected txn, got %q", cat)
	}
}

func TestOpenAICompatClientHasVisionModel(t *testing.T) {
	c := NewOpenAICompatClient("key", "", "")
	if !c.HasVisionModel() {
		t.Error("OpenAI should always report HasVisionModel=true")
	}
}

func TestOpenAICompatClientDefaultsApplied(t *testing.T) {
	c := NewOpenAICompatClient("", "", "")
	if c.baseURL != "https://api.openai.com/v1" {
		t.Errorf("unexpected baseURL: %q", c.baseURL)
	}
	if c.model != "gpt-4o" {
		t.Errorf("unexpected model: %q", c.model)
	}
}
