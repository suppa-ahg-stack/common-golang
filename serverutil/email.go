package serverutil

import (
	"crypto/tls"
	"fmt"

	"github.com/go-mail/mail/v2"
)

type EmailSender struct {
	dialer *mail.Dialer
	from   string
}

func NewEmailSender(host string, port int, user, password, from string) *EmailSender {
	d := mail.NewDialer(host, port, user, password)
	d.TLSConfig = &tls.Config{ServerName: host}
	return &EmailSender{
		dialer: d,
		from:   from,
	}
}

func (e *EmailSender) SendResetPassword(to, resetLink string) error {
	if e.dialer.Host == "" {
		return nil
	}
	m := mail.NewMessage()
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
	if e.dialer.Host == "" {
		return nil
	}
	m := mail.NewMessage()
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
