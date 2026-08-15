package domain

import "testing"

func TestHashCode_Deterministic(t *testing.T) {
	h1 := HashCode("123456", "salt-a")
	h2 := HashCode("123456", "salt-a")
	if h1 != h2 {
		t.Fatal("hash must be deterministic for same salt+code")
	}
	if HashCode("123456", "salt-b") == h1 {
		t.Fatal("different salt must change hash")
	}
}

func TestVerifyHash(t *testing.T) {
	h := HashCode("654321", "s")
	if !VerifyHash(h, "654321", "s") {
		t.Fatal("correct code must verify")
	}
	if VerifyHash(h, "000000", "s") {
		t.Fatal("wrong code must not verify")
	}
	if VerifyHash(h, "654321", "wrong-salt") {
		t.Fatal("correct code with wrong salt must not verify")
	}
}
