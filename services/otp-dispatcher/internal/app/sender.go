package app

import "context"

// emailSender delivers a rendered subject+body via an EmailProvider (Resend or SMTP).
type emailSender struct {
	p    EmailProvider
	name string
}

// NewEmailSender builds a Sender backed by an EmailProvider.
func NewEmailSender(p EmailProvider, name string) Sender { return &emailSender{p: p, name: name} }

func (s *emailSender) Name() string { return s.name }
func (s *emailSender) Send(ctx context.Context, to, subject, body string) (string, error) {
	return s.p.Send(ctx, to, subject, body)
}

// smsSender delivers a rendered body via an SMSProvider (Twilio). SMS has no subject.
type smsSender struct {
	p    SMSProvider
	name string
}

// NewSMSSender builds a Sender backed by an SMSProvider.
func NewSMSSender(p SMSProvider, name string) Sender { return &smsSender{p: p, name: name} }

func (s *smsSender) Name() string { return s.name }
func (s *smsSender) Send(ctx context.Context, to, _, body string) (string, error) {
	return s.p.Send(ctx, to, body)
}
