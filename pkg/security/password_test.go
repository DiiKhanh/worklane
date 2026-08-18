package security

import "testing"

func TestHashPassword_RoundTrip(t *testing.T) {
	enc, err := HashPassword("correct horse")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if enc == "correct horse" {
		t.Fatal("hash must not equal plaintext")
	}
	ok, err := VerifyPassword(enc, "correct horse")
	if err != nil || !ok {
		t.Fatalf("verify correct: ok=%v err=%v", ok, err)
	}
	bad, err := VerifyPassword(enc, "wrong")
	if err != nil || bad {
		t.Fatalf("verify wrong must be false: ok=%v err=%v", bad, err)
	}
}

func TestVerifyPassword_BadFormat(t *testing.T) {
	if _, err := VerifyPassword("not-a-hash", "x"); err == nil {
		t.Fatal("want error on malformed hash")
	}
}
