package notifications

import (
	"context"
	"testing"
)

func TestRecorderCapturesNotificationAndActivation(t *testing.T) {
	rec := NewRecorder()
	req := Request{
		ID:       "msg-1",
		Title:    "New mail",
		Body:     "From Alice",
		DeepLink: "herald://mail/message?folder=INBOX&message_id=msg-1",
		Sound:    true,
	}
	if err := rec.Notify(context.Background(), req); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	delivered := rec.Delivered()
	if len(delivered) != 1 || delivered[0] != req {
		t.Fatalf("Delivered = %#v, want %#v", delivered, []Request{req})
	}

	rec.Activate(req.DeepLink)
	select {
	case got := <-rec.Responses():
		if got != req.DeepLink {
			t.Fatalf("activation = %q, want %q", got, req.DeepLink)
		}
	default:
		t.Fatal("expected activation response")
	}
}

func TestDisabledNotifierDoesNotDeliver(t *testing.T) {
	notifier := New(Options{Enabled: false})
	if err := notifier.Notify(context.Background(), Request{Title: "Hidden", Body: "No-op"}); err != nil {
		t.Fatalf("disabled Notify returned error: %v", err)
	}
	if caps := notifier.Capabilities(); caps.Delivery || caps.Activation {
		t.Fatalf("disabled capabilities = %#v, want no delivery or activation", caps)
	}
}
