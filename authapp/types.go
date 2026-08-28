// Package authapp contains the stable wire contract and client for auth_app's
// internal API. Consumer applications should depend on this package instead of
// maintaining private copies of the JSON DTOs.
package authapp

import "time"

type SessionValidateRequest struct {
	SessionToken string `json:"session_token"`
	RefreshToken string `json:"refresh_token"`
}

type Identity struct {
	ID                 int64  `json:"id"`
	Email              string `json:"email"`
	FullName           string `json:"full_name"`
	Status             string `json:"status"`
	PasswordMustChange bool   `json:"password_must_change"`
}

type SessionValidateResponse struct {
	User                      *Identity `json:"user"`
	SessionToken              string    `json:"session_token,omitempty"`
	RefreshToken              string    `json:"refresh_token,omitempty"`
	SessionDomain             string    `json:"session_domain"`
	SessionMaxAgeSeconds      int       `json:"session_max_age_seconds"`
	RefreshTokenMaxAgeSeconds int       `json:"refresh_token_max_age_seconds"`
}

type CookiePolicy struct {
	Domain                    string
	SessionMaxAgeSeconds      int
	RefreshTokenMaxAgeSeconds int
}

func (r SessionValidateResponse) CookiePolicy() CookiePolicy {
	return CookiePolicy{
		Domain:                    r.SessionDomain,
		SessionMaxAgeSeconds:      r.SessionMaxAgeSeconds,
		RefreshTokenMaxAgeSeconds: r.RefreshTokenMaxAgeSeconds,
	}
}

type UserProfileResponse struct {
	FirstName       *string    `json:"first_name"`
	LastName        *string    `json:"last_name"`
	Phone           *string    `json:"phone"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at"`
}

type UserResponse struct {
	ID                 int64               `json:"id"`
	Email              string              `json:"email"`
	FullName           string              `json:"full_name"`
	Status             string              `json:"status"`
	PasswordMustChange bool                `json:"password_must_change"`
	Profile            UserProfileResponse `json:"profile"`
}

type ListUsersResponse struct {
	Users []UserResponse `json:"users"`
	Next  bool           `json:"next"`
}

type BatchGetUsersRequest struct {
	IDs    []int64  `json:"ids"`
	Emails []string `json:"emails"`
}

type UserProfileRequest struct {
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Phone     *string `json:"phone"`
}

type CreateUserRequest struct {
	Email          string              `json:"email"`
	FullName       string              `json:"full_name"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	Profile        *UserProfileRequest `json:"profile,omitempty"`
}

type CreateUserResponse struct {
	ID       int64  `json:"id"`
	Created  bool   `json:"created"`
	Status   string `json:"status"`
	Replayed bool   `json:"replayed"`
}

type CreatePasswordSetupInvitationResponse struct {
	Status    string `json:"status"`
	SetupLink string `json:"setup_link,omitempty"`
}

type CreatePasswordResetInvitationResponse struct {
	Status    string `json:"status"`
	ResetLink string `json:"reset_link,omitempty"`
}

type UpdateUserRequest struct {
	Email              *string             `json:"email,omitempty"`
	FullName           *string             `json:"full_name,omitempty"`
	PasswordMustChange *bool               `json:"password_must_change,omitempty"`
	IdempotencyKey     string              `json:"idempotency_key"`
	Profile            *UserProfileRequest `json:"profile,omitempty"`
}
