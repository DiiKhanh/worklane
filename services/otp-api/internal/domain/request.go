package domain

import (
	"strings"
	"time"
)

// Channel is the delivery channel of an OTP. Only email exists in the MVP.
type Channel string

const ChannelEmail Channel = "email"

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
