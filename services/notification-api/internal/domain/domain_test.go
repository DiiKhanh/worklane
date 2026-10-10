package domain

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestValidateRecipient(t *testing.T) {
	tests := []struct {
		name, channel, recipient string
		err                      error
	}{
		{"email ok", ChannelEmail, "an@example.com", nil},
		{"email with display name", ChannelEmail, "An <an@example.com>", ErrInvalidRecipient},
		{"email missing domain", ChannelEmail, "an@", ErrInvalidRecipient},
		{"email empty", ChannelEmail, "", ErrInvalidRecipient},
		{"sms ok", ChannelSMS, "+84901234567", nil},
		{"sms no plus", ChannelSMS, "84901234567", ErrInvalidRecipient},
		{"sms too short", ChannelSMS, "+8490", ErrInvalidRecipient},
		{"sms letters", ChannelSMS, "+84abc234567", ErrInvalidRecipient},
		{"unknown channel", "push", "token", ErrInvalidChannel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateRecipient(tt.channel, tt.recipient); !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestMask(t *testing.T) {
	tests := []struct{ channel, in, want string }{
		{ChannelEmail, "duy@gmail.com", "d***@gmail.com"},
		{ChannelEmail, "@gmail.com", "***"},
		{ChannelEmail, "no-at-sign", "***"},
		{ChannelSMS, "+84901234567", "+84***67"},
		{ChannelSMS, "84901234567", "***"},
		{ChannelSMS, "+849", "***"},
	}
	for _, tt := range tests {
		if got := Mask(tt.channel, tt.in); got != tt.want {
			t.Errorf("Mask(%s, %q) = %q, want %q", tt.channel, tt.in, got, tt.want)
		}
	}
}

func TestIDFromIdempotencyKey(t *testing.T) {
	id := IDFromIdempotencyKey("tenant-a", "order-42")
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("id %q is not uuid-shaped (must fit CHAR(36))", id)
	}
	if id != IDFromIdempotencyKey("tenant-a", "order-42") {
		t.Fatal("same tenant and key must give the same id")
	}
	if id == IDFromIdempotencyKey("tenant-b", "order-42") {
		t.Fatal("the same key under another tenant must give a different id")
	}
	// The separator keeps (tenant, key) pairs from colliding by concatenation.
	if IDFromIdempotencyKey("ab", "c") == IDFromIdempotencyKey("a", "bc") {
		t.Fatal("tenant/key boundary must be part of the digest")
	}
}

func TestValidKindChannelAndUserRef(t *testing.T) {
	if !ValidKind(KindMarketing) || !ValidKind(KindTransactional) || ValidKind("promo") || ValidKind("") {
		t.Fatal("ValidKind accepts exactly transactional and marketing")
	}
	for _, c := range Channels() {
		if !ValidChannel(c) {
			t.Fatalf("Channels() lists invalid channel %q", c)
		}
	}
	if ValidChannel("push") {
		t.Fatal("push is deferred and must be invalid")
	}
	if err := ValidateUserRef(strings.Repeat("u", MaxUserRefLen)); err != nil {
		t.Fatalf("max-length user_ref: %v", err)
	}
	if err := ValidateUserRef(strings.Repeat("u", MaxUserRefLen+1)); !errors.Is(err, ErrInvalidUserRef) {
		t.Fatalf("oversized user_ref: err = %v", err)
	}
}

func TestValidateTemplateFields(t *testing.T) {
	tests := []struct {
		name                                  string
		tname, channel, locale, subject, body string
		err                                   error
	}{
		{"ok email", "Welcome", ChannelEmail, "en", "Hi", "Hello {{name}}", nil},
		{"ok sms without subject", "Ping", ChannelSMS, "vi", "", "Hello", nil},
		{"bad channel", "Welcome", "push", "en", "Hi", "Hello", ErrInvalidChannel},
		{"blank name", "  ", ChannelEmail, "en", "Hi", "Hello", ErrInvalidTemplate},
		{"long name", strings.Repeat("n", MaxTemplateNameLen+1), ChannelEmail, "en", "Hi", "Hello", ErrInvalidTemplate},
		{"blank locale", "Welcome", ChannelEmail, "", "Hi", "Hello", ErrInvalidTemplate},
		{"long locale", "Welcome", ChannelEmail, strings.Repeat("l", MaxLocaleLen+1), "Hi", "Hello", ErrInvalidTemplate},
		{"sms with subject", "Ping", ChannelSMS, "vi", "Hi", "Hello", ErrInvalidTemplate},
		{"long subject", "Welcome", ChannelEmail, "en", strings.Repeat("s", MaxSubjectLen+1), "Hello", ErrInvalidTemplate},
		{"blank body", "Welcome", ChannelEmail, "en", "Hi", " \n", ErrInvalidTemplate},
		{"long body", "Welcome", ChannelEmail, "en", "Hi", strings.Repeat("b", MaxBodyLen+1), ErrInvalidTemplate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateFields(tt.tname, tt.channel, tt.locale, tt.subject, tt.body)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
		})
	}
}
