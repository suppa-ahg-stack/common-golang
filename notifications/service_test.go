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
	createFunc       func(ctx context.Context, input CreateNotificationInput) (Notification, error)
	unreadCountFunc  func(ctx context.Context, userID int64) (int64, error)
	listFunc         func(ctx context.Context, userID int64, opts ListOptions) ([]Notification, error)
	markReadFunc     func(ctx context.Context, id int64, userID int64) ([]int64, error)
	markReadManyFunc func(ctx context.Context, ids []int64, userID int64) ([]int64, error)
	markAllReadFunc  func(ctx context.Context, userID int64) ([]int64, error)
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

func (f *fakeStore) MarkRead(ctx context.Context, id int64, userID int64) ([]int64, error) {
	return f.markReadFunc(ctx, id, userID)
}

func (f *fakeStore) MarkReadMany(ctx context.Context, ids []int64, userID int64) ([]int64, error) {
	if f.markReadManyFunc != nil {
		return f.markReadManyFunc(ctx, ids, userID)
	}
	return nil, nil
}

func (f *fakeStore) MarkAllRead(ctx context.Context, userID int64) ([]int64, error) {
	if f.markAllReadFunc != nil {
		return f.markAllReadFunc(ctx, userID)
	}
	return nil, nil
}

type spyPublisher struct {
	createdEvents []struct {
		userID string
		event  NotificationCreatedEvent
	}
	readEvents []struct {
		userID string
		event  NotificationReadEvent
	}
}

func (p *spyPublisher) PublishNotificationCreated(ctx context.Context, userID string, event NotificationCreatedEvent) error {
	p.createdEvents = append(p.createdEvents, struct {
		userID string
		event  NotificationCreatedEvent
	}{userID: userID, event: event})
	return nil
}

func (p *spyPublisher) PublishNotificationRead(ctx context.Context, userID string, event NotificationReadEvent) error {
	p.readEvents = append(p.readEvents, struct {
		userID string
		event  NotificationReadEvent
	}{userID: userID, event: event})
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
	if len(pub.createdEvents) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.createdEvents))
	}
	if pub.createdEvents[0].userID != "42" {
		t.Fatalf("expected userID 42, got %s", pub.createdEvents[0].userID)
	}
	if pub.createdEvents[0].event.UnreadCount != 5 {
		t.Fatalf("expected unread count 5, got %d", pub.createdEvents[0].event.UnreadCount)
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
		markReadFunc: func(_ context.Context, id int64, userID int64) ([]int64, error) {
			called = true
			if id != 9 || userID != 42 {
				t.Fatalf("unexpected args: id=%d userID=%d", id, userID)
			}
			return []int64{9}, nil
		},
		unreadCountFunc: func(_ context.Context, userID int64) (int64, error) {
			return 3, nil
		},
	}
	pub := &spyPublisher{}
	svc := NewService(store, pub, testLogger())
	if err := svc.MarkRead(ctx, 9, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected markRead to be called")
	}
	if len(pub.readEvents) != 1 {
		t.Fatalf("expected 1 read event, got %d", len(pub.readEvents))
	}
	if pub.readEvents[0].userID != "42" {
		t.Fatalf("expected userID 42, got %s", pub.readEvents[0].userID)
	}
	if len(pub.readEvents[0].event.IDs) != 1 || pub.readEvents[0].event.IDs[0] != 9 {
		t.Fatalf("unexpected event ids: %v", pub.readEvents[0].event.IDs)
	}
	if pub.readEvents[0].event.UnreadCount != 3 {
		t.Fatalf("expected unread count 3, got %d", pub.readEvents[0].event.UnreadCount)
	}
}

func TestServiceMarkReadAlreadyReadDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{
		markReadFunc: func(_ context.Context, id int64, userID int64) ([]int64, error) {
			return nil, nil
		},
	}
	pub := &spyPublisher{}
	svc := NewService(store, pub, testLogger())
	if err := svc.MarkRead(ctx, 9, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.readEvents) != 0 {
		t.Fatalf("expected 0 read events, got %d", len(pub.readEvents))
	}
}

func TestServiceMarkReadMany(t *testing.T) {
	ctx := context.Background()
	called := false
	store := &fakeStore{
		markReadManyFunc: func(_ context.Context, ids []int64, userID int64) ([]int64, error) {
			called = true
			if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 || userID != 42 {
				t.Fatalf("unexpected args: ids=%v userID=%d", ids, userID)
			}
			return []int64{1}, nil
		},
		unreadCountFunc: func(_ context.Context, userID int64) (int64, error) {
			return 0, nil
		},
	}
	pub := &spyPublisher{}
	svc := NewService(store, pub, testLogger())
	if err := svc.MarkReadMany(ctx, []int64{1, 2}, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected markReadMany to be called")
	}
	if len(pub.readEvents) != 1 {
		t.Fatalf("expected 1 read event, got %d", len(pub.readEvents))
	}
	if pub.readEvents[0].userID != "42" {
		t.Fatalf("expected userID 42, got %s", pub.readEvents[0].userID)
	}
	if len(pub.readEvents[0].event.IDs) != 1 || pub.readEvents[0].event.IDs[0] != 1 {
		t.Fatalf("unexpected event ids: %v", pub.readEvents[0].event.IDs)
	}
}

func TestServiceMarkReadManyEmpty(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	pub := &spyPublisher{}
	svc := NewService(store, pub, testLogger())
	if err := svc.MarkReadMany(ctx, []int64{}, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.readEvents) != 0 {
		t.Fatalf("expected 0 read events, got %d", len(pub.readEvents))
	}
}

func TestServiceMarkAllRead(t *testing.T) {
	ctx := context.Background()
	called := false
	store := &fakeStore{
		markAllReadFunc: func(_ context.Context, userID int64) ([]int64, error) {
			called = true
			if userID != 42 {
				t.Fatalf("expected userID 42, got %d", userID)
			}
			return []int64{1, 2}, nil
		},
		unreadCountFunc: func(_ context.Context, userID int64) (int64, error) {
			return 0, nil
		},
	}
	pub := &spyPublisher{}
	svc := NewService(store, pub, testLogger())
	if err := svc.MarkAllRead(ctx, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected markAllRead to be called")
	}
	if len(pub.readEvents) != 1 {
		t.Fatalf("expected 1 read event, got %d", len(pub.readEvents))
	}
	if pub.readEvents[0].userID != "42" {
		t.Fatalf("expected userID 42, got %s", pub.readEvents[0].userID)
	}
	if len(pub.readEvents[0].event.IDs) != 2 || pub.readEvents[0].event.IDs[0] != 1 || pub.readEvents[0].event.IDs[1] != 2 {
		t.Fatalf("unexpected event ids: %v", pub.readEvents[0].event.IDs)
	}
}
