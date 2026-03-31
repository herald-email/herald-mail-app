package ai

import (
	"context"
	"testing"
)

type mockAI struct {
	embedCalled bool
	chatCalled  bool
}

func (m *mockAI) Chat(messages []ChatMessage) (string, error) {
	m.chatCalled = true
	return "ok", nil
}
func (m *mockAI) Classify(_, _ string) (Category, error)    { return "imp", nil }
func (m *mockAI) Embed(_ string) ([]float32, error)          { m.embedCalled = true; return []float32{1, 2, 3}, nil }
func (m *mockAI) SetEmbeddingModel(_ string)                 {}
func (m *mockAI) GenerateQuickReplies(_, _, _ string) ([]string, error) { return nil, nil }
func (m *mockAI) EnrichContact(_ string, _ []string) (string, []string, error) {
	return "", nil, nil
}
func (m *mockAI) HasVisionModel() bool { return false }
func (m *mockAI) DescribeImage(_ context.Context, _ []byte, _ string) (string, error) {
	return "", nil
}
func (m *mockAI) Ping() error { return nil }

// compile-time check
var _ AIClient = (*mockAI)(nil)

func TestCompositeClientEmbedRoutesToEmbeddingBackend(t *testing.T) {
	primary := &mockAI{}
	embedding := &mockAI{}
	c := NewCompositeClient(primary, embedding)
	c.Embed("test")
	if !embedding.embedCalled {
		t.Error("Embed should route to embedding backend")
	}
	if primary.embedCalled {
		t.Error("Embed should NOT call primary backend")
	}
}

func TestCompositeClientEmbedFallsThroughWhenNoEmbedding(t *testing.T) {
	primary := &mockAI{}
	c := NewCompositeClient(primary, nil)
	c.Embed("test")
	if !primary.embedCalled {
		t.Error("Embed should fall through to primary when no embedding backend")
	}
}

func TestCompositeClientChatRoutesToPrimary(t *testing.T) {
	primary := &mockAI{}
	embedding := &mockAI{}
	c := NewCompositeClient(primary, embedding)
	c.Chat(nil)
	if !primary.chatCalled {
		t.Error("Chat should route to primary backend")
	}
}
