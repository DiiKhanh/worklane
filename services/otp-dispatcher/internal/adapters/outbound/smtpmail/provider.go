// Package smtpmail is an EmailProvider adapter that sends over SMTP. It is used for
// local development and end-to-end tests against MailHog (a fake SMTP inbox), whereas
// resendmail is used for production. Both satisfy the same app.EmailProvider port, so
// the composition root swaps them by config with no change to the domain or handler.
package smtpmail

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/duykhanh/worklane/services/otp-dispatcher/internal/app"
)

// lineBreakRe matches a run of line breaks, which would end a header line early.
var lineBreakRe = regexp.MustCompile(`[\r\n]+`)

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
		return "", classify(err)
	}
	return "", nil
}

// classify wraps a send error and marks it permanent when the server gave a 5yz reply
// (RFC 5321: a definitive rejection such as an unknown mailbox, so retrying the same
// message cannot succeed). A 4yz reply or a network error stays transient.
func classify(err error) error {
	wrapped := fmt.Errorf("smtp: send: %w", err)
	var reply *textproto.Error
	if errors.As(err, &reply) && reply.Code >= 500 && reply.Code < 600 {
		return app.Permanent(wrapped)
	}
	return wrapped
}

// buildMessage assembles RFC 5322 headers + body. Lines are CRLF-terminated as SMTP
// requires. Header values are forced onto one line so that no value can add a header
// or start the body early (header injection).
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + headerValue(from) + "\r\n")
	b.WriteString("To: " + headerValue(to) + "\r\n")
	b.WriteString("Subject: " + headerValue(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// headerValue collapses line breaks in a header value to a single space.
func headerValue(v string) string {
	return lineBreakRe.ReplaceAllString(v, " ")
}
