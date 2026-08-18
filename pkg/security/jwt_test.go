package security

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func testKeys(t *testing.T) (priv string, pub string) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(privKey)
	pkix, _ := x509.MarshalPKIXPublicKey(pubKey)
	priv = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	pub = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}))
	return priv, pub
}

func TestJWT_IssueParse(t *testing.T) {
	priv, pub := testKeys(t)
	iss, err := NewIssuer(priv, time.Hour)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	ver, err := NewVerifier(pub)
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	tok, exp, err := iss.Issue("u1", "t1", "a@b.co", time.Now())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Fatal("exp should be in the future")
	}
	c, err := ver.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.UserID != "u1" || c.TenantID != "t1" || c.Email != "a@b.co" {
		t.Fatalf("claims wrong: %+v", c)
	}
}

func TestJWT_Expired(t *testing.T) {
	priv, pub := testKeys(t)
	iss, _ := NewIssuer(priv, time.Hour)
	ver, _ := NewVerifier(pub)
	tok, _, _ := iss.Issue("u1", "t1", "a@b.co", time.Now().Add(-2*time.Hour))
	if _, err := ver.Parse(tok); err == nil {
		t.Fatal("expired token must fail to parse")
	}
}

func TestJWT_WrongKey(t *testing.T) {
	priv, _ := testKeys(t)
	_, otherPub := testKeys(t)
	iss, _ := NewIssuer(priv, time.Hour)
	ver, _ := NewVerifier(otherPub)
	tok, _, _ := iss.Issue("u1", "t1", "a@b.co", time.Now())
	if _, err := ver.Parse(tok); err == nil {
		t.Fatal("token signed by a different key must fail")
	}
}
