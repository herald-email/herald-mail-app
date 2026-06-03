package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/herald-email/herald-mail-app/internal/config"
	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/work"
)

const (
	MessageReadSourceCache       = "cache"
	MessageReadSourceProvider    = "imap"
	MessageReadSourceUnavailable = "unavailable"
)

type MessageReadClass string

const (
	MessageReadClassInteractive MessageReadClass = "interactive"
	MessageReadClassBackground  MessageReadClass = "background"
)

type messageReadClassContextKey struct{}

func withMessageReadClass(ctx context.Context, class MessageReadClass) context.Context {
	if ctx == nil || class == "" {
		return ctx
	}
	return context.WithValue(ctx, messageReadClassContextKey{}, class)
}

func messageReadClassFromContext(ctx context.Context) MessageReadClass {
	if ctx == nil {
		return MessageReadClassInteractive
	}
	class, _ := ctx.Value(messageReadClassContextKey{}).(MessageReadClass)
	if class == MessageReadClassBackground {
		return MessageReadClassBackground
	}
	return MessageReadClassInteractive
}

type MessageReadIntent struct {
	ViewID string
	Class  MessageReadClass
}

type MessageReadResult struct {
	Body   *models.EmailBody
	Source string
}

type MessageCacheStore interface {
	GetMessageBodyByRef(models.MessageRef) (*models.EmailBody, error)
	PutMessageBodyByRef(models.MessageRef, *models.EmailBody, string) error
	GetPreviewBodyByRef(models.MessageRef) (*models.EmailBody, error)
	CachePreviewBodyByRef(models.MessageRef, *models.EmailBody, string) error
	InvalidateMessageByRef(models.MessageRef) error
	InvalidatePreviewByRef(models.MessageRef) error
}

type MessageSource interface {
	FetchMessageNoCache(context.Context, models.MessageRef) (*models.EmailBody, error)
	FetchMessagePreviewNoCache(context.Context, models.MessageRef) (*models.EmailBody, error)
}

type MessageServiceOptions struct {
	Cache         MessageCacheStore
	Source        MessageSource
	Coordinator   *work.Coordinator
	StoragePolicy func() string
}

type MessageService struct {
	cache               MessageCacheStore
	source              MessageSource
	coordinator         *work.Coordinator
	ownsCoordinator     bool
	storagePolicy       func() string
	unavailableBodyText string
}

func NewMessageService(opts MessageServiceOptions) *MessageService {
	coordinator := opts.Coordinator
	ownsCoordinator := false
	if coordinator == nil {
		coordinator = work.NewCoordinator()
		ownsCoordinator = true
	}
	storagePolicy := opts.StoragePolicy
	if storagePolicy == nil {
		storagePolicy = func() string { return config.CacheStoragePolicyNoAttachments }
	}
	return &MessageService{
		cache:           opts.Cache,
		source:          opts.Source,
		coordinator:     coordinator,
		ownsCoordinator: ownsCoordinator,
		storagePolicy:   storagePolicy,
		unavailableBodyText: "(Body unavailable: this cached email has no server UID yet, " +
			"so Herald cannot safely load its full contents. Re-sync the folder or use server search to refresh it.)",
	}
}

func (s *MessageService) Close() {
	if s != nil && s.ownsCoordinator && s.coordinator != nil {
		s.coordinator.Close()
	}
}

func (s *MessageService) GetMessage(ctx context.Context, ref models.MessageRef) (MessageReadResult, error) {
	if s == nil {
		return MessageReadResult{}, fmt.Errorf("message service is nil")
	}
	ref = ref.WithDefaults()
	if s.cache != nil {
		body, err := s.cache.GetMessageBodyByRef(ref)
		if err != nil {
			return MessageReadResult{}, err
		}
		if body != nil {
			return MessageReadResult{Body: cloneEmailBody(body), Source: MessageReadSourceCache}, nil
		}
	}
	if ref.UID == 0 {
		return MessageReadResult{Body: s.unavailableBody(), Source: MessageReadSourceUnavailable}, nil
	}
	return s.submitProviderRead(ctx, ref, MessageReadIntent{}, "message-body", func(runCtx context.Context) (MessageReadResult, error) {
		return s.GetMessageNoCache(runCtx, ref)
	})
}

