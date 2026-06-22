package serverutil

import (
	"errors"
	"testing"
	"time"
)

func TestNewEmailSender_ValidAuthenticated(t *testing.T) {
	sender, err := NewEmailSender("smtp.example.com", 587, "user", "pass", "sender@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !sender.IsConfigured() {
		t.Error("expected sender to be configured")
	}
}

func TestNewEmailSender_ValidUnauthenticated(t *testing.T) {
	sender, err := NewEmailSender("smtp.example.com", 25, "", "", "sender@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !sender.IsConfigured() {
		t.Error("expected sender to be configured")
	}
}

func TestNewEmailSender_AbsentConfigIsDisabled(t *testing.T) {
	sender, err := NewEmailSender("", 0, "", "", "")
	if err != nil {
		t.Fatalf("expected no error for absent config, got %v", err)
	}
	if sender.IsConfigured() {
		t.Error("expected sender to be disabled")
	}
}

func TestNewEmailSender_RejectsHostWithoutFrom(t *testing.T) {
	_, err := NewEmailSender("smtp.example.com", 587, "", "", "")
	if err == nil {
		t.Fatal("expected error when host is set but from is missing")
	}
}

func TestNewEmailSender_RejectsFromWithoutHost(t *testing.T) {
	_, err := NewEmailSender("", 0, "", "", "sender@example.com")
	if err == nil {
		t.Fatal("expected error when from is set but host is missing")
	}
}

func TestNewEmailSender_RejectsUserWithoutPassword(t *testing.T) {
	_, err := NewEmailSender("smtp.example.com", 587, "user", "", "sender@example.com")
	if err == nil {
		t.Fatal("expected error when user is set without password")
	}
}

func TestNewEmailSender_RejectsPasswordWithoutUser(t *testing.T) {
	_, err := NewEmailSender("smtp.example.com", 587, "", "pass", "sender@example.com")
	if err == nil {
		t.Fatal("expected error when password is set without user")
	}
}

func TestNewEmailSender_RejectsInvalidFrom(t *testing.T) {
	_, err := NewEmailSender("smtp.example.com", 587, "", "", "not-an-email")
	if err == nil {
		t.Fatal("expected error for invalid sender address")
	}
}

func TestNewEmailSender_RejectsDisplayNameFrom(t *testing.T) {
	_, err := NewEmailSender("smtp.example.com", 587, "", "", "Sender <sender@example.com>")
	if err == nil {
		t.Fatal("expected error when from contains a display name")
	}
}

func TestNewEmailSender_RejectsInvalidPort(t *testing.T) {
	cases := []int{0, -1, 70000, 65536}
	for _, port := range cases {
		_, err := NewEmailSender("smtp.example.com", port, "", "", "sender@example.com")
		if err == nil {
			t.Errorf("expected error for port %d", port)
		}
	}
}

func TestEmailSender_SetTimeout(t *testing.T) {
	sender, err := NewEmailSender("smtp.example.com", 587, "user", "pass", "sender@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	sender.SetTimeout(42 * time.Second)
	if got := sender.Timeout(); got != 42*time.Second {
		t.Errorf("expected timeout 42s, got %v", got)
	}

	sender.SetTimeout(0)
	if got := sender.Timeout(); got != 0 {
		t.Errorf("expected timeout 0, got %v", got)
	}
}

func TestEmailSender_SetTimeoutNoPanicOnNil(t *testing.T) {
	var sender *EmailSender
	sender.SetTimeout(10 * time.Second)
}

func TestEmailSender_SendMethodsReturnErrEmailNotConfiguredWhenDisabled(t *testing.T) {
	sender, err := NewEmailSender("", 0, "", "", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := sender.SendResetPassword("to@example.com", "http://link"); !errors.Is(err, ErrEmailNotConfigured) {
		t.Errorf("SendResetPassword: expected ErrEmailNotConfigured, got %v", err)
	}
	if err := sender.SendOtpCode("to@example.com", "123456", 300); !errors.Is(err, ErrEmailNotConfigured) {
		t.Errorf("SendOtpCode: expected ErrEmailNotConfigured, got %v", err)
	}
	if err := sender.SendPasswordSetup("to@example.com", "http://link", 24); !errors.Is(err, ErrEmailNotConfigured) {
		t.Errorf("SendPasswordSetup: expected ErrEmailNotConfigured, got %v", err)
	}
}
