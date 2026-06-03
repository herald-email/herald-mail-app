package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/herald-email/herald-mail-app/internal/backend"
	"github.com/herald-email/herald-mail-app/internal/models"
)

func TestDaemonCalendarEventCreateUpdateDelete(t *testing.T) {
	b := backend.NewDemoBackend()
	collections, err := b.ListCalendarCollections()
	if err != nil {
		t.Fatalf("ListCalendarCollections: %v", err)
	}
	if len(collections) == 0 {
		t.Fatal("demo backend has no calendar collections")
	}
	collection := collections[0].Ref
	s := &Server{backend: b, broadcaster: NewBroadcaster()}

	createPayload := map[string]any{
		"source_id":   string(collection.SourceID),
		"account_id":  string(collection.AccountID),
		"calendar_id": collection.CollectionID,
		"event_id":    "daemon-calendar-create",
		"title":       "Daemon calendar create",
		"start":       "2026-06-03T16:00:00Z",
		"end":         "2026-06-03T16:30:00Z",
		"timezone":    "UTC",
	}
	createReq := jsonRequest(t, http.MethodPost, "/v1/calendar/events", createPayload)
	createRR := httptest.NewRecorder()
	s.handleCreateCalendarEvent(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", createRR.Code, createRR.Body.String())
	}
	var created models.CalendarEvent
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Title != "Daemon calendar create" || created.Ref.EventID != "daemon-calendar-create" {
		t.Fatalf("created = %#v", created)
	}

	updatePayload := map[string]any{"title": "Daemon calendar updated", "location": "Room D"}
	updateURL := "/v1/calendar/events/" + url.PathEscape(created.Ref.EventID) + "?source_id=" + url.QueryEscape(string(created.Ref.SourceID)) + "&account_id=" + url.QueryEscape(string(created.Ref.AccountID)) + "&calendar_id=" + url.QueryEscape(created.Ref.CalendarID)
	updateReq := jsonRequest(t, http.MethodPatch, updateURL, updatePayload)
	updateReq.SetPathValue("id", created.Ref.EventID)
	updateRR := httptest.NewRecorder()
	s.handleUpdateCalendarEvent(updateRR, updateReq)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", updateRR.Code, updateRR.Body.String())
	}
	updated, err := b.GetCalendarEvent(created.Ref)
	if err != nil {
		t.Fatalf("GetCalendarEvent after update: %v", err)
	}
	if updated.Title != "Daemon calendar updated" || updated.Location != "Room D" {
		t.Fatalf("updated = %#v", updated)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, updateURL, nil)
	deleteReq.SetPathValue("id", created.Ref.EventID)
	deleteRR := httptest.NewRecorder()
	s.handleDeleteCalendarEvent(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", deleteRR.Code, deleteRR.Body.String())
	}
	if _, err := b.GetCalendarEvent(created.Ref); err == nil {
		t.Fatal("deleted event still returned from demo backend")
	}
}

func jsonRequest(t *testing.T, method, target string, payload any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		t.Fatalf("encode payload: %v", err)
	}
	return httptest.NewRequest(method, target, &buf)
}
