package notifications

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"suppa-ahg-stack/common-golang/logger"
)

// Publisher sends notification events to clients.
// In auth_app this is implemented with the SSE broker keyed by user ID.
type Publisher interface {
	PublishNotificationCreated(ctx context.Context, userID string, event NotificationCreatedEvent) error
	PublishNotificationRead(ctx context.Context, userID string, event NotificationReadEvent) error
}

// Service is the application service for notifications.
type Service struct {
	store     Store
	publisher Publisher
	l         *logger.FileLogger
}

// NewService creates a notification service.
func NewService(store Store, publisher Publisher, l *logger.FileLogger) *Service {
	return &Service{
		store:     store,
		publisher: publisher,
		l:         l,
	}
}

// Create creates a notification for a user and publishes a notification-created event
// to the session identified by sessionID.
func (s *Service) Create(ctx context.Context, sessionID string, input CreateNotificationInput) (Notification, error) {
	n, err := s.store.Create(ctx, input)
	if err != nil {
		return Notification{}, fmt.Errorf("create notification: %w", err)
	}

	unread, err := s.store.UnreadCount(ctx, input.UserID)
	if err != nil {
		s.l.Error(fmt.Sprintf("failed to compute unread count after create: %v", err))
		unread = 1
	}

	event := NotificationCreatedEvent{
		ID:          n.ID,
		CreatedAt:   n.CreatedAt.Format(time.RFC3339Nano),
		Title:       n.Title,
		Body:        n.Body,
		UnreadCount: unread,
	}

	if s.publisher != nil {
		if err := s.publisher.PublishNotificationCreated(ctx, strconv.FormatInt(input.UserID, 10), event); err != nil {
			s.l.Error(fmt.Sprintf("failed to publish notification-created: %v", err))
		}
	}

	return n, nil
}

// List returns paginated notifications for a user.
func (s *Service) List(ctx context.Context, userID int64, opts ListOptions) (NotificationPage, error) {
	items, err := s.store.List(ctx, userID, opts)
	if err != nil {
		return NotificationPage{}, fmt.Errorf("list notifications: %w", err)
	}

	page := NotificationPage{
		Items: items,
	}

	if len(items) > 0 && opts.Limit > 0 && len(items) == opts.Limit {
		last := items[len(items)-1]
		page.NextCursor = EncodeCursor(last.CreatedAt, last.ID)
		page.HasMore = true
	}

	return page, nil
}

// UnreadCount returns the unread notification count for a user.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (NotificationSummary, error) {
	count, err := s.store.UnreadCount(ctx, userID)
	if err != nil {
		return NotificationSummary{}, fmt.Errorf("unread count: %w", err)
	}
	return NotificationSummary{UnreadCount: count}, nil
}

// MarkRead marks a notification as read for a user and broadcasts the update.
func (s *Service) MarkRead(ctx context.Context, id int64, userID int64) error {
	if err := s.store.MarkRead(ctx, id, userID); err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	s.broadcastRead(ctx, []int64{id}, userID)
	return nil
}

// MarkReadMany marks multiple notifications as read for a user and broadcasts the update.
func (s *Service) MarkReadMany(ctx context.Context, ids []int64, userID int64) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.store.MarkReadMany(ctx, ids, userID); err != nil {
		return fmt.Errorf("mark read many: %w", err)
	}
	s.broadcastRead(ctx, ids, userID)
	return nil
}

func (s *Service) broadcastRead(ctx context.Context, ids []int64, userID int64) {
	unread, err := s.store.UnreadCount(ctx, userID)
	if err != nil {
		s.l.Error(fmt.Sprintf("failed to compute unread count after mark read, skipping SSE broadcast: %v", err))
		return
	}

	event := NotificationReadEvent{
		IDs:         ids,
		UnreadCount: unread,
	}

	if s.publisher != nil {
		if err := s.publisher.PublishNotificationRead(ctx, strconv.FormatInt(userID, 10), event); err != nil {
			s.l.Error(fmt.Sprintf("failed to publish notification-read: %v", err))
		}
	}
}
