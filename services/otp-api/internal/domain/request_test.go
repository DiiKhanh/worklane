package domain

import "testing"

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
