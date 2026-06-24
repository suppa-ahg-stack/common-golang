package notifications

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"suppa-ahg-stack/common-golang/logger"
)

type fakeStore struct {
	createFunc        func(ctx context.Context, input CreateNotificationInput) (Notification, error)
	unreadCountFunc   func(ctx context.Context, userID int64) (int64, error)
	listFunc          func(ctx context.Context, userID int64, opts ListOptions) ([]Notification, error)
	markReadFunc      func(ctx context.Context, id int64, userID int64) error
	markReadManyFunc  func(ctx context.Context, ids []int64, userID int64) error
}

func (f *fakeStore) Create(ctx context.Context, input CreateNotificationInput) (Notification, error) {
	return f.createFunc(ctx, input)
}

func (f *fakeStore) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	return f.unreadCountFunc(ctx, userID)
}

func (f *fakeStore) List(ctx context.Context, userID int64, opts ListOptions) ([]Notification, error) {
	return f.listFunc(ctx, userID, opts)
}

func (f *fakeStore) MarkRead(ctx context.Context, id int64, userID int64) error {
	return f.markReadFunc(ctx, id, userID)
}

func (f *fakeStore) MarkReadMany(ctx context.Context, ids []int64, userID int64) error {
	if f.markReadManyFunc != nil {
		return f.markReadManyFunc(ctx, ids, userID)
	}
	return nil
}

type spyPublisher struct {
	published []struct {
		sessionID string
		event     NotificationCreatedEvent
	}
}

func (p *spyPublisher) PublishNotificationCreated(ctx context.Context, sessionID string, event NotificationCreatedEvent) error {
	p.published = append(p.published, struct {
		sessionID string
		event     NotificationCreatedEvent
	}{sessionID: sessionID, event: event})
	return nil
}

func testLogger() *logger.FileLogger {
	return &logger.FileLogger{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestServiceCreate(t *testing.T) {
	ctx := context.Background()
	created := Notification{ID: 1, UserID: 42, Title: "Hello", Body: "World", CreatedAt: time.Now()}

	store := &fakeStore{
		createFunc: func(_ context.Context, input CreateNotificationInput) (Notification, error) {
			return created, nil
		},
		unreadCountFunc: func(_ context.Context, userID int64) (int64, error) {
			return 5, nil
		},
	}
	pub := &spyPublisher{}
	svc := NewService(store, pub, testLogger())

	input := CreateNotificationInput{UserID: 42, Title: "Hello", Body: "World"}
	n, err := svc.Create(ctx, "session-token", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.ID != created.ID {
		t.Fatalf("expected id %d, got %d", created.ID, n.ID)
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.published))
	}
	if pub.published[0].sessionID != "session-token" {
		t.Fatalf("expected sessionID session-token, got %s", pub.published[0].sessionID)
	}
	if pub.published[0].event.UnreadCount != 5 {
		t.Fatalf("expected unread count 5, got %d", pub.published[0].event.UnreadCount)
	}
}

func TestServiceCreateStoreError(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{
		createFunc: func(_ context.Context, input CreateNotificationInput) (Notification, error) {
			return Notification{}, errors.New("db down")
		},
	}
	svc := NewService(store, &NoopPublisher{}, testLogger())
	_, err := svc.Create(ctx, "session", CreateNotificationInput{UserID: 1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceListPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	items := []Notification{
		{ID: 1, UserID: 42, Title: "A", CreatedAt: now.Add(-1 * time.Hour)},
		{ID: 2, UserID: 42, Title: "B", CreatedAt: now},
	}

	store := &fakeStore{
		listFunc: func(_ context.Context, userID int64, opts ListOptions) ([]Notification, error) {
			return items, nil
		},
	}
	svc := NewService(store, &NoopPublisher{}, testLogger())

	page, err := svc.List(ctx, 42, ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page.Items))
	}
	if !page.HasMore {
		t.Fatal("expected hasMore true")
	}
	if page.NextCursor == "" {
		t.Fatal("expected non-empty next cursor")
	}

	_, _, err = DecodeCursor(page.NextCursor)
	if err != nil {
		t.Fatalf("cursor decode failed: %v", err)
	}
}

func TestServiceListNoMore(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{
		listFunc: func(_ context.Context, userID int64, opts ListOptions) ([]Notification, error) {
			return []Notification{{ID: 1, UserID: 42, Title: "A"}}, nil
		},
	}
	svc := NewService(store, &NoopPublisher{}, testLogger())

	page, err := svc.List(ctx, 42, ListOptions{Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.HasMore {
		t.Fatal("expected hasMore false")
	}
	if page.NextCursor != "" {
		t.Fatal("expected empty next cursor")
	}
}

func TestServiceUnreadCount(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{
		unreadCountFunc: func(_ context.Context, userID int64) (int64, error) {
			return 7, nil
		},
	}
	svc := NewService(store, &NoopPublisher{}, testLogger())
	summary, err := svc.UnreadCount(ctx, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.UnreadCount != 7 {
		t.Fatalf("expected 7, got %d", summary.UnreadCount)
	}
}

func TestServiceMarkRead(t *testing.T) {
	ctx := context.Background()
	called := false
	store := &fakeStore{
		markReadFunc: func(_ context.Context, id int64, userID int64) error {
			called = true
			if id != 9 || userID != 42 {
				t.Fatalf("unexpected args: id=%d userID=%d", id, userID)
			}
			return nil
		},
	}
	svc := NewService(store, &NoopPublisher{}, testLogger())
	if err := svc.MarkRead(ctx, 9, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected markRead to be called")
	}
}

func TestServiceMarkReadMany(t *testing.T) {
	ctx := context.Background()
	called := false
	store := &fakeStore{
		markReadManyFunc: func(_ context.Context, ids []int64, userID int64) error {
			called = true
			if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 || userID != 42 {
				t.Fatalf("unexpected args: ids=%v userID=%d", ids, userID)
			}
			return nil
		},
	}
	svc := NewService(store, &NoopPublisher{}, testLogger())
	if err := svc.MarkReadMany(ctx, []int64{1, 2}, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected markReadMany to be called")
	}
}

func TestServiceMarkReadManyEmpty(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	svc := NewService(store, &NoopPublisher{}, testLogger())
	if err := svc.MarkReadMany(ctx, []int64{}, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
