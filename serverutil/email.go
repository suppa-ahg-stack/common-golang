package serverutil

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	gomail "github.com/go-mail/mail/v2"
)

// ErrEmailNotConfigured is returned by EmailSender when SMTP is not configured
// and a send is attempted. Callers can detect it with errors.Is to decide
// whether to treat the failure as a configuration problem.
var ErrEmailNotConfigured = errors.New("email sender not configured")

var (
	errSMTPHostFromMismatch   = errors.New("SMTP host and sender must both be configured or both omitted")
	errSMTPPortInvalid        = errors.New("SMTP port must be between 1 and 65535")
	errSMTPFromInvalid        = errors.New("SMTP_FROM must be a valid email address")
	errSMTPCredentialsPartial = errors.New("SMTP_USER and SMTP_PASSWORD must both be supplied or both omitted")
)

type EmailSender struct {
	dialer *gomail.Dialer
	from   string
}

// NewEmailSender builds an EmailSender after validating the SMTP configuration.
//
// Validation rules:
//   - host and from must either both be configured or both be absent.
//   - port must be between 1 and 65535 when host is configured.
//   - from must be a valid email address when configured.
//   - user and password must either both be supplied or both be omitted.
//
// When all fields are absent the returned sender is disabled: every Send*
// method returns ErrEmailNotConfigured. A disabled sender is a valid, non-nil
// value, so callers do not need nil guards.
func NewEmailSender(host string, port int, user, password, from string) (*EmailSender, error) {
	hostSet := strings.TrimSpace(host) != ""
	fromSet := strings.TrimSpace(from) != ""
	userSet := strings.TrimSpace(user) != ""
	passSet := strings.TrimSpace(password) != ""

	if !hostSet && !fromSet && !userSet && !passSet {
		d := gomail.NewDialer("", 0, "", "")
		d.TLSConfig = &tls.Config{}
		return &EmailSender{dialer: d, from: ""}, nil
	}

	if hostSet != fromSet {
		return nil, errSMTPHostFromMismatch
	}

	if port < 1 || port > 65535 {
		return nil, errSMTPPortInvalid
	}

	if addr, err := mail.ParseAddress(from); err != nil || strings.TrimSpace(from) != addr.Address {
		return nil, errSMTPFromInvalid
	}

	if userSet != passSet {
		return nil, errSMTPCredentialsPartial
	}

	d := gomail.NewDialer(host, port, user, password)
	d.TLSConfig = &tls.Config{ServerName: host}
	return &EmailSender{
		dialer: d,
		from:   from,
	}, nil
}

// IsConfigured reports whether the sender has a usable SMTP configuration.
func (e *EmailSender) IsConfigured() bool {
	return e != nil && e.dialer != nil && e.dialer.Host != ""
}

// SetTimeout configures the maximum duration allowed for a single SMTP
// connection. A zero timeout disables the timeout.
func (e *EmailSender) SetTimeout(timeout time.Duration) {
	if e == nil || e.dialer == nil {
		return
	}
	e.dialer.Timeout = timeout
}

// Timeout returns the configured SMTP timeout. It returns zero when the sender
// is nil or its underlying dialer is not initialized.
func (e *EmailSender) Timeout() time.Duration {
	if e == nil || e.dialer == nil {
		return 0
	}
	return e.dialer.Timeout
}

func (e *EmailSender) SendResetPassword(to, resetLink string) error {
	if !e.IsConfigured() {
		return ErrEmailNotConfigured
	}
	m := gomail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", "Reset your password")
	m.SetBody("text/plain", fmt.Sprintf(
		"Click the link below to reset your password:\n\n%s\n\nThis link expires in 1 hour.\n\nIf you did not request this, ignore this email.",
		resetLink,
	))
	return e.dialer.DialAndSend(m)
}

func (e *EmailSender) SendOtpCode(to, code string, ttlSeconds int) error {
	if !e.IsConfigured() {
		return ErrEmailNotConfigured
	}
	m := gomail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", "Your verification code")
	m.SetBody("text/plain", fmt.Sprintf(
		"Your verification code is: %s\n\nThis code expires in %s.\n\nIf you did not request this, ignore this email.",
		code,
		formatTTL(ttlSeconds),
	))
	return e.dialer.DialAndSend(m)
}

// SendPasswordSetup delivers a single-use password-setup link. The raw token
// and link are never logged.
func (e *EmailSender) SendPasswordSetup(to, setupLink string, ttlHours int) error {
	if !e.IsConfigured() {
		return ErrEmailNotConfigured
	}
	m := gomail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", "Set up your account")
	m.SetBody("text/plain", fmt.Sprintf(
		"Click the link below to set up your account password:\n\n%s\n\nThis link expires in %d hour(s) and can only be used once.\n\nIf you did not request this, ignore this email.",
		setupLink,
		ttlHours,
	))
	return e.dialer.DialAndSend(m)
}

func formatTTL(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	minutes := seconds / 60
	remainingSeconds := seconds % 60
	if remainingSeconds == 0 {
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	return fmt.Sprintf("%d minutes %d seconds", minutes, remainingSeconds)
}
