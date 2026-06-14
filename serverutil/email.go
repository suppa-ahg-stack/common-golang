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

func (e *EmailSender) SendOtpCode(to, code string) error {
	if e.dialer.Host == "" {
		return nil
	}
	m := mail.NewMessage()
	m.SetHeader("From", e.from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", "Your verification code")
	m.SetBody("text/plain", fmt.Sprintf(
		"Your verification code is: %s\n\nThis code expires in 5 minutes.\n\nIf you did not request this, ignore this email.",
		code,
	))
	return e.dialer.DialAndSend(m)
}
