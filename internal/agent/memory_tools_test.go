package agent

import (
	"context"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/herald-email/herald-mail-app/internal/memory"
)

type fakeMemoryToolSource struct {
	memories    []memory.Memory
	replyPrep   memory.ReplyPrep
	searchCalls int
	replyCalls  int
	lastQuery   memory.Query
	lastReply   memory.ReplyPrepQuery
}

func (f *fakeMemoryToolSource) SearchMemories(_ context.Context, query memory.Query) ([]memory.Memory, error) {
	f.searchCalls++
	f.lastQuery = query
	out := make([]memory.Memory, 0, len(f.memories))
	for _, mem := range f.memories {
		if memory.MemoryMatches(mem, query) {
			out = append(out, mem)
		}
	}
	return out, nil
}

func (f *fakeMemoryToolSource) BuildReplyMemoryContext(_ context.Context, query memory.ReplyPrepQuery) (memory.ReplyPrep, error) {
	f.replyCalls++
	f.lastReply = query
	if f.replyPrep.Memories != nil || f.replyPrep.Nudges != nil {
		return f.replyPrep, nil
	}
	return memory.BuildReplyPrepFromMemories(query, f.memories, memory.DefaultSettings()), nil
}

func TestMemoryToolServiceSearchAndReplyPrep(t *testing.T) {
	source := &fakeMemoryToolSource{memories: []memory.Memory{
		agentMemory("mem-1", memory.KindOpenQuestion, "sergey@example.com", "Sergey asked whether you can send availability by Friday.", 0.91),
		agentMemory("mem-2", memory.KindCommitment, "alex@example.com", "Alex mentioned a lower-confidence task.", 0.20),
	}}
	service := NewMemoryToolService(source, MemoryToolOptions{MaxResults: 5, ChatMinScore: 0.35, ComposeMinScore: 0.75})

	result, err := service.ContactHistory(context.Background(), ContactHistoryParams{Person: "sergey@example.com"})
	if err != nil {
		t.Fatalf("ContactHistory returned error: %v", err)
	}
	if source.searchCalls != 1 || result.Total != 1 || result.Memories[0].ID != "mem-1" {
		t.Fatalf("contact history result = %#v calls=%d", result, source.searchCalls)
	}

	prep, err := service.ReplyMemoryContext(context.Background(), ReplyMemoryContextParams{
		Recipient: "sergey@example.com",
		Subject:   "Interview",
	})
	if err != nil {
		t.Fatalf("ReplyMemoryContext returned error: %v", err)
	}
	if source.replyCalls != 1 || source.lastReply.Recipient != "sergey@example.com" {
		t.Fatalf("reply source calls=%d query=%#v", source.replyCalls, source.lastReply)
	}
	if source.lastReply.MinConfidence != 0.75 {
		t.Fatalf("reply min confidence = %.2f, want Compose threshold 0.75", source.lastReply.MinConfidence)
	}
	if len(prep.Nudges) != 1 || prep.Nudges[0].Evidence[0].MessageID == "" {
		t.Fatalf("reply prep = %#v", prep)
	}
}

func TestGollemRunnerCanCallMemoryTool(t *testing.T) {
	emailSource := &fakeEmailToolSource{}
	memorySource := &fakeMemoryToolSource{memories: []memory.Memory{
		agentMemory("mem-1", memory.KindOpenQuestion, "sergey@example.com", "Sergey asked for an update.", 0.91),
	}}
	model := core.NewTestModel(
		core.ToolCallResponse("get_contact_history", `{"person":"sergey@example.com"}`),
		core.ToolCallResponse("final_result", `{"reply":"Sergey asked for an update. Source mem-1."}`),
	)
	runner := NewGollemRunnerWithEmailAndMemoryToolsAndOptions(
		model,
		emailSource,
		EmailToolOptions{MaxResults: 5},
		memorySource,
		MemoryToolOptions{MaxResults: 5},
	)

	result, err := runner.Run(context.Background(), ChatInput{UserMessage: "what should I remember about Sergey?"})
	if err != nil {
		t.Fatalf("runner returned error: %v", err)
	}
	if result.Reply == "" || memorySource.searchCalls != 1 {
		t.Fatalf("runner result=%#v memory calls=%d", result, memorySource.searchCalls)
	}
}

func agentMemory(id, kind, person, claim string, confidence float64) memory.Memory {
	return memory.PrepareMemoryForAppend(memory.Memory{
		ID:             id,
		Kind:           kind,
		Claim:          claim,
		Summary:        claim,
		Topic:          "Interview",
		People:         []string{person},
		Company:        "Example",
		Domain:         "example.com",
		Status:         memory.StatusWaiting,
		Confidence:     confidence,
		LastActivityAt: time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Evidence: []memory.Evidence{{
			SourceType: memory.SourceEmail,
			MessageID:  id + "-msg",
			Folder:     "INBOX",
			Date:       time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
			Snippet:    claim,
		}},
	}, time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC))
}