func (s *MessageService) GetMessageNoCache(ctx context.Context, ref models.MessageRef) (MessageReadResult, error) {
	if s == nil {
		return MessageReadResult{}, fmt.Errorf("message service is nil")
	}
	ref = ref.WithDefaults()
	if ref.UID == 0 {
		return MessageReadResult{Body: s.unavailableBody(), Source: MessageReadSourceUnavailable}, nil
	}
	if s.source == nil {
		return MessageReadResult{}, fmt.Errorf("message source unavailable")
	}
	body, err := s.source.FetchMessageNoCache(ctx, ref)
	if err != nil {
		return MessageReadResult{Source: MessageReadSourceProvider}, err
	}
	body = normalizeReadBody(ref, body)
	if body != nil {
		if err := s.PutMessageBody(ref, body); err != nil {
			return MessageReadResult{}, err
		}
	}
	return MessageReadResult{Body: cloneEmailBody(body), Source: MessageReadSourceProvider}, nil
}

func (s *MessageService) GetMessagePreview(ctx context.Context, ref models.MessageRef, intent MessageReadIntent) (MessageReadResult, error) {
	if s == nil {
		return MessageReadResult{}, fmt.Errorf("message service is nil")
	}
	ref = ref.WithDefaults()
	if s.cache != nil {
		body, err := s.cache.GetPreviewBodyByRef(ref)
		if err != nil {
			return MessageReadResult{}, err
		}
		if body != nil {
			return MessageReadResult{Body: cloneEmailBody(body), Source: MessageReadSourceCache}, nil
		}
	}
	if ref.UID == 0 {
		return MessageReadResult{Body: s.unavailableBody(), Source: MessageReadSourceUnavailable}, nil
	}
	return s.submitProviderRead(ctx, ref, intent, "message-preview", func(runCtx context.Context) (MessageReadResult, error) {
		return s.GetMessagePreviewNoCache(runCtx, ref)
	})
}

func (s *MessageService) GetMessagePreviewNoCache(ctx context.Context, ref models.MessageRef) (MessageReadResult, error) {
	if s == nil {
		return MessageReadResult{}, fmt.Errorf("message service is nil")
	}
	ref = ref.WithDefaults()
	if ref.UID == 0 {
		return MessageReadResult{Body: s.unavailableBody(), Source: MessageReadSourceUnavailable}, nil
	}
	if s.source == nil {
		return MessageReadResult{}, fmt.Errorf("message source unavailable")
	}
	var (
		body *models.EmailBody
		err  error
	)
	if s.currentStoragePolicy() == config.CacheStoragePolicyPreserveAll {
		body, err = s.source.FetchMessageNoCache(ctx, ref)
	} else {
		body, err = s.source.FetchMessagePreviewNoCache(ctx, ref)
		if err == nil && body == nil {
			body, err = s.source.FetchMessageNoCache(ctx, ref)
		}
	}
	if err != nil {
		return MessageReadResult{Source: MessageReadSourceProvider}, err
	}
	body = normalizeReadBody(ref, body)
	if body != nil {
		if err := s.PutMessagePreview(ref, body); err != nil {
			return MessageReadResult{}, err
		}
	}
	return MessageReadResult{Body: cloneEmailBody(body), Source: MessageReadSourceProvider}, nil
}

func (s *MessageService) PutMessageBody(ref models.MessageRef, body *models.EmailBody) error {
	if s == nil || s.cache == nil || body == nil {
		return nil
	}
	return s.cache.PutMessageBodyByRef(ref.WithDefaults(), body, s.currentStoragePolicy())
}

func (s *MessageService) PutMessagePreview(ref models.MessageRef, body *models.EmailBody) error {
	if s == nil || s.cache == nil || body == nil {
		return nil
	}
	return s.cache.CachePreviewBodyByRef(ref.WithDefaults(), body, s.currentStoragePolicy())
}

func (s *MessageService) InvalidateMessage(ref models.MessageRef) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.InvalidateMessageByRef(ref.WithDefaults())
}

func (s *MessageService) InvalidatePreview(ref models.MessageRef) error {
	if s == nil || s.cache == nil {
		return nil
	}
	return s.cache.InvalidatePreviewByRef(ref.WithDefaults())
}

