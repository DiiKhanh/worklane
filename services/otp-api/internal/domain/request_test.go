package domain

import (
	"errors"
	"testing"
)

func TestValidChannel(t *testing.T) {
	for _, c := range []Channel{ChannelEmail, ChannelSMS} {
		if !ValidChannel(c) {
			t.Fatalf("%q should be valid", c)
		}
	}
	if ValidChannel(Channel("push")) {
		t.Fatal("push is not a supported channel yet")
	}
}

func TestValidateRecipient(t *testing.T) {
	cases := []struct {
		channel   Channel
		recipient string
		wantErr   error
	}{
		{ChannelEmail, "user@example.com", nil},
		{ChannelEmail, "+84901234567", ErrInvalidRecipient},
		{ChannelSMS, "+84901234567", nil},
		{ChannelSMS, "0901234567", ErrInvalidRecipient}, // not E.164
		{ChannelSMS, "user@example.com", ErrInvalidRecipient},
		{Channel("push"), "x", ErrInvalidChannel},
	}
	for _, tc := range cases {
		if got := ValidateRecipient(tc.channel, tc.recipient); !errors.Is(got, tc.wantErr) {
			t.Fatalf("ValidateRecipient(%q,%q)=%v want %v", tc.channel, tc.recipient, got, tc.wantErr)
		}
	}
}

func TestMask_Phone(t *testing.T) {
	if got := Mask(ChannelSMS, "+84901234567"); got != "+84***67" {
		t.Fatalf("mask sms = %q want +84***67", got)
	}
	if got := Mask(ChannelEmail, "a@b.co"); got != "a***@b.co" {
		t.Fatalf("mask email = %q want a***@b.co", got)
	}
	if got := Mask(ChannelSMS, "bad"); got != "***" {
		t.Fatalf("malformed phone must mask to ***, got %q", got)
	}
}

func TestMaskRecipient(t *testing.T) {
	cases := map[string]string{
		"duykhanh@gmail.com": "d***@gmail.com",
		"a@b.co":             "a***@b.co",
	}
	for in, want := range cases {
		if got := MaskRecipient(in); got != want {
			t.Fatalf("mask(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMaskRecipient_Malformed(t *testing.T) {
	// No local part / no '@' must never leak the input; return a fixed mask.
	for _, in := range []string{"", "@gmail.com", "no-at-sign"} {
		if got := MaskRecipient(in); got != "***" {
			t.Fatalf("mask(%q)=%q want ***", in, got)
		}
	}
}
