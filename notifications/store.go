package notifications

import "context"

// Store abstracts notification persistence. Apps provide their own sqlc-backed implementation.
type Store interface {
	// Create persists a new notification and returns it.
	Create(ctx context.Context, input CreateNotificationInput) (Notification, error)

	// UnreadCount returns the number of unread notifications for a user.
	UnreadCount(ctx context.Context, userID int64) (int64, error)

	// List returns notifications for a user, oldest first within the page, with cursor-based pagination.
	List(ctx context.Context, userID int64, opts ListOptions) ([]Notification, error)

	// MarkRead marks a single notification as read for a user. It is idempotent.
	MarkRead(ctx context.Context, id int64, userID int64) error

	// MarkReadMany marks several notifications as read for a user. It is idempotent.
	MarkReadMany(ctx context.Context, ids []int64, userID int64) error
}
