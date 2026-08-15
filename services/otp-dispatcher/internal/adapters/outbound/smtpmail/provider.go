// Package smtpmail is an EmailProvider adapter that sends over SMTP. It is used for
// local development and end-to-end tests against MailHog (a fake SMTP inbox), whereas
// resendmail is used for production. Both satisfy the same app.EmailProvider port, so
// the composition root swaps them by config with no change to the domain or handler.
package smtpmail

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// Provider sends mail through an SMTP server (e.g. MailHog).
type Provider struct {
	addr string
	from string
	auth smtp.Auth // nil when the server needs no auth (MailHog)
}

// New builds an SMTP provider. Pass empty user/pass for an unauthenticated server.
func New(host string, port int, from, user, pass string) *Provider {
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	return &Provider{addr: fmt.Sprintf("%s:%d", host, port), from: from, auth: auth}
}

// Send delivers one plaintext email. SMTP returns no provider message id, so we return
// an empty string on success (the delivery log records status/latency either way).
func (p *Provider) Send(_ context.Context, to, subject, body string) (string, error) {
	if err := smtp.SendMail(p.addr, p.auth, p.from, []string{to}, buildMessage(p.from, to, subject, body)); err != nil {
		return "", fmt.Errorf("smtp: send: %w", err)
	}
	return "", nil
}

// buildMessage assembles RFC 5322 headers + body. Lines are CRLF-terminated as SMTP
// requires.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
