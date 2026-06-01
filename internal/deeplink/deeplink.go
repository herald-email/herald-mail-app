package deeplink

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/models"
)

const (
	Scheme = "herald"
	Host   = "mail"
)

type Kind string

const (
	KindFolder  Kind = "folder"
	KindMessage Kind = "message"
	KindSender  Kind = "sender"
	KindSearch  Kind = "search"
	KindCompose Kind = "compose"
)

type Target struct {
	Kind      Kind
	Folder    string
	MessageID string
	LocalID   string
	Sender    string
	Query     string
	To        string
	Subject   string
	SourceID  models.SourceID
	AccountID models.AccountID
}

func Build(target Target) string {
	values := url.Values{}
	add := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			values.Set(key, value)
		}
	}

	add("folder", target.Folder)
	add("message_id", target.MessageID)
	add("local_id", target.LocalID)
	add("sender", target.Sender)
	add("q", target.Query)
	add("to", target.To)
	add("subject", target.Subject)
	add("source_id", string(target.SourceID))
	add("account_id", string(target.AccountID))

	u := url.URL{
		Scheme:   Scheme,
		Host:     Host,
		Path:     "/" + string(target.Kind),
		RawQuery: values.Encode(),
	}
	return u.String()
}

func Parse(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, fmt.Errorf("empty Herald deep link")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("parse Herald deep link: %w", err)
	}
	if parsed.Scheme != Scheme || parsed.Host != Host {
		return Target{}, fmt.Errorf("unsupported Herald deep link target")
	}

	kind := Kind(strings.Trim(parsed.EscapedPath(), "/"))
	values := parsed.Query()
	target := Target{
		Kind:      kind,
		Folder:    strings.TrimSpace(values.Get("folder")),
		MessageID: strings.TrimSpace(values.Get("message_id")),
		LocalID:   strings.TrimSpace(values.Get("local_id")),
		Sender:    strings.TrimSpace(values.Get("sender")),
		Query:     strings.TrimSpace(values.Get("q")),
		To:        strings.TrimSpace(values.Get("to")),
		Subject:   strings.TrimSpace(values.Get("subject")),
		SourceID:  models.SourceID(strings.TrimSpace(values.Get("source_id"))),
		AccountID: models.AccountID(strings.TrimSpace(values.Get("account_id"))),
	}

	switch kind {
	case KindFolder:
		if target.Folder == "" {
			return Target{}, fmt.Errorf("folder deep link requires folder")
		}
	case KindMessage:
		if target.Folder == "" || target.MessageID == "" {
			return Target{}, fmt.Errorf("message deep link requires folder and message_id")
		}
	case KindSender:
		if target.Folder == "" || target.Sender == "" {
			return Target{}, fmt.Errorf("sender deep link requires folder and sender")
		}
	case KindSearch:
		if target.Query == "" {
			return Target{}, fmt.Errorf("search deep link requires q")
		}
	case KindCompose:
		if target.To == "" && target.Subject == "" {
			return Target{}, fmt.Errorf("compose deep link requires to or subject")
		}
	default:
		return Target{}, fmt.Errorf("unknown Herald deep link kind %q", kind)
	}

	return target, nil
}

func MessageTarget(email *models.EmailData) Target {
	if email == nil {
		return Target{Kind: KindMessage}
	}
	ref := email.MessageRef()
	return Target{
		Kind:      KindMessage,
		Folder:    ref.Folder,
		MessageID: ref.MessageID,
		LocalID:   ref.LocalID,
		SourceID:  ref.SourceID,
		AccountID: ref.AccountID,
	}
}

func FolderTarget(folder string, sourceID models.SourceID, accountID models.AccountID) Target {
	return Target{
		Kind:      KindFolder,
		Folder:    strings.TrimSpace(folder),
		SourceID:  sourceID,
		AccountID: accountID,
	}
}
