package models

import "strings"

type SourceID string
type AccountID string

type SourceKind string

const (
	DefaultMailSourceID     SourceID  = "default-mail"
	DefaultCalendarSourceID SourceID  = "default-calendar"
	DefaultAccountID        AccountID = "default"

	SourceKindMail     SourceKind = "mail"
	SourceKindCalendar SourceKind = "calendar"
)

type CollectionRef struct {
	SourceID     SourceID
	AccountID    AccountID
	Kind         SourceKind
	CollectionID string
	DisplayName  string
}

type MessageRef struct {
	SourceID    SourceID
	AccountID   AccountID
	Folder      string
	UID         uint32
	UIDValidity uint32
	MessageID   string
	LocalID     string
}

type EventRef struct {
	SourceID   SourceID
	AccountID  AccountID
	CalendarID string
	EventID    string
	InstanceID string
	ETag       string
	LocalID    string
}

func NormalizeSourceID(id SourceID, fallback SourceID) SourceID {
	if strings.TrimSpace(string(id)) != "" {
		return id
	}
	if fallback != "" {
		return fallback
	}
	return DefaultMailSourceID
}

func NormalizeAccountID(id AccountID) AccountID {
	if strings.TrimSpace(string(id)) != "" {
		return id
	}
	return DefaultAccountID
}

func DefaultSourceIDForKind(kind SourceKind) SourceID {
	if kind == SourceKindCalendar {
		return DefaultCalendarSourceID
	}
	return DefaultMailSourceID
}

func (r CollectionRef) CacheKey() string {
	kind := r.Kind
	if kind == "" {
		kind = SourceKindMail
	}
	return strings.Join([]string{
		string(kind),
		string(NormalizeSourceID(r.SourceID, DefaultSourceIDForKind(kind))),
		string(NormalizeAccountID(r.AccountID)),
		r.CollectionID,
	}, ":")
}

func (r MessageRef) WithDefaults() MessageRef {
	r.SourceID = NormalizeSourceID(r.SourceID, DefaultMailSourceID)
	r.AccountID = NormalizeAccountID(r.AccountID)
	if r.LocalID == "" {
		r.LocalID = strings.Join([]string{
			string(SourceKindMail),
			string(r.SourceID),
			string(r.AccountID),
			r.Folder,
			r.MessageID,
		}, ":")
	}
	return r
}

func (r EventRef) WithDefaults() EventRef {
	r.SourceID = NormalizeSourceID(r.SourceID, DefaultCalendarSourceID)
	r.AccountID = NormalizeAccountID(r.AccountID)
	if r.LocalID == "" {
		r.LocalID = strings.Join([]string{
			string(SourceKindCalendar),
			string(r.SourceID),
			string(r.AccountID),
			r.CalendarID,
			r.EventID,
			r.InstanceID,
		}, ":")
	}
	return r
}
