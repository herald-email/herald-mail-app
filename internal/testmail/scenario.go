package testmail

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	goImap "github.com/emersion/go-imap"
)

// ScenarioName identifies one sanitized realistic mail scenario.
type ScenarioName string

const (
	ScenarioPlainThread      ScenarioName = "plain-thread"
	ScenarioCalendlyInvite   ScenarioName = "calendly-invite"
	ScenarioNewsletterTable  ScenarioName = "newsletter-table"
	ScenarioReceiptHTML      ScenarioName = "receipt-html"
	ScenarioMalformedCharset ScenarioName = "malformed-charset"
	ScenarioInlineCIDImage   ScenarioName = "inline-cid-image"
	ScenarioLongLinkTracking ScenarioName = "long-link-tracking"
)

// Scenario is a named set of fixture placements ready to seed into a Lab.
type Scenario struct {
	Name     ScenarioName
	Messages []ScenarioMessage
}

// ScenarioMessage is one sanitized fixture plus its default virtual mailbox
// placement.
type ScenarioMessage struct {
	Key       string
	File      string
	Account   string
	Folder    string
	Subject   string
	MessageID string
	Flags     []string
	Data      []byte
}

// SeededScenario is a running lab with one named scenario appended.
type SeededScenario struct {
	Name     ScenarioName
	Lab      *Lab
	Messages []ScenarioMessage
	Refs     map[string]MessageRef
}

// CorpusRoot returns the absolute path to the committed sanitized corpus.
func CorpusRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("testdata", "corpus")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "corpus")
}

// LoadScenario loads a named sanitized corpus scenario and applies the default
// virtual account/folder placement for tests.
func LoadScenario(name ScenarioName) (Scenario, error) {
	scenarios, err := LoadCorpus(CorpusRoot())
	if err != nil {
		return Scenario{}, err
	}
	corpus, ok := scenarios[string(name)]
	if !ok {
		return Scenario{}, fmt.Errorf("testmail: unknown scenario %q", name)
	}
	messages, err := scenarioMessages(name, corpus)
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{Name: name, Messages: messages}, nil
}

// StartScenario starts a virtual mail lab and appends the named scenario.
func StartScenario(t testing.TB, name ScenarioName, opts ...Option) *SeededScenario {
	t.Helper()
	scenario, err := LoadScenario(name)
	if err != nil {
		t.Fatalf("testmail: load scenario %q: %v", name, err)
	}
	lab := Start(t, opts...)
	refs := make(map[string]MessageRef, len(scenario.Messages))
	for _, msg := range scenario.Messages {
		account := lab.Account(msg.Account)
		if account == nil {
			t.Fatalf("testmail: scenario %q account %q not found", name, msg.Account)
		}
		refs[msg.Key] = account.AppendEML(msg.Folder, msg.Data, msg.Flags...)
	}
	return &SeededScenario{
		Name:     scenario.Name,
		Lab:      lab,
		Messages: cloneScenarioMessages(scenario.Messages),
		Refs:     refs,
	}
}

func scenarioMessages(name ScenarioName, corpus CorpusScenario) ([]ScenarioMessage, error) {
	byFile := make(map[string]CorpusMessage, len(corpus.Messages))
	for _, msg := range corpus.Messages {
		byFile[msg.Name] = msg
	}

	switch name {
	case ScenarioPlainThread:
		original, ok := byFile["original.eml"]
		if !ok {
			return nil, fmt.Errorf("testmail: %s missing original.eml", name)
		}
		reply, ok := byFile["reply.eml"]
		if !ok {
			return nil, fmt.Errorf("testmail: %s missing reply.eml", name)
		}
		return []ScenarioMessage{
			mustScenarioMessage("original", original, DefaultAliceAddress, "INBOX", nil),
			mustScenarioMessage("reply-sent", reply, DefaultAliceAddress, "Sent", []string{goImap.SeenFlag}),
			mustScenarioMessage("reply-inbox", reply, DefaultBobAddress, "INBOX", nil),
		}, nil
	default:
		messages := make([]ScenarioMessage, 0, len(corpus.Messages))
		for _, msg := range corpus.Messages {
			key := strings.TrimSuffix(msg.Name, filepath.Ext(msg.Name))
			messages = append(messages, mustScenarioMessage(key, msg, DefaultAliceAddress, "INBOX", nil))
		}
		sort.Slice(messages, func(i, j int) bool { return messages[i].Key < messages[j].Key })
		return messages, nil
	}
}

func mustScenarioMessage(key string, corpusMsg CorpusMessage, account, folder string, flags []string) ScenarioMessage {
	subject, messageID := parseScenarioHeaders(corpusMsg.Data)
	return ScenarioMessage{
		Key:       key,
		File:      corpusMsg.Name,
		Account:   account,
		Folder:    folder,
		Subject:   subject,
		MessageID: messageID,
		Flags:     append([]string(nil), flags...),
		Data:      append([]byte(nil), corpusMsg.Data...),
	}
}

func parseScenarioHeaders(data []byte) (subject, messageID string) {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return "", ""
	}
	subject = msg.Header.Get("Subject")
	if decoded, err := (&mime.WordDecoder{}).DecodeHeader(subject); err == nil {
		subject = decoded
	}
	return subject, msg.Header.Get("Message-ID")
}

func cloneScenarioMessages(messages []ScenarioMessage) []ScenarioMessage {
	out := make([]ScenarioMessage, len(messages))
	for i, msg := range messages {
		out[i] = msg
		out[i].Flags = append([]string(nil), msg.Flags...)
		out[i].Data = append([]byte(nil), msg.Data...)
	}
	return out
}
