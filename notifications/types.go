package notifications

import "time"

// Notification represents a single user notification.
type Notification struct {
	ID        int64          `json:"id"`
	UserID    int64          `json:"user_id"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload"`
	ReadAt    *time.Time     `json:"read_at"`
	CreatedAt time.Time      `json:"created_at"`
}

// NotificationPage is a paginated list of notifications.
type NotificationPage struct {
	Items      []Notification `json:"Items"`
	NextCursor string         `json:"NextCursor"`
	HasMore    bool           `json:"HasMore"`
}

// NotificationSummary is the unread count returned to clients.
type NotificationSummary struct {
	UnreadCount int64 `json:"unread_count"`
}

// CreateNotificationInput is the data required to create a notification.
type CreateNotificationInput struct {
	UserID  int64
	Title   string
	Body    string
	Kind    string
	Payload map[string]any
}

// ListOptions controls pagination for listing notifications.
type ListOptions struct {
	Limit  int
	Cursor string
}

// NotificationCreatedEvent is the SSE payload sent when a new notification is created.
type NotificationCreatedEvent struct {
	ID          int64  `json:"id"`
	CreatedAt   string `json:"created_at"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	UnreadCount int64  `json:"unread_count"`
}
