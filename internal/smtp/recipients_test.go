package smtp

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeAddressList_AcceptsAutocompleteDisplayNameWithTrailingComma(t *testing.T) {
	header, envelope, err := normalizeAddressList("To", "Rowan Finch <rowan@example.com>, ", true)
	if err != nil {
		t.Fatalf("normalizeAddressList returned error: %v", err)
	}
	if header != "Rowan Finch <rowan@example.com>" {
		t.Fatalf("header = %q, want display-name address without trailing comma", header)
	}
	if want := []string{"rowan@example.com"}; !reflect.DeepEqual(envelope, want) {
		t.Fatalf("envelope = %#v, want %#v", envelope, want)
	}
}

func TestNormalizeAddressList_MultipleRecipientsUseBareEnvelopeAddresses(t *testing.T) {
	header, envelope, err := normalizeAddressList("To", "alice@example.com, Bob Example <bob@example.com>", true)
	if err != nil {
		t.Fatalf("normalizeAddressList returned error: %v", err)
	}
	if header != "alice@example.com, Bob Example <bob@example.com>" {
		t.Fatalf("header = %q, want canonical address list", header)
	}
	if want := []string{"alice@example.com", "bob@example.com"}; !reflect.DeepEqual(envelope, want) {
		t.Fatalf("envelope = %#v, want %#v", envelope, want)
	}
}

func TestNormalizeAddressList_CCAndBCCDisplayNamesNormalizeForEnvelope(t *testing.T) {
	_, ccEnvelope, err := normalizeAddressList("Cc", "Copy Person <copy@example.com>", false)
	if err != nil {
		t.Fatalf("normalizeAddressList Cc returned error: %v", err)
	}
	if want := []string{"copy@example.com"}; !reflect.DeepEqual(ccEnvelope, want) {
		t.Fatalf("cc envelope = %#v, want %#v", ccEnvelope, want)
	}

	_, bccEnvelope, err := normalizeAddressList("Bcc", "Blind Person <blind@example.com>", false)
	if err != nil {
		t.Fatalf("normalizeAddressList Bcc returned error: %v", err)
	}
	if want := []string{"blind@example.com"}; !reflect.DeepEqual(bccEnvelope, want) {
		t.Fatalf("bcc envelope = %#v, want %#v", bccEnvelope, want)
	}
}

func TestNormalizeAddressList_QuotedDisplayNameContainingComma(t *testing.T) {
	header, envelope, err := normalizeAddressList("To", `"Doe, Jane" <jane@example.com>, Ops <ops@example.com>`, true)
	if err != nil {
		t.Fatalf("normalizeAddressList returned error: %v", err)
	}
	if header != `"Doe, Jane" <jane@example.com>, Ops <ops@example.com>` {
		t.Fatalf("header = %q, want quoted display name preserved", header)
	}
	if want := []string{"jane@example.com", "ops@example.com"}; !reflect.DeepEqual(envelope, want) {
		t.Fatalf("envelope = %#v, want %#v", envelope, want)
	}
}

func TestNormalizeAddressList_InvalidRecipientReturnsFieldError(t *testing.T) {
	_, _, err := normalizeAddressList("To", "not an address", true)
	if err == nil {
		t.Fatal("expected invalid recipient error")
	}
	if !strings.Contains(err.Error(), "To") || !strings.Contains(err.Error(), "not an address") {
		t.Fatalf("error = %q, want field name and invalid value", err.Error())
	}
}

func TestNormalizeMailbox_UsesBareEnvelopeAddress(t *testing.T) {
	header, envelope, err := normalizeMailbox("From", "Herald User <me@example.com>")
	if err != nil {
		t.Fatalf("normalizeMailbox returned error: %v", err)
	}
	if header != "Herald User <me@example.com>" {
		t.Fatalf("header = %q, want display-name sender", header)
	}
	if envelope != "me@example.com" {
		t.Fatalf("envelope = %q, want bare sender address", envelope)
	}
}

func TestNormalizeRecipientFields_CombinesToCcAndBccEnvelopeOnly(t *testing.T) {
	rcpts, err := normalizeRecipientFields(
		"To Person <to@example.com>",
		"Copy Person <copy@example.com>",
		"Blind Person <blind@example.com>",
	)
	if err != nil {
		t.Fatalf("normalizeRecipientFields returned error: %v", err)
	}
	if rcpts.CCHeader != "Copy Person <copy@example.com>" {
		t.Fatalf("CCHeader = %q, want display-name Cc header", rcpts.CCHeader)
	}
	if want := []string{"to@example.com", "copy@example.com", "blind@example.com"}; !reflect.DeepEqual(rcpts.AllEnvelope, want) {
		t.Fatalf("AllEnvelope = %#v, want %#v", rcpts.AllEnvelope, want)
	}
}
