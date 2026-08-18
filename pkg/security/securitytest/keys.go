// Package securitytest provides test-only helpers for pkg/security. It lives in its own
// package so the production security package never imports "testing".
package securitytest

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// KeyPair returns a fresh Ed25519 (privPEM, pubPEM) for tests in any package.
func KeyPair(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(priv)
	pkix, _ := x509.MarshalPKIXPublicKey(pub)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix}))
}
