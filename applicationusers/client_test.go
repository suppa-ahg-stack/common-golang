package applicationusers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	cinternalapi "suppa-ahg-stack/common-golang/serverutil/internalapi"
)

func TestClientGetRolesForUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"roles": []string{"admin"}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	roles, err := client.GetRolesForUser(context.Background(), 42, "Planning")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 1 || roles[0] != "admin" {
		t.Errorf("unexpected roles: %v", roles)
	}
}

func TestClientGetRolesForUserNotAttached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	roles, err := client.GetRolesForUser(context.Background(), 42, "Planning")
	if err != nil {
		t.Fatalf("expected nil error for detached user, got %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("expected empty roles, got %v", roles)
	}
}

func TestClientListApplicationUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		q := r.URL.Query()
		if q.Get("search") != "foo" {
			t.Errorf("unexpected search: %s", q.Get("search"))
		}
		if q.Get("role_name") != "admin" {
			t.Errorf("unexpected role_name: %s", q.Get("role_name"))
		}
		if q.Get("limit") != "20" {
			t.Errorf("unexpected limit: %s", q.Get("limit"))
		}
		if q.Get("offset") != "10" {
			t.Errorf("unexpected offset: %s", q.Get("offset"))
		}
		_ = json.NewEncoder(w).Encode(ApplicationUserListResponse{
			Users: []ApplicationUser{{UserID: 1, Email: "a@example.com", Roles: []string{"admin"}}},
			Total: 1,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	resp, err := client.ListApplicationUsers(context.Background(), "Planning", ApplicationUserFilter{
		Search:   "foo",
		RoleName: "admin",
		Limit:    20,
		Offset:   10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
	if len(resp.Users) != 1 || resp.Users[0].UserID != 1 {
		t.Errorf("unexpected users: %+v", resp.Users)
	}
}

func TestClientListApplicationUsersDefaultPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("limit") != "" || q.Get("offset") != "" {
			t.Errorf("expected empty limit/offset, got limit=%q offset=%q", q.Get("limit"), q.Get("offset"))
		}
		_ = json.NewEncoder(w).Encode(ApplicationUserListResponse{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	_, err := client.ListApplicationUsers(context.Background(), "Planning", ApplicationUserFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientGetApplicationUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ApplicationUserDetailResponse{
			ApplicationUser: ApplicationUser{UserID: 42, Email: "a@example.com", Roles: []string{"admin"}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	resp, err := client.GetApplicationUser(context.Background(), "Planning", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UserID != 42 {
		t.Errorf("unexpected user id: %d", resp.UserID)
	}
}

func TestClientCreateOrAttachApplicationUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		var body CreateOrAttachApplicationUserRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Email != "new@example.com" {
			t.Errorf("unexpected email: %s", body.Email)
		}
		if len(body.Roles) != 1 || body.Roles[0] != "admin" {
			t.Errorf("unexpected roles: %v", body.Roles)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(ApplicationUserDetailResponse{
			ApplicationUser: ApplicationUser{UserID: 7, Email: body.Email, Roles: body.Roles},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	resp, err := client.CreateOrAttachApplicationUser(context.Background(), "Planning", "new@example.com", []string{"admin"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UserID != 7 {
		t.Errorf("unexpected user id: %d", resp.UserID)
	}
}

func TestClientUpdateApplicationUserRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users/42/roles" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
		}
		var body UpdateApplicationUserRolesRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Roles) != 1 || body.Roles[0] != "formation_manager" {
			t.Errorf("unexpected roles: %v", body.Roles)
		}
		_ = json.NewEncoder(w).Encode(ApplicationUserDetailResponse{
			ApplicationUser: ApplicationUser{UserID: 42, Email: "a@example.com", Roles: body.Roles},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	resp, err := client.UpdateApplicationUserRoles(context.Background(), "Planning", 42, []string{"formation_manager"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Roles) != 1 || resp.Roles[0] != "formation_manager" {
		t.Errorf("unexpected roles: %v", resp.Roles)
	}
}

func TestClientToggleApplicationUserActive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users/42/toggle-active" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ToggleStateResponse{NewState: true})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	state, err := client.ToggleApplicationUserActive(context.Background(), "Planning", 42, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !state {
		t.Errorf("expected true, got %v", state)
	}
}

func TestClientSendApplicationUserPasswordReset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users/42/password-reset" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(PasswordResetResponse{Status: "queued"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	resp, err := client.SendApplicationUserPasswordReset(context.Background(), "Planning", 42, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "queued" {
		t.Errorf("unexpected status: %s", resp.Status)
	}
}

func TestClientGetApplicationUserAudit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/applications/Planning/users/42/audit" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		uid := int64(42)
		_ = json.NewEncoder(w).Encode(ApplicationUserAuditResponse{
			Entries: []AuditLogEntry{
				{ID: 1, UserID: &uid, Action: "user:update", EntityType: "user", CreatedAt: time.Now().UTC()},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	resp, err := client.GetApplicationUserAudit(context.Background(), "Planning", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(resp.Entries))
	}
}

func TestClientGetApplicationUserNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	_, err := client.GetApplicationUser(context.Background(), "Planning", 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestClientListApplicationUsersUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "down"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	_, err := client.ListApplicationUsers(context.Background(), "Planning", ApplicationUserFilter{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestClientUpdateApplicationUserRolesClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid roles"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	_, err := client.UpdateApplicationUserRoles(context.Background(), "Planning", 42, []string{"bad_role"}, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	var respErr *cinternalapi.ResponseError
	if !errors.As(err, &respErr) {
		t.Fatalf("expected wrapped ResponseError, got %T", err)
	}
	if respErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", respErr.StatusCode)
	}
}

func TestClientPathEscaping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/internal/v1/applications/Planning%20App/users/42"
		if r.URL.Path != expected && r.URL.EscapedPath() != expected {
			t.Errorf("unexpected path: %s (escaped %s)", r.URL.Path, r.URL.EscapedPath())
		}
		_ = json.NewEncoder(w).Encode(ApplicationUserDetailResponse{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	_, err := client.GetApplicationUser(context.Background(), "Planning App", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientUserIDInPath(t *testing.T) {
	userID := int64(123)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/internal/v1/applications/Planning/users/" + strconv.FormatInt(userID, 10) + "/roles"
		if r.URL.Path != expected {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ApplicationUserDetailResponse{})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	_, err := client.UpdateApplicationUserRoles(context.Background(), "Planning", userID, []string{"admin"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "cfm_planning")
	healthy, _, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !healthy {
		t.Error("expected healthy")
	}
}
