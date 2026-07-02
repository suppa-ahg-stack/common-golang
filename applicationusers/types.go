// Package applicationusers contains the DTOs and HTTP client used to talk to
// orgs_backoffice's application-user API. It is intentionally small and only
// covers the read/write operations that are now used by more than one service.
package applicationusers

import "time"

// ApplicationUser is the public view of a user attached to an application.
type ApplicationUser struct {
	UserID    int64    `json:"user_id"`
	Email     string   `json:"email"`
	FullName  string   `json:"full_name"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Phone     string   `json:"phone"`
	Status    string   `json:"status"`
	IsActive  bool     `json:"is_active"`
	IsBlocked bool     `json:"is_blocked"`
	IsRoot    bool     `json:"is_root"`
	Roles     []string `json:"roles"`
}

// ApplicationUserFilter carries the supported filters for listing application
// users.
type ApplicationUserFilter struct {
	Search   string
	RoleName string
	Limit    int
	Offset   int
}

// CreateOrAttachApplicationUserRequest creates a new auth_app identity or
// attaches an existing one to the root organisation and grants the requested
// roles on the named application.
type CreateOrAttachApplicationUserRequest struct {
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

// UpdateApplicationUserRolesRequest replaces the user's roles on the named
// application with the supplied set.
type UpdateApplicationUserRolesRequest struct {
	Roles []string `json:"roles"`
}

// ApplicationUserListResponse returns a page of application users and the total
// count of matching users before pagination.
type ApplicationUserListResponse struct {
	Users []ApplicationUser `json:"users"`
	Total int               `json:"total"`
}

// ApplicationUserDetailResponse returns the full detail of a single application
// user. It embeds ApplicationUser because the fields are identical.
type ApplicationUserDetailResponse struct {
	ApplicationUser
}

// ToggleStateResponse returns the new boolean state after toggling is_active or
// is_blocked.
type ToggleStateResponse struct {
	NewState bool `json:"new_state"`
}

// PasswordResetResponse is returned by the password-reset invitation endpoint.
// Status is "queued". ResetLink is only populated in development to help debug
// reset flows without requiring email delivery.
type PasswordResetResponse struct {
	Status    string `json:"status"`
	ResetLink string `json:"reset_link,omitempty"`
}

// AuditLogEntry mirrors the audit log view-model exposed by orgs_backoffice.
type AuditLogEntry struct {
	ID         int64     `json:"id"`
	UserID     *int64    `json:"user_id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   *int64    `json:"entity_id"`
	OrgID      *int64    `json:"org_id"`
	Payload    string    `json:"payload"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
}

// ApplicationUserAuditResponse returns audit log entries for an application user.
type ApplicationUserAuditResponse struct {
	Entries []AuditLogEntry `json:"entries"`
}
