package domain

import (
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// Channel is the delivery channel of an OTP.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

var e164Re = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// ValidChannel reports whether c is a delivery channel the platform supports.
func ValidChannel(c Channel) bool {
	return c == ChannelEmail || c == ChannelSMS
}

// ValidateRecipient checks that recipient is well-formed for its channel: an email
// address for email, an E.164 phone number for sms.
func ValidateRecipient(channel Channel, recipient string) error {
	switch channel {
	case ChannelEmail:
		if addr, err := mail.ParseAddress(recipient); err != nil || addr.Address != recipient {
			return ErrInvalidRecipient
		}
	case ChannelSMS:
		if !e164Re.MatchString(recipient) {
			return ErrInvalidRecipient
		}
	default:
		return ErrInvalidChannel
	}
	return nil
}

// Mask hides PII in a recipient for audit rows and logs, dispatching on channel.
func Mask(channel Channel, recipient string) string {
	if channel == ChannelSMS {
		return maskPhone(recipient)
	}
	return MaskRecipient(recipient)
}

// maskPhone keeps the leading '+', the first two and last two digits (e.g. +84***67).
// Anything too short or not starting with '+' returns a fixed mask so nothing leaks.
func maskPhone(p string) string {
	if len(p) < 5 || p[0] != '+' {
		return "***"
	}
	return p[:3] + "***" + p[len(p)-2:]
}

// OTPRequest is the domain entity for one send request, mirrored to the
// otp_requests audit table (with the recipient masked).
type OTPRequest struct {
	ID        string
	TenantID  string
	Recipient string
	Channel   Channel
	State     State
	CreatedAt time.Time
}

// MaskRecipient hides the local part of an email for logs and audit rows, keeping
// only the first character and the domain (e.g. "d***@gmail.com"). Malformed input
// with no usable local part returns a fixed "***" so nothing sensitive leaks.
func MaskRecipient(email string) string {
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
