package app

import (
	"context"
	"fmt"
)

// emailSender renders subject+body from the email Template and delivers via EmailProvider.
type emailSender struct {
	p    EmailProvider
	name string
	tpl  Template
}

// NewEmailSender builds a Sender backed by an EmailProvider (e.g. Resend or SMTP).
func NewEmailSender(p EmailProvider, name string, tpl Template) Sender {
	return &emailSender{p: p, name: name, tpl: tpl}
}

func (s *emailSender) Name() string { return s.name }
func (s *emailSender) Send(ctx context.Context, to, code string) (string, error) {
	subject, body := s.tpl.Render(code)
	return s.p.Send(ctx, to, subject, body)
}

// smsSender renders a body-only message and delivers via SMSProvider.
type smsSender struct {
	p       SMSProvider
	name    string
	bodyFmt string
}

// NewSMSSender builds a Sender backed by an SMSProvider (e.g. Twilio). bodyFmt is a
// printf format with a single %s for the code.
func NewSMSSender(p SMSProvider, name, bodyFmt string) Sender {
	return &smsSender{p: p, name: name, bodyFmt: bodyFmt}
}

func (s *smsSender) Name() string { return s.name }
func (s *smsSender) Send(ctx context.Context, to, code string) (string, error) {
	return s.p.Send(ctx, to, fmt.Sprintf(s.bodyFmt, code))
}
