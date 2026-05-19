package accountcheck

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResultErrNamesFailedSurfaces(t *testing.T) {
	result := Result{
		IMAP: Check{Surface: "IMAP", Err: errors.New("login rejected")},
		SMTP: Check{Surface: "SMTP", Err: errors.New("auth rejected")},
	}

	err := result.Err()
	if err == nil {
		t.Fatal("expected combined error")
	}
	if got := err.Error(); !strings.Contains(got, "IMAP") || !strings.Contains(got, "SMTP") {
		t.Fatalf("combined error should name both failed surfaces, got %q", got)
	}
}

func TestResultUserMessageExplainsConfigWasNotSaved(t *testing.T) {
	result := Result{
		IMAP: Check{Surface: "IMAP", Err: errors.New("login rejected")},
		SMTP: Check{Surface: "SMTP"},
	}

	msg := result.UserMessage("/tmp/herald.log", "/tmp/herald.yaml")
	for _, want := range []string{"IMAP", "not saved", "/tmp/herald.yaml", "/tmp/herald.log"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("UserMessage missing %q in %q", want, msg)
		}
	}
}

func TestValidateNilConfigFailsBothSurfaces(t *testing.T) {
	result := Validate(context.Background(), nil, "")
	if result.IMAP.OK() || result.SMTP.OK() {
		t.Fatalf("expected nil config to fail both surfaces, got %#v", result)
	}
	if err := result.Err(); err == nil || !strings.Contains(err.Error(), "IMAP") || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("expected combined nil-config error, got %v", err)
	}
}