func (s *MessageService) submitProviderRead(ctx context.Context, ref models.MessageRef, intent MessageReadIntent, operation string, run func(context.Context) (MessageReadResult, error)) (MessageReadResult, error) {
	policy := work.PolicyCoalesceByResource | work.PolicyReplayCompletedResource
	priority := work.PriorityInteractive
	readClass := intent.Class
	if readClass == "" {
		readClass = messageReadClassFromContext(ctx)
	}
	if readClass == MessageReadClassBackground {
		policy |= work.PolicySerialBySource
		priority = work.PriorityBackground
	}
	intentKey := work.IntentKey{}
	if strings.TrimSpace(intent.ViewID) != "" && readClass != MessageReadClassBackground {
		policy |= work.PolicyTakeLatestByIntent
		intentKey = work.IntentKey{ViewID: intent.ViewID}
	}
	result := s.coordinator.Submit(ctx, work.Spec{
		SourceID:    work.SourceID(ref.SourceID),
		IntentKey:   intentKey,
		ResourceKey: s.resourceKey(ref, operation),
		Policy:      policy,
		Priority:    priority,
		Run: func(runCtx context.Context) (any, error) {
			runCtx = withMessageReadClass(runCtx, readClass)
			return run(runCtx)
		},
	})
	value, err := result.Await(ctx)
	if err != nil {
		return MessageReadResult{}, err
	}
	read, ok := value.(MessageReadResult)
	if !ok {
		return MessageReadResult{}, fmt.Errorf("message service returned %T", value)
	}
	if strings.TrimSpace(read.Source) == "" {
		read.Source = MessageReadSourceProvider
	}
	read.Body = cloneEmailBody(read.Body)
	return read, nil
}

func (s *MessageService) resourceKey(ref models.MessageRef, operation string) work.ResourceKey {
	ref = ref.WithDefaults()
	itemID := ref.LocalID
	if strings.TrimSpace(itemID) == "" {
		itemID = ref.MessageID
	}
	if strings.TrimSpace(itemID) == "" && ref.UID != 0 {
		itemID = fmt.Sprintf("uid:%d", ref.UID)
	}
	return work.ResourceKey{
		SourceID:     string(ref.SourceID),
		AccountID:    string(ref.AccountID),
		CollectionID: ref.Folder,
		ItemID:       itemID,
		Operation:    operation,
		Freshness:    fmt.Sprintf("uidvalidity:%d:policy:%s", ref.UIDValidity, s.currentStoragePolicy()),
	}
}

func (s *MessageService) currentStoragePolicy() string {
	if s == nil || s.storagePolicy == nil {
		return config.CacheStoragePolicyNoAttachments
	}
	return config.NormalizeCacheStoragePolicy(s.storagePolicy())
}

func (s *MessageService) unavailableBody() *models.EmailBody {
	text := ""
	if s != nil {
		text = s.unavailableBodyText
	}
	if strings.TrimSpace(text) == "" {
		text = "(Body unavailable.)"
	}
	return &models.EmailBody{TextPlain: text}
}

func normalizeReadBody(ref models.MessageRef, body *models.EmailBody) *models.EmailBody {
	if body == nil {
		return nil
	}
	if strings.TrimSpace(body.MessageID) == "" {
		body = cloneEmailBody(body)
		body.MessageID = ref.MessageID
	}
	return body
}

func cloneEmailBody(body *models.EmailBody) *models.EmailBody {
	if body == nil {
		return nil
	}
	cloned := *body
	if body.InlineImages != nil {
		cloned.InlineImages = make([]models.InlineImage, len(body.InlineImages))
		for i, image := range body.InlineImages {
			cloned.InlineImages[i] = image
			if image.Data != nil {
				cloned.InlineImages[i].Data = append([]byte(nil), image.Data...)
			}
		}
	}
	if body.Attachments != nil {
		cloned.Attachments = make([]models.Attachment, len(body.Attachments))
		for i, attachment := range body.Attachments {
			cloned.Attachments[i] = attachment
			if attachment.Data != nil {
				cloned.Attachments[i].Data = append([]byte(nil), attachment.Data...)
			}
		}
	}
	return &cloned
}
