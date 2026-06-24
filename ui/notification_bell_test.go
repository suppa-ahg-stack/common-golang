package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func renderNotificationBellToString(t *testing.T, ctx context.Context, cmp templ.Component) string {
	t.Helper()
	var buf strings.Builder
	if err := cmp.Render(ctx, &buf); err != nil {
		t.Fatalf("failed to render component: %v", err)
	}
	return buf.String()
}

func TestNotificationBell(t *testing.T) {
	ctx := context.Background()
	props := NotificationBellProps{
		ID:               "notification-bell",
		SummaryEndpoint:  "/notifications/summary",
		ListEndpoint:     "/notifications",
		MarkReadEndpoint: "/notifications/{id}/read",
		InitialPageSize:  20,
		FetchPageSize:    20,
		Title:            "Notifications",
		EmptyLabel:       "No notifications",
		LoadingLabel:     "Loading...",
		ErrorLabel:       "Error",
		RefreshLabel:     "Refresh",
		DemoButtonLabel:  "Send notification",
		AriaLabel:        "Notifications",
	}

	html := renderNotificationBellToString(t, ctx, NotificationBell(props))

	if !strings.Contains(html, `id="notification-bell"`) {
		t.Error("expected notification bell id")
	}
	if !strings.Contains(html, `x-data="notificationBell"`) {
		t.Error("expected x-data=notificationBell")
	}
	if !strings.Contains(html, `x-init="initFromDOM`) {
		t.Error("expected x-init call")
	}
	if !strings.Contains(html, `notification-bell`) || !strings.Contains(html, `notification-bell-dropdown`) {
		t.Error("expected x-init with DOM ids")
	}
	if !strings.Contains(html, `/notifications/summary`) {
		t.Error("expected summary endpoint")
	}
	if !strings.Contains(html, `data-initial-page-size="20"`) {
		t.Error("expected initial page size attribute")
	}
	if !strings.Contains(html, "Notifications") {
		t.Error("expected title text")
	}
	if !strings.Contains(html, `x-text="emptyLabel"`) {
		t.Error("expected empty label binding")
	}
	if !strings.Contains(html, `x-show="store.unreadCount > 0"`) {
		t.Error("expected data-bound unread count")
	}
	if !strings.Contains(html, `data-empty-label="No notifications"`) {
		t.Error("expected empty label data attribute")
	}
	if !strings.Contains(html, `data-refresh-label="Refresh"`) {
		t.Error("expected refresh label data attribute")
	}
}
